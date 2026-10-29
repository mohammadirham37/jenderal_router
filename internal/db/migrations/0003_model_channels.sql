-- Ketersediaan model per kanal: chat dashboard dan API (/v1).
-- Nilai awal mengikuti kolom enabled agar perilaku model lama tidak berubah.
ALTER TABLE models ADD COLUMN chat_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE models ADD COLUMN api_enabled  INTEGER NOT NULL DEFAULT 1;
UPDATE models SET chat_enabled = enabled, api_enabled = enabled;
