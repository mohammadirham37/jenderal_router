-- Pencatatan isi request API (opt-in per user / per API key; saklar global
-- di settings "log_content"). Isi lengkap disimpan di request_logs.content_debug
-- dan dikosongkan otomatis setelah 7 hari.
ALTER TABLE users ADD COLUMN log_content INTEGER NOT NULL DEFAULT 0;
ALTER TABLE api_keys ADD COLUMN log_content INTEGER NOT NULL DEFAULT 0;
-- indeks parsial untuk job pengosongan: hanya baris yang masih berisi
CREATE INDEX IF NOT EXISTS idx_request_logs_content_ts ON request_logs(ts) WHERE content_debug != '';
