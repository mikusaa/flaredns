package server

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/mikusaa/flaredns/backend/internal/cloudflare"
	"github.com/gin-gonic/gin"
)

func (a *App) listTokens(c *gin.Context) {
	items, err := a.store.ListTokens(c.Request.Context())
	if err != nil {
		internal(c, err)
		return
	}
	ok(c, items)
}

func (a *App) createToken(c *gin.Context) {
	var input struct {
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	if !bindJSON(c, &input) {
		return
	}
	input.Name, input.Token = strings.TrimSpace(input.Name), strings.TrimSpace(input.Token)
	if len(input.Name) < 1 || len(input.Name) > 80 || len(input.Token) < 20 {
		fail(c, http.StatusBadRequest, "invalid_token", "请填写名称和有效的 Cloudflare API Token", nil)
		return
	}
	if err := a.cloudflare.VerifyToken(c.Request.Context(), input.Token); err != nil {
		fail(c, http.StatusBadRequest, "cloudflare_auth_failed", err.Error(), nil)
		return
	}
	zones, err := a.cloudflare.ListZones(c.Request.Context(), input.Token)
	if err != nil {
		fail(c, http.StatusBadRequest, "cloudflare_zone_failed", err.Error(), nil)
		return
	}
	id, err := a.store.CreateToken(c.Request.Context(), a.cipher, input.Name, input.Token)
	if err != nil {
		internal(c, err)
		return
	}
	counts := a.countRecords(c.Request.Context(), input.Token, zones)
	if err := a.store.SyncZones(c.Request.Context(), id, zones, counts); err != nil {
		if cleanupErr := a.store.DeleteToken(c.Request.Context(), id); cleanupErr != nil {
			internal(c, fmt.Errorf("sync zones: %w; roll back API token: %v", err, cleanupErr))
			return
		}
		internal(c, err)
		return
	}
	s := currentSession(c)
	_ = a.store.AddAudit(c.Request.Context(), s.UserID, s.Username, "添加 API Token", "api_token", strconv.FormatInt(id, 10), input.Name, nil, gin.H{"name": input.Name, "zones": len(zones)}, true, "", c.ClientIP())
	created(c, gin.H{"id": id, "name": input.Name, "zone_count": len(zones)})
}

func (a *App) deleteToken(c *gin.Context) {
	id, okID := parseID(c, "id")
	if !okID {
		return
	}
	name, _, err := a.store.TokenSecret(c.Request.Context(), a.cipher, id)
	if err != nil {
		if err == sql.ErrNoRows {
			fail(c, http.StatusNotFound, "not_found", "API Token 不存在", nil)
			return
		}
		internal(c, err)
		return
	}
	if err := a.store.DeleteToken(c.Request.Context(), id); err != nil {
		internal(c, err)
		return
	}
	s := currentSession(c)
	_ = a.store.AddAudit(c.Request.Context(), s.UserID, s.Username, "删除 API Token", "api_token", c.Param("id"), name, gin.H{"name": name}, nil, true, "", c.ClientIP())
	ok(c, gin.H{"deleted": true})
}

func (a *App) verifyToken(c *gin.Context) {
	id, okID := parseID(c, "id")
	if !okID {
		return
	}
	_, token, err := a.store.TokenSecret(c.Request.Context(), a.cipher, id)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "API Token 不存在", nil)
		return
	}
	err = a.cloudflare.VerifyToken(c.Request.Context(), token)
	if err != nil {
		_ = a.store.SetTokenStatus(c.Request.Context(), id, "invalid", err.Error())
		fail(c, http.StatusBadGateway, "cloudflare_auth_failed", err.Error(), nil)
		return
	}
	_ = a.store.SetTokenStatus(c.Request.Context(), id, "valid", "")
	ok(c, gin.H{"valid": true})
}

func (a *App) syncToken(c *gin.Context) {
	id, okID := parseID(c, "id")
	if !okID {
		return
	}
	_, token, err := a.store.TokenSecret(c.Request.Context(), a.cipher, id)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "API Token 不存在", nil)
		return
	}
	if err = a.cloudflare.VerifyToken(c.Request.Context(), token); err != nil {
		_ = a.store.SetTokenStatus(c.Request.Context(), id, "invalid", err.Error())
		fail(c, http.StatusBadGateway, "cloudflare_auth_failed", err.Error(), nil)
		return
	}
	zones, err := a.cloudflare.ListZones(c.Request.Context(), token)
	if err != nil {
		_ = a.store.SetTokenStatus(c.Request.Context(), id, "error", err.Error())
		fail(c, http.StatusBadGateway, "cloudflare_error", err.Error(), nil)
		return
	}
	counts := a.countRecords(c.Request.Context(), token, zones)
	if err := a.store.SyncZones(c.Request.Context(), id, zones, counts); err != nil {
		internal(c, err)
		return
	}
	_ = a.store.SetTokenStatus(c.Request.Context(), id, "valid", "")
	ok(c, gin.H{"zone_count": len(zones)})
}

func (a *App) countRecords(ctx context.Context, token string, zones []cloudflare.Zone) map[string]int {
	counts := make(map[string]int, len(zones))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, zone := range zones {
		zone := zone
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			count, err := a.cloudflare.CountRecords(ctx, token, zone.ID)
			if err == nil {
				mu.Lock()
				counts[zone.ID] = count
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return counts
}

func (a *App) listZones(c *gin.Context) {
	items, err := a.store.ListZones(c.Request.Context())
	if err != nil {
		internal(c, err)
		return
	}
	ok(c, items)
}

func (a *App) setDefaultZone(c *gin.Context) {
	id, okID := parseID(c, "id")
	if !okID {
		return
	}
	if err := a.store.SetDefaultZone(c.Request.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			fail(c, http.StatusNotFound, "not_found", "Zone 不存在", nil)
			return
		}
		internal(c, err)
		return
	}
	ok(c, gin.H{"default": true})
}

func (a *App) zoneAccess(c *gin.Context) (int64, string, string, bool) {
	id, okID := parseID(c, "id")
	if !okID {
		return 0, "", "", false
	}
	zone, err := a.store.ZoneByID(c.Request.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			fail(c, http.StatusNotFound, "not_found", "Zone 不存在", nil)
		} else {
			internal(c, err)
		}
		return 0, "", "", false
	}
	_, token, err := a.store.TokenSecret(c.Request.Context(), a.cipher, zone.APITokenID)
	if err != nil {
		internal(c, err)
		return 0, "", "", false
	}
	return id, zone.CloudflareID, token, true
}

func (a *App) listRecords(c *gin.Context) {
	zoneID, cloudflareID, token, okID := a.zoneAccess(c)
	if !okID {
		return
	}
	records, err := a.cloudflare.ListRecords(c.Request.Context(), token, cloudflareID)
	if err != nil {
		fail(c, http.StatusBadGateway, "cloudflare_error", err.Error(), nil)
		return
	}
	_ = a.store.SetZoneRecordCount(c.Request.Context(), zoneID, len(records))
	c.JSON(http.StatusOK, gin.H{"data": records, "meta": gin.H{"total": len(records)}})
}

func (a *App) createRecord(c *gin.Context) {
	zoneID, cloudflareID, token, okID := a.zoneAccess(c)
	if !okID {
		return
	}
	var payload cloudflare.RecordPayload
	if !bindJSON(c, &payload) {
		return
	}
	if fields := validateRecord(&payload); len(fields) > 0 {
		fail(c, http.StatusBadRequest, "invalid_record", "DNS 记录内容不正确", fields)
		return
	}
	record, err := a.cloudflare.CreateRecord(c.Request.Context(), token, cloudflareID, payload)
	if err != nil {
		fail(c, http.StatusBadGateway, "cloudflare_error", err.Error(), nil)
		return
	}
	_ = a.store.UpdateZoneRecordCount(c.Request.Context(), zoneID, 1)
	a.auditRecord(c, "创建 DNS 记录", record.ID, record.Name, nil, record, true, "")
	created(c, record)
}

func (a *App) updateRecord(c *gin.Context) {
	_, cloudflareID, token, okID := a.zoneAccess(c)
	if !okID {
		return
	}
	var payload cloudflare.RecordPayload
	if !bindJSON(c, &payload) {
		return
	}
	if fields := validateRecord(&payload); len(fields) > 0 {
		fail(c, http.StatusBadRequest, "invalid_record", "DNS 记录内容不正确", fields)
		return
	}
	before := a.findRecord(c, token, cloudflareID, c.Param("recordID"))
	if before == nil {
		return
	}
	record, err := a.cloudflare.UpdateRecord(c.Request.Context(), token, cloudflareID, c.Param("recordID"), payload)
	if err != nil {
		fail(c, http.StatusBadGateway, "cloudflare_error", err.Error(), nil)
		return
	}
	a.auditRecord(c, "修改 DNS 记录", record.ID, record.Name, before, record, true, "")
	ok(c, record)
}

func (a *App) deleteRecord(c *gin.Context) {
	zoneID, cloudflareID, token, okID := a.zoneAccess(c)
	if !okID {
		return
	}
	before := a.findRecord(c, token, cloudflareID, c.Param("recordID"))
	if before == nil {
		return
	}
	if err := a.cloudflare.DeleteRecord(c.Request.Context(), token, cloudflareID, c.Param("recordID")); err != nil {
		fail(c, http.StatusBadGateway, "cloudflare_error", err.Error(), nil)
		return
	}
	_ = a.store.UpdateZoneRecordCount(c.Request.Context(), zoneID, -1)
	a.auditRecord(c, "删除 DNS 记录", before.ID, before.Name, before, nil, true, "")
	ok(c, gin.H{"deleted": true})
}

func (a *App) findRecord(c *gin.Context, token, zoneID, recordID string) *cloudflare.DNSRecord {
	records, err := a.cloudflare.ListRecords(c.Request.Context(), token, zoneID)
	if err != nil {
		fail(c, http.StatusBadGateway, "cloudflare_error", err.Error(), nil)
		return nil
	}
	for i := range records {
		if records[i].ID == recordID {
			return &records[i]
		}
	}
	fail(c, http.StatusNotFound, "not_found", "DNS 记录不存在", nil)
	return nil
}

type batchResult struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func (a *App) batchRecords(c *gin.Context) {
	zoneID, cloudflareID, token, okID := a.zoneAccess(c)
	if !okID {
		return
	}
	var input struct {
		Action    string   `json:"action"`
		RecordIDs []string `json:"record_ids"`
	}
	if !bindJSON(c, &input) {
		return
	}
	if input.Action != "delete" && input.Action != "proxy_on" && input.Action != "proxy_off" {
		fail(c, http.StatusBadRequest, "invalid_action", "不支持的批量操作", nil)
		return
	}
	if len(input.RecordIDs) < 1 || len(input.RecordIDs) > 100 {
		fail(c, http.StatusBadRequest, "invalid_selection", "一次请选择 1 至 100 条记录", nil)
		return
	}
	seen := make(map[string]struct{}, len(input.RecordIDs))
	for i, id := range input.RecordIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			fail(c, http.StatusBadRequest, "invalid_selection", "记录 ID 不能为空", nil)
			return
		}
		if _, exists := seen[id]; exists {
			fail(c, http.StatusBadRequest, "invalid_selection", "记录 ID 不能重复", nil)
			return
		}
		seen[id] = struct{}{}
		input.RecordIDs[i] = id
	}
	all, err := a.cloudflare.ListRecords(c.Request.Context(), token, cloudflareID)
	if err != nil {
		fail(c, http.StatusBadGateway, "cloudflare_error", err.Error(), nil)
		return
	}
	byID := map[string]cloudflare.DNSRecord{}
	for _, record := range all {
		byID[record.ID] = record
	}
	results := make([]batchResult, len(input.RecordIDs))
	ctx := c.Request.Context()
	session := currentSession(c)
	clientIP := c.ClientIP()
	actionName := map[string]string{"delete": "批量删除 DNS 记录", "proxy_on": "批量开启 Proxy", "proxy_off": "批量关闭 Proxy"}[input.Action]
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, id := range input.RecordIDs {
		i, id := i, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			before, found := byID[id]
			if !found {
				results[i] = batchResult{ID: id, Error: "记录不存在"}
				return
			}
			var actionErr error
			var after any
			if input.Action == "delete" {
				actionErr = a.cloudflare.DeleteRecord(ctx, token, cloudflareID, id)
			} else if !before.Proxiable {
				actionErr = fmt.Errorf("该记录不支持 Cloudflare Proxy")
			} else {
				payload := cloudflare.RecordPayload{Type: before.Type, Name: before.Name, Content: before.Content, TTL: before.TTL, Proxied: input.Action == "proxy_on", Priority: before.Priority, Data: before.Data}
				if payload.Proxied {
					payload.TTL = 1
				}
				after, actionErr = a.cloudflare.UpdateRecord(ctx, token, cloudflareID, id, payload)
			}
			results[i] = batchResult{ID: id, Success: actionErr == nil}
			if actionErr != nil {
				results[i].Error = actionErr.Error()
			}
			_ = a.store.AddAudit(ctx, session.UserID, session.Username, actionName, "dns_record", id, before.Name, before, after, actionErr == nil, results[i].Error, clientIP)
		}()
	}
	wg.Wait()
	deleted := 0
	for _, item := range results {
		if input.Action == "delete" && item.Success {
			deleted++
		}
	}
	if deleted > 0 {
		_ = a.store.UpdateZoneRecordCount(c.Request.Context(), zoneID, -deleted)
	}
	ok(c, gin.H{"results": results})
}

func (a *App) auditRecord(c *gin.Context, action, id, name string, before, after any, success bool, message string) {
	s := currentSession(c)
	_ = a.store.AddAudit(c.Request.Context(), s.UserID, s.Username, action, "dns_record", id, name, before, after, success, message, c.ClientIP())
}

func (a *App) listLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	items, total, err := a.store.ListAudit(c.Request.Context(), page, perPage)
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "meta": gin.H{"page": page, "per_page": perPage, "total": total}})
}

func validateRecord(p *cloudflare.RecordPayload) map[string]string {
	fields := map[string]string{}
	p.Type = strings.ToUpper(strings.TrimSpace(p.Type))
	p.Name = strings.TrimSpace(p.Name)
	p.Content = strings.TrimSpace(p.Content)
	supported := map[string]bool{"A": true, "AAAA": true, "CNAME": true, "TXT": true, "MX": true, "SRV": true, "CAA": true}
	if !supported[p.Type] {
		fields["type"] = "不支持该记录类型"
	}
	if p.Name == "" {
		fields["name"] = "名称不能为空"
	}
	if p.TTL != 1 && (p.TTL < 60 || p.TTL > 86400) {
		fields["ttl"] = "TTL 应为 Auto 或 60 至 86400 秒"
	}
	switch p.Type {
	case "A":
		if ip := net.ParseIP(p.Content); ip == nil || ip.To4() == nil {
			fields["content"] = "请输入有效 IPv4 地址"
		}
	case "AAAA":
		if ip := net.ParseIP(p.Content); ip == nil || ip.To4() != nil {
			fields["content"] = "请输入有效 IPv6 地址"
		}
	case "CNAME":
		if p.Content == "" {
			fields["content"] = "目标域名不能为空"
		}
	case "TXT":
		if p.Content == "" {
			fields["content"] = "TXT 内容不能为空"
		}
	case "MX":
		if p.Content == "" {
			fields["content"] = "邮件服务器不能为空"
		}
		if p.Priority == nil || *p.Priority < 0 || *p.Priority > 65535 {
			fields["priority"] = "优先级应为 0 至 65535"
		}
		p.Proxied = false
	case "SRV":
		requiredData(p.Data, fields, "service", "proto", "name", "priority", "weight", "port", "target")
		validateDataInteger(p.Data, fields, "priority", 0, 65535)
		validateDataInteger(p.Data, fields, "weight", 0, 65535)
		validateDataInteger(p.Data, fields, "port", 1, 65535)
		p.Content = ""
		p.Proxied = false
	case "CAA":
		requiredData(p.Data, fields, "flags", "tag", "value")
		validateDataInteger(p.Data, fields, "flags", 0, 255)
		if tag := strings.TrimSpace(fmt.Sprint(p.Data["tag"])); tag != "issue" && tag != "issuewild" && tag != "iodef" {
			fields["tag"] = "Tag 应为 issue、issuewild 或 iodef"
		}
		p.Content = ""
		p.Proxied = false
	}
	if p.Proxied && p.Type != "A" && p.Type != "AAAA" && p.Type != "CNAME" {
		fields["proxied"] = "该记录类型不支持 Proxy"
	}
	if p.Proxied {
		p.TTL = 1
	}
	return fields
}
func requiredData(data map[string]any, fields map[string]string, keys ...string) {
	for _, key := range keys {
		if data == nil || data[key] == nil || strings.TrimSpace(fmt.Sprint(data[key])) == "" {
			fields[key] = "该字段不能为空"
		}
	}
}

func validateDataInteger(data map[string]any, fields map[string]string, key string, min, max int64) {
	if data == nil || data[key] == nil {
		return
	}
	value, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(data[key])), 10, 64)
	if err != nil || value < min || value > max {
		fields[key] = fmt.Sprintf("应为 %d 至 %d 的整数", min, max)
	}
}
