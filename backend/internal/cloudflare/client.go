package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e APIError) Error() string { return fmt.Sprintf("Cloudflare API %d: %s", e.Code, e.Message) }

type envelope struct {
	Success    bool            `json:"success"`
	Result     json.RawMessage `json:"result"`
	Errors     []APIError      `json:"errors"`
	ResultInfo struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
		TotalCount int `json:"total_count"`
	} `json:"result_info"`
}

type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type DNSRecord struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Content    string         `json:"content"`
	TTL        int            `json:"ttl"`
	Proxied    bool           `json:"proxied"`
	Proxiable  bool           `json:"proxiable"`
	Priority   *int           `json:"priority,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Comment    string         `json:"comment,omitempty"`
	ModifiedOn time.Time      `json:"modified_on"`
}

type RecordPayload struct {
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Content  string         `json:"content,omitempty"`
	TTL      int            `json:"ttl"`
	Proxied  bool           `json:"proxied,omitempty"`
	Priority *int           `json:"priority,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

func New(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) VerifyToken(ctx context.Context, token string) error {
	var result struct {
		Status string `json:"status"`
	}
	if _, err := c.do(ctx, token, http.MethodGet, "/user/tokens/verify", nil, &result); err != nil {
		return err
	}
	if result.Status != "active" {
		return fmt.Errorf("Cloudflare API token is %s", result.Status)
	}
	return nil
}

func (c *Client) ListZones(ctx context.Context, token string) ([]Zone, error) {
	all := make([]Zone, 0)
	for page := 1; ; page++ {
		var zones []Zone
		info, err := c.do(ctx, token, http.MethodGet, "/zones?per_page=50&page="+strconv.Itoa(page), nil, &zones)
		if err != nil {
			return nil, err
		}
		all = append(all, zones...)
		if info.TotalPages <= page || len(zones) == 0 {
			return all, nil
		}
	}
}

func (c *Client) CountRecords(ctx context.Context, token, zoneID string) (int, error) {
	var records []DNSRecord
	info, err := c.do(ctx, token, http.MethodGet, "/zones/"+url.PathEscape(zoneID)+"/dns_records?per_page=1", nil, &records)
	return info.TotalCount, err
}

func (c *Client) ListRecords(ctx context.Context, token, zoneID string) ([]DNSRecord, error) {
	all := make([]DNSRecord, 0)
	for page := 1; ; page++ {
		var records []DNSRecord
		path := fmt.Sprintf("/zones/%s/dns_records?per_page=500&page=%d", url.PathEscape(zoneID), page)
		info, err := c.do(ctx, token, http.MethodGet, path, nil, &records)
		if err != nil {
			return nil, err
		}
		all = append(all, records...)
		if info.TotalPages <= page || len(records) == 0 {
			return all, nil
		}
	}
}

func (c *Client) CreateRecord(ctx context.Context, token, zoneID string, payload RecordPayload) (*DNSRecord, error) {
	var record DNSRecord
	_, err := c.do(ctx, token, http.MethodPost, "/zones/"+url.PathEscape(zoneID)+"/dns_records", payload, &record)
	return &record, err
}

func (c *Client) UpdateRecord(ctx context.Context, token, zoneID, recordID string, payload RecordPayload) (*DNSRecord, error) {
	var record DNSRecord
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records/" + url.PathEscape(recordID)
	_, err := c.do(ctx, token, http.MethodPut, path, payload, &record)
	return &record, err
}

func (c *Client) DeleteRecord(ctx context.Context, token, zoneID, recordID string) error {
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records/" + url.PathEscape(recordID)
	_, err := c.do(ctx, token, http.MethodDelete, path, nil, nil)
	return err
}

func (c *Client) do(ctx context.Context, token, method, path string, body, target any) (*struct {
	Page, TotalPages, TotalCount int
}, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cloudflare request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("Cloudflare returned HTTP %d with an invalid response", resp.StatusCode)
	}
	if !env.Success || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if len(env.Errors) > 0 {
			return nil, env.Errors[0]
		}
		return nil, fmt.Errorf("Cloudflare returned HTTP %d", resp.StatusCode)
	}
	if target != nil && len(env.Result) > 0 && string(env.Result) != "null" {
		if err := json.Unmarshal(env.Result, target); err != nil {
			return nil, err
		}
	}
	return &struct{ Page, TotalPages, TotalCount int }{env.ResultInfo.Page, env.ResultInfo.TotalPages, env.ResultInfo.TotalCount}, nil
}
