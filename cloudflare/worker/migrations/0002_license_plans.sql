ALTER TABLE licenses ADD COLUMN plan TEXT NOT NULL DEFAULT 'lifetime';
ALTER TABLE licenses ADD COLUMN duration_days INTEGER;
ALTER TABLE licenses ADD COLUMN activated_at TEXT;
ALTER TABLE licenses ADD COLUMN expires_at TEXT;
