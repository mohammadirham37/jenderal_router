-- Kuota default: baris kuota user bersumber 'default' disinkronkan dari
-- setting "default_quota" (berlaku untuk member/viewer lama & baru);
-- 'custom' = diatur khusus per user/key oleh admin dan tidak ditimpa.
ALTER TABLE quotas ADD COLUMN source TEXT NOT NULL DEFAULT 'custom';
