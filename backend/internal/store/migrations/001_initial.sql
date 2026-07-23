CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    webauthn_handle BLOB NOT NULL UNIQUE,
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash BLOB NOT NULL UNIQUE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    reauthenticated_at DATETIME,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX sessions_expires_idx ON sessions(expires_at);

CREATE TABLE passkeys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BLOB NOT NULL UNIQUE,
    credential_json BLOB NOT NULL,
    name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME
);

CREATE TABLE webauthn_challenges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    challenge_id TEXT NOT NULL UNIQUE,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    session_data BLOB NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX webauthn_challenges_expires_idx ON webauthn_challenges(expires_at);

CREATE TABLE api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    encrypted_token BLOB NOT NULL,
    nonce BLOB NOT NULL,
    status TEXT NOT NULL DEFAULT 'valid',
    last_error TEXT NOT NULL DEFAULT '',
    last_verified_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE zones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    api_token_id INTEGER NOT NULL REFERENCES api_tokens(id) ON DELETE CASCADE,
    cloudflare_id TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT '',
    record_count INTEGER NOT NULL DEFAULT 0,
    is_default INTEGER NOT NULL DEFAULT 0,
    last_synced_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(api_token_id, cloudflare_id)
);
CREATE INDEX zones_default_idx ON zones(is_default);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    username TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    resource_name TEXT NOT NULL DEFAULT '',
    before_json TEXT,
    after_json TEXT,
    success INTEGER NOT NULL DEFAULT 1,
    error_message TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX audit_logs_created_idx ON audit_logs(created_at DESC);
