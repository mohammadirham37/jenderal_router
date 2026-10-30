-- Metadata pesan chat playground (JSON): penalaran model, pemanggilan tool
-- beserta hasilnya — agar riwayat bisa dibangun ulang utuh untuk model.
ALTER TABLE messages ADD COLUMN meta TEXT NOT NULL DEFAULT '';
