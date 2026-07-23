package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListZonesPaginatesAndAuthenticates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing bearer token")
		}
		page := r.URL.Query().Get("page")
		result := []Zone{{ID: "one", Name: "one.example", Status: "active"}}
		if page == "2" {
			result = []Zone{{ID: "two", Name: "two.example", Status: "active"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result, "result_info": map[string]int{"page": map[string]int{"": 1, "1": 1, "2": 2}[page], "total_pages": 2, "total_count": 2}})
	}))
	defer server.Close()
	zones, err := New(server.URL).ListZones(context.Background(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) != 2 || zones[1].Name != "two.example" {
		t.Fatalf("unexpected zones: %#v", zones)
	}
}

func TestCloudflareErrorIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []APIError{{Code: 9109, Message: "Invalid token"}}})
	}))
	defer server.Close()
	err := New(server.URL).VerifyToken(context.Background(), "bad")
	if err == nil || err.Error() != "Cloudflare API 9109: Invalid token" {
		t.Fatalf("unexpected error: %v", err)
	}
}
