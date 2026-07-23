package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/mikusaa/flaredns/backend/internal/cloudflare"
	"github.com/mikusaa/flaredns/backend/internal/security"
)

type APIToken struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	LastError      string     `json:"last_error"`
	ZoneCount      int        `json:"zone_count"`
	LastVerifiedAt *time.Time `json:"last_verified_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type Zone struct {
	ID           int64      `json:"id"`
	APITokenID   int64      `json:"api_token_id"`
	TokenName    string     `json:"token_name"`
	CloudflareID string     `json:"cloudflare_id"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	RecordCount  int        `json:"record_count"`
	IsDefault    bool       `json:"is_default"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
}

type AuditLog struct {
	ID           int64           `json:"id"`
	Username     string          `json:"username"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	ResourceName string          `json:"resource_name"`
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	Success      bool            `json:"success"`
	ErrorMessage string          `json:"error_message"`
	IPAddress    string          `json:"ip_address"`
	CreatedAt    time.Time       `json:"created_at"`
}

func (s *Store) ListTokens(ctx context.Context) ([]APIToken, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT t.id, t.name, t.status, t.last_error, t.last_verified_at, t.created_at, COUNT(z.id)
		FROM api_tokens t LEFT JOIN zones z ON z.api_token_id = t.id GROUP BY t.id ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]APIToken, 0)
	for rows.Next() {
		var item APIToken
		var verified sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.Status, &item.LastError, &verified, &item.CreatedAt, &item.ZoneCount); err != nil {
			return nil, err
		}
		if verified.Valid {
			item.LastVerifiedAt = &verified.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CreateToken(ctx context.Context, cipher *security.Cipher, name, token string) (int64, error) {
	ciphertext, nonce, err := cipher.Encrypt(token)
	if err != nil {
		return 0, err
	}
	result, err := s.DB.ExecContext(ctx, `INSERT INTO api_tokens(name, encrypted_token, nonce, last_verified_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`, name, ciphertext, nonce)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) TokenSecret(ctx context.Context, cipher *security.Cipher, id int64) (string, string, error) {
	var name string
	var ciphertext, nonce []byte
	if err := s.DB.QueryRowContext(ctx, `SELECT name, encrypted_token, nonce FROM api_tokens WHERE id = ?`, id).Scan(&name, &ciphertext, &nonce); err != nil {
		return "", "", err
	}
	token, err := cipher.Decrypt(ciphertext, nonce)
	return name, token, err
}

func (s *Store) SetTokenStatus(ctx context.Context, id int64, status, message string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE api_tokens SET status = ?, last_error = ?, last_verified_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, message, id)
	return err
}

func (s *Store) DeleteToken(ctx context.Context, id int64) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	if err == nil {
		if n, _ := result.RowsAffected(); n == 0 {
			return sql.ErrNoRows
		}
	}
	return err
}

func (s *Store) SyncZones(ctx context.Context, tokenID int64, zones []cloudflare.Zone, counts map[string]int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT cloudflare_id FROM zones WHERE api_token_id = ?`, tokenID)
	if err != nil {
		tx.Rollback()
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			tx.Rollback()
			return err
		}
		existing[id] = true
	}
	rows.Close()
	for _, zone := range zones {
		delete(existing, zone.ID)
		_, err = tx.ExecContext(ctx, `INSERT INTO zones(api_token_id, cloudflare_id, name, status, record_count, last_synced_at)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(api_token_id, cloudflare_id) DO UPDATE SET name=excluded.name, status=excluded.status,
			record_count=excluded.record_count, last_synced_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP`, tokenID, zone.ID, zone.Name, zone.Status, counts[zone.ID])
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	for id := range existing {
		if _, err = tx.ExecContext(ctx, `DELETE FROM zones WHERE api_token_id = ? AND cloudflare_id = ?`, tokenID, id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListZones(ctx context.Context) ([]Zone, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT z.id, z.api_token_id, t.name, z.cloudflare_id, z.name, z.status, z.record_count, z.is_default, z.last_synced_at
		FROM zones z JOIN api_tokens t ON t.id=z.api_token_id ORDER BY z.is_default DESC, z.name, t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Zone, 0)
	for rows.Next() {
		var item Zone
		var synced sql.NullTime
		if err := rows.Scan(&item.ID, &item.APITokenID, &item.TokenName, &item.CloudflareID, &item.Name, &item.Status, &item.RecordCount, &item.IsDefault, &synced); err != nil {
			return nil, err
		}
		if synced.Valid {
			item.LastSyncedAt = &synced.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ZoneByID(ctx context.Context, id int64) (*Zone, error) {
	item := &Zone{}
	var synced sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT z.id, z.api_token_id, t.name, z.cloudflare_id, z.name, z.status, z.record_count, z.is_default, z.last_synced_at
		FROM zones z JOIN api_tokens t ON t.id=z.api_token_id WHERE z.id = ?`, id).
		Scan(&item.ID, &item.APITokenID, &item.TokenName, &item.CloudflareID, &item.Name, &item.Status, &item.RecordCount, &item.IsDefault, &synced)
	if synced.Valid {
		item.LastSyncedAt = &synced.Time
	}
	return item, err
}

func (s *Store) SetDefaultZone(ctx context.Context, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE zones SET is_default = 0 WHERE is_default = 1`); err == nil {
		var exists int
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM zones WHERE id = ?`, id).Scan(&exists)
		if err == nil && exists == 0 {
			err = sql.ErrNoRows
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE zones SET is_default = 1 WHERE id = ?`, id)
		}
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateZoneRecordCount(ctx context.Context, id int64, delta int) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE zones SET record_count = MAX(0, record_count + ?), updated_at = CURRENT_TIMESTAMP WHERE id = ?`, delta, id)
	return err
}

func (s *Store) SetZoneRecordCount(ctx context.Context, id int64, count int) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE zones SET record_count = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, count, id)
	return err
}

func (s *Store) AddAudit(ctx context.Context, userID int64, username, action, resourceType, resourceID, resourceName string, before, after any, success bool, errorMessage, ip string) error {
	marshal := func(value any) any {
		if value == nil {
			return nil
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		return string(raw)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO audit_logs(user_id, username, action, resource_type, resource_id, resource_name, before_json, after_json, success, error_message, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, userID, username, action, resourceType, resourceID, resourceName, marshal(before), marshal(after), success, errorMessage, ip)
	return err
}

func (s *Store) ListAudit(ctx context.Context, page, perPage int) ([]AuditLog, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, username, action, resource_type, resource_id, resource_name,
		COALESCE(before_json, 'null'), COALESCE(after_json, 'null'), success, error_message, ip_address, created_at
		FROM audit_logs ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]AuditLog, 0)
	for rows.Next() {
		var item AuditLog
		var beforeJSON, afterJSON string
		if err := rows.Scan(&item.ID, &item.Username, &item.Action, &item.ResourceType, &item.ResourceID, &item.ResourceName,
			&beforeJSON, &afterJSON, &item.Success, &item.ErrorMessage, &item.IPAddress, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.Before = json.RawMessage(beforeJSON)
		item.After = json.RawMessage(afterJSON)
		result = append(result, item)
	}
	return result, total, rows.Err()
}

func (s *Store) DebugNoPlaintextToken(ctx context.Context, token string) (bool, error) {
	var count int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_tokens WHERE CAST(encrypted_token AS TEXT) = ?`, token).Scan(&count)
	return count == 0, err
}
