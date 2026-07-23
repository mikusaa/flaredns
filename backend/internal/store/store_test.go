package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/mikusaa/flaredns/backend/internal/security"
	"github.com/go-webauthn/webauthn/webauthn"
)

func TestEmptyCollectionsMarshalAsArrays(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	values := []any{}
	tokens, err := s.ListTokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	values = append(values, tokens)
	zones, err := s.ListZones(ctx)
	if err != nil {
		t.Fatal(err)
	}
	values = append(values, zones)
	logs, _, err := s.ListAudit(ctx, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	values = append(values, logs)
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != "[]" {
			t.Fatalf("expected [], got %s", raw)
		}
	}
}

func TestAuditLogJSONRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, _, err := s.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	admin, err := s.UserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddAudit(ctx, admin.ID, "admin", "修改 DNS 记录", "dns_record", "record-1", "api.example.com",
		map[string]any{"content": "1.1.1.1"}, map[string]any{"content": "2.2.2.2"}, true, "", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	logs, total, err := s.ListAudit(ctx, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("unexpected audit result: total=%d logs=%d", total, len(logs))
	}
	var after map[string]string
	if err := json.Unmarshal(logs[0].After, &after); err != nil {
		t.Fatal(err)
	}
	if after["content"] != "2.2.2.2" {
		t.Fatalf("unexpected audit JSON: %s", logs[0].After)
	}
}

func TestResetPasswordRevokesSessionsAndClearsLockout(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	oldPassword, _, err := s.EnsureAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := s.UserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	sessionToken, _, err := s.CreateSession(ctx, admin.ID, time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordLoginFailure(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}

	newPassword := "new-password-1234"
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}

	admin, err = s.UserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if admin.Failed != 0 || admin.LockedUntil.Valid {
		t.Fatalf("login lockout was not cleared: failed=%d locked=%v", admin.Failed, admin.LockedUntil.Valid)
	}
	if security.VerifyPassword(admin.PasswordHash, oldPassword) {
		t.Fatal("old password is still valid")
	}
	if !security.VerifyPassword(admin.PasswordHash, newPassword) {
		t.Fatal("new password is invalid")
	}
	if _, err := s.SessionByToken(ctx, sessionToken); err != sql.ErrNoRows {
		t.Fatalf("existing session was not revoked: %v", err)
	}
	logs, total, err := s.ListAudit(ctx, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(logs) != 1 || logs[0].Action != "通过 CLI 重置密码" {
		t.Fatalf("password reset audit log missing: total=%d logs=%+v", total, logs)
	}
}

func TestUserByCredentialID(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, _, err := s.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	admin, err := s.UserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	credentialID := []byte("credential-id")
	if _, err := s.AddPasskey(ctx, admin.ID, "test key", &webauthn.Credential{ID: credentialID, PublicKey: []byte("public-key")}); err != nil {
		t.Fatal(err)
	}

	found, err := s.UserByCredentialID(ctx, credentialID)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != admin.ID || len(found.Credentials) != 1 || string(found.Credentials[0].ID) != string(credentialID) {
		t.Fatalf("unexpected credential owner: %+v", found)
	}
}
