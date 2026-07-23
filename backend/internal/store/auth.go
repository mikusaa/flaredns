package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mikusaa/flaredns/backend/internal/security"
	"github.com/go-webauthn/webauthn/webauthn"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Handle       []byte
	Failed       int
	LockedUntil  sql.NullTime
	Credentials  []webauthn.Credential
}

func (u *User) WebAuthnID() []byte                         { return u.Handle }
func (u *User) WebAuthnName() string                       { return u.Username }
func (u *User) WebAuthnDisplayName() string                { return u.Username }
func (u *User) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

type Session struct {
	ID                int64     `json:"-"`
	UserID            int64     `json:"user_id"`
	Username          string    `json:"username"`
	CSRFToken         string    `json:"csrf_token"`
	ReauthenticatedAt time.Time `json:"reauthenticated_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type Passkey struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func (s *Store) EnsureAdmin(ctx context.Context) (password string, created bool, err error) {
	var count int
	if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count > 0 {
		return "", false, err
	}
	password, err = security.RandomToken(18)
	if err != nil {
		return "", false, err
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return "", false, err
	}
	handle := make([]byte, 64)
	if _, err := security.RandomBytes(handle); err != nil {
		return "", false, err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO users(username, password_hash, webauthn_handle) VALUES ('admin', ?, ?)`, hash, handle)
	return password, err == nil, err
}

func (s *Store) UserByUsername(ctx context.Context, username string) (*User, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT id, username, password_hash, webauthn_handle, failed_attempts, locked_until FROM users WHERE username = ?`, username)
	return s.scanUserWithCredentials(ctx, row)
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT id, username, password_hash, webauthn_handle, failed_attempts, locked_until FROM users WHERE id = ?`, id)
	return s.scanUserWithCredentials(ctx, row)
}

func (s *Store) UserByHandle(ctx context.Context, handle []byte) (*User, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT id, username, password_hash, webauthn_handle, failed_attempts, locked_until FROM users WHERE webauthn_handle = ?`, handle)
	return s.scanUserWithCredentials(ctx, row)
}

func (s *Store) UserByCredentialID(ctx context.Context, credentialID []byte) (*User, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT u.id, u.username, u.password_hash, u.webauthn_handle, u.failed_attempts, u.locked_until
		FROM users u JOIN passkeys p ON p.user_id = u.id WHERE p.credential_id = ?`, credentialID)
	return s.scanUserWithCredentials(ctx, row)
}

type scanner interface{ Scan(...any) error }

func (s *Store) scanUserWithCredentials(ctx context.Context, row scanner) (*User, error) {
	u := &User{}
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Handle, &u.Failed, &u.LockedUntil); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT credential_json FROM passkeys WHERE user_id = ? ORDER BY id`, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var credential webauthn.Credential
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &credential); err != nil {
			return nil, fmt.Errorf("decode passkey: %w", err)
		}
		u.Credentials = append(u.Credentials, credential)
	}
	return u, rows.Err()
}

func (s *Store) RecordLoginFailure(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET failed_attempts = failed_attempts + 1,
		locked_until = CASE WHEN failed_attempts + 1 >= 5 THEN datetime('now', '+15 minutes') ELSE locked_until END,
		updated_at = CURRENT_TIMESTAMP WHERE id = ?`, userID)
	return err
}

func (s *Store) ClearLoginFailures(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET failed_attempts = 0, locked_until = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, userID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration, reauthenticated bool) (raw string, session *Session, err error) {
	raw, err = security.RandomToken(32)
	if err != nil {
		return "", nil, err
	}
	csrf, err := security.RandomToken(24)
	if err != nil {
		return "", nil, err
	}
	hash := sha256.Sum256([]byte(raw))
	expires := time.Now().UTC().Add(ttl)
	var reauth any
	if reauthenticated {
		reauth = time.Now().UTC()
	}
	result, err := s.DB.ExecContext(ctx, `INSERT INTO sessions(token_hash, user_id, csrf_token, reauthenticated_at, expires_at) VALUES (?, ?, ?, ?, ?)`, hash[:], userID, csrf, reauth, expires)
	if err != nil {
		return "", nil, err
	}
	id, _ := result.LastInsertId()
	return raw, &Session{ID: id, UserID: userID, CSRFToken: csrf, ExpiresAt: expires}, nil
}

func (s *Store) SessionByToken(ctx context.Context, raw string) (*Session, error) {
	hash := sha256.Sum256([]byte(raw))
	session := &Session{}
	var reauth sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT s.id, s.user_id, u.username, s.csrf_token, s.reauthenticated_at, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ? AND s.expires_at > CURRENT_TIMESTAMP`, hash[:]).
		Scan(&session.ID, &session.UserID, &session.Username, &session.CSRFToken, &reauth, &session.ExpiresAt)
	if reauth.Valid {
		session.ReauthenticatedAt = reauth.Time
	}
	return session, err
}

func (s *Store) DeleteSession(ctx context.Context, raw string) error {
	hash := sha256.Sum256([]byte(raw))
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hash[:])
	return err
}

func (s *Store) MarkReauthenticated(ctx context.Context, sessionID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET reauthenticated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID)
	return err
}

func (s *Store) ChangePassword(ctx context.Context, userID, currentSessionID int64, hash string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, failed_attempts = 0, locked_until = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, hash, userID); err == nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id != ?`, userID, currentSessionID)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) ResetPassword(ctx context.Context, username, hash string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var userID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, username).Scan(&userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, failed_attempts = 0, locked_until = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, hash, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM webauthn_challenges`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(user_id, username, action, resource_type, resource_id, resource_name, success)
		VALUES (?, ?, '通过 CLI 重置密码', 'user', ?, ?, 1)`, userID, username, fmt.Sprint(userID), username); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveChallenge(ctx context.Context, id, kind string, userID *int64, data []byte, expires time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO webauthn_challenges(challenge_id, user_id, kind, session_data, expires_at) VALUES (?, ?, ?, ?, ?)`, id, userID, kind, data, expires)
	return err
}

func (s *Store) ConsumeChallenge(ctx context.Context, id, kind string) ([]byte, *int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	var data []byte
	var userID sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT session_data, user_id FROM webauthn_challenges WHERE challenge_id = ? AND kind = ? AND expires_at > CURRENT_TIMESTAMP`, id, kind).Scan(&data, &userID)
	if err == nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM webauthn_challenges WHERE challenge_id = ?`, id)
	}
	if err != nil {
		tx.Rollback()
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	if userID.Valid {
		return data, &userID.Int64, nil
	}
	return data, nil, nil
}

func (s *Store) CleanupExpiredAuth(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP; DELETE FROM webauthn_challenges WHERE expires_at <= CURRENT_TIMESTAMP`)
	return err
}

func (s *Store) AddPasskey(ctx context.Context, userID int64, name string, credential *webauthn.Credential) (int64, error) {
	raw, err := json.Marshal(credential)
	if err != nil {
		return 0, err
	}
	result, err := s.DB.ExecContext(ctx, `INSERT INTO passkeys(user_id, credential_id, credential_json, name) VALUES (?, ?, ?, ?)`, userID, credential.ID, raw, name)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) UpdatePasskeyUsage(ctx context.Context, userID int64, credential *webauthn.Credential) error {
	raw, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE passkeys SET credential_json = ?, last_used_at = CURRENT_TIMESTAMP WHERE user_id = ? AND credential_id = ?`, raw, userID, credential.ID)
	return err
}

func (s *Store) ListPasskeys(ctx context.Context, userID int64) ([]Passkey, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, created_at, last_used_at FROM passkeys WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Passkey, 0)
	for rows.Next() {
		var item Passkey
		var last sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.CreatedAt, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			item.LastUsedAt = &last.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) RenamePasskey(ctx context.Context, userID, id int64, name string) error {
	result, err := s.DB.ExecContext(ctx, `UPDATE passkeys SET name = ? WHERE id = ? AND user_id = ?`, name, id, userID)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			return sql.ErrNoRows
		}
	}
	return err
}

func (s *Store) DeletePasskey(ctx context.Context, userID, id int64) error {
	result, err := s.DB.ExecContext(ctx, `DELETE FROM passkeys WHERE id = ? AND user_id = ?`, id, userID)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			return sql.ErrNoRows
		}
	}
	return err
}

var ErrNotFound = errors.New("not found")
