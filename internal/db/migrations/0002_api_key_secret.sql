-- Salinan terenkripsi plaintext API key — agar admin bisa menyalin ulang
-- key dari dashboard tanpa menyimpannya sebagai teks biasa.
-- Verifikasi auth tetap memakai key_hash (kolom ini tidak dipakai auth).
ALTER TABLE api_keys ADD COLUMN secret_enc TEXT NOT NULL DEFAULT '';
