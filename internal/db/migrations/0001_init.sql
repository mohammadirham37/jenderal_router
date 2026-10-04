-- Migrasi awal JenderalRouter (PRD §8.3 + sessions/settings).
-- SQL portabel (SQLite dulu; PostgreSQL menyusul).

CREATE TABLE IF NOT EXISTS users (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    email             TEXT NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL,
    role              TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('super_admin','admin','member','viewer')),
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    default_combo_id  INTEGER REFERENCES combos(id) ON DELETE SET NULL,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS providers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT NOT NULL CHECK (type IN ('openai','anthropic','gemini','openai-compatible','llamastash')),
    name       TEXT NOT NULL UNIQUE,
    prefix     TEXT NOT NULL UNIQUE,
    base_url   TEXT NOT NULL,
    settings   TEXT NOT NULL DEFAULT '{}',   -- JSON: custom headers, ssrf allow, concurrency, dll.
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS credentials (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id    INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    label          TEXT NOT NULL DEFAULT '',
    secret_enc     TEXT NOT NULL,             -- AES-256-GCM
    weight         INTEGER NOT NULL DEFAULT 1,
    status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','invalid','disabled')),
    cooldown_until TEXT NOT NULL DEFAULT '',  -- RFC3339 kosong = siap
    use_count      INTEGER NOT NULL DEFAULT 0,
    fail_count     INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_credentials_provider ON credentials(provider_id);

CREATE TABLE IF NOT EXISTS models (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id    INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    upstream_name  TEXT NOT NULL,
    public_id      TEXT NOT NULL UNIQUE,      -- prefix/nama-model (FR-1.7)
    alias          TEXT NOT NULL DEFAULT '',
    display_name   TEXT NOT NULL DEFAULT '',
    price_in_per_1m  REAL NOT NULL DEFAULT 0,
    price_out_per_1m REAL NOT NULL DEFAULT 0,
    context_window INTEGER NOT NULL DEFAULT 0,
    capabilities   TEXT NOT NULL DEFAULT '{}', -- JSON: {"tools":true,"vision":true}
    enabled        INTEGER NOT NULL DEFAULT 1,
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_models_provider ON models(provider_id);

CREATE TABLE IF NOT EXISTS combos (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS combo_steps (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    combo_id   INTEGER NOT NULL REFERENCES combos(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    model_id   INTEGER NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    timeout_ms INTEGER NOT NULL DEFAULT 120000,
    UNIQUE (combo_id, position)
);

CREATE TABLE IF NOT EXISTS api_keys (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    prefix         TEXT NOT NULL,
    key_hash       TEXT NOT NULL UNIQUE,
    name           TEXT NOT NULL DEFAULT '',
    allowed_models TEXT NOT NULL DEFAULT '*',  -- '*' atau daftar public_id/alias/combo dipisah koma
    ip_allowlist   TEXT NOT NULL DEFAULT '',   -- CIDR dipisah koma; kosong = semua
    rpm            INTEGER NOT NULL DEFAULT 0, -- 0 = tanpa batas
    tpm            INTEGER NOT NULL DEFAULT 0,
    expires_at     TEXT NOT NULL DEFAULT '',
    revoked_at     TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);

CREATE TABLE IF NOT EXISTS quotas (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    scope          TEXT NOT NULL CHECK (scope IN ('user','key')),
    scope_id       INTEGER NOT NULL,
    period         TEXT NOT NULL CHECK (period IN ('day','month')),
    token_limit    INTEGER NOT NULL DEFAULT 0, -- 0 = tak terbatas
    request_limit  INTEGER NOT NULL DEFAULT 0,
    cost_limit_usd REAL NOT NULL DEFAULT 0,
    used_tokens    INTEGER NOT NULL DEFAULT 0,
    used_requests  INTEGER NOT NULL DEFAULT 0,
    used_cost      REAL NOT NULL DEFAULT 0,
    reset_at       TEXT NOT NULL DEFAULT '',
    UNIQUE (scope, scope_id, period)
);
CREATE INDEX IF NOT EXISTS idx_quotas_scope ON quotas(scope, scope_id);

CREATE TABLE IF NOT EXISTS request_logs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    ts              TEXT NOT NULL DEFAULT (datetime('now')),
    user_id         INTEGER,
    key_id          INTEGER,
    requested_model TEXT NOT NULL DEFAULT '',
    provider_id     INTEGER,
    model_id        INTEGER,
    provider_name   TEXT NOT NULL DEFAULT '',
    model_name      TEXT NOT NULL DEFAULT '',
    status          INTEGER NOT NULL DEFAULT 0,
    error_code      TEXT NOT NULL DEFAULT '',
    latency_ms      INTEGER NOT NULL DEFAULT 0,
    ttft_ms         INTEGER NOT NULL DEFAULT 0,
    tokens_in       INTEGER NOT NULL DEFAULT 0,
    tokens_out      INTEGER NOT NULL DEFAULT 0,
    tokens_estimated INTEGER NOT NULL DEFAULT 0,
    cost_usd        REAL NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 1,
    stream          INTEGER NOT NULL DEFAULT 0,
    ip              TEXT NOT NULL DEFAULT '',
    -- NFR-14: isi prompt tidak disimpan default; kolom ini kosong kecuali JR_LOG_PROMPTS
    content_debug   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_request_logs_ts ON request_logs(ts);
CREATE INDEX IF NOT EXISTS idx_request_logs_user ON request_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_provider ON request_logs(provider_id);

CREATE TABLE IF NOT EXISTS conversations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL DEFAULT 'Percakapan baru',
    model      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_conversations_user ON conversations(user_id);

CREATE TABLE IF NOT EXISTS messages (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('user','assistant','system')),
    content         TEXT NOT NULL DEFAULT '',
    model           TEXT NOT NULL DEFAULT '',
    provider_name   TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id);

CREATE TABLE IF NOT EXISTS audit_logs (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    ts        TEXT NOT NULL DEFAULT (datetime('now')),
    actor_id  INTEGER,
    action    TEXT NOT NULL,
    target    TEXT NOT NULL DEFAULT '',
    before    TEXT NOT NULL DEFAULT '{}',
    after     TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_ts ON audit_logs(ts);

CREATE TABLE IF NOT EXISTS sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash TEXT NOT NULL UNIQUE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
