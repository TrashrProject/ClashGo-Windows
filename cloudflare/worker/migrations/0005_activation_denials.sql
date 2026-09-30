ALTER TABLE licenses ADD COLUMN denied_activations INTEGER NOT NULL DEFAULT 0;
ALTER TABLE licenses ADD COLUMN last_denied_at TEXT;
ALTER TABLE licenses ADD COLUMN last_denied_machine TEXT;
