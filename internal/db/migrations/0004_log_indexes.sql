-- Indeks untuk query analitik per user (detail user, top users, /usage/me)
-- dan filter model (Ringkasan → filter per model).
CREATE INDEX IF NOT EXISTS idx_request_logs_user_ts ON request_logs(user_id, ts);
CREATE INDEX IF NOT EXISTS idx_request_logs_ts_model ON request_logs(ts, requested_model);
