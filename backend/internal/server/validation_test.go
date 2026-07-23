package server

import (
	"github.com/mikusaa/flaredns/backend/internal/cloudflare"
	"testing"
)

func TestValidateRecord(t *testing.T) {
	tests := []struct {
		name    string
		payload cloudflare.RecordPayload
		valid   bool
	}{
		{"A", cloudflare.RecordPayload{Type: "A", Name: "api.example.com", Content: "1.2.3.4", TTL: 1, Proxied: true}, true},
		{"bad IPv4", cloudflare.RecordPayload{Type: "A", Name: "api.example.com", Content: "300.1.1.1", TTL: 1}, false},
		{"MX priority", cloudflare.RecordPayload{Type: "MX", Name: "example.com", Content: "mail.example.com", TTL: 300}, false},
		{"valid SRV", cloudflare.RecordPayload{Type: "SRV", Name: "_sip._tcp.example.com", TTL: 300, Data: map[string]any{"service": "_sip", "proto": "_tcp", "name": "example.com", "priority": 10, "weight": 5, "port": 443, "target": "sip.example.com"}}, true},
		{"SRV port out of range", cloudflare.RecordPayload{Type: "SRV", Name: "_sip._tcp.example.com", TTL: 300, Data: map[string]any{"service": "_sip", "proto": "_tcp", "name": "example.com", "priority": 10, "weight": 5, "port": 70000, "target": "sip.example.com"}}, false},
		{"CAA invalid flags", cloudflare.RecordPayload{Type: "CAA", Name: "example.com", TTL: 300, Data: map[string]any{"flags": 256, "tag": "issue", "value": "letsencrypt.org"}}, false},
		{"CAA invalid tag", cloudflare.RecordPayload{Type: "CAA", Name: "example.com", TTL: 300, Data: map[string]any{"flags": 0, "tag": "unknown", "value": "letsencrypt.org"}}, false},
		{"unsupported", cloudflare.RecordPayload{Type: "HTTPS", Name: "example.com", Content: "x", TTL: 300}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := validateRecord(&test.payload)
			if test.valid && len(fields) > 0 {
				t.Fatalf("unexpected fields: %#v", fields)
			}
			if !test.valid && len(fields) == 0 {
				t.Fatal("expected validation failure")
			}
		})
	}
}
