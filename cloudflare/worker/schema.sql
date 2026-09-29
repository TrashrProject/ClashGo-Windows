CREATE TABLE IF NOT EXISTS licenses (
  id TEXT PRIMARY KEY,
  license_hash TEXT NOT NULL UNIQUE,
  hint TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('member','developer','admin')),
  active INTEGER NOT NULL DEFAULT 1,
  machine_id TEXT,
  created_at TEXT NOT NULL,
  last_seen_at TEXT,
  app_version TEXT,
  plan TEXT NOT NULL DEFAULT 'lifetime',
  duration_days INTEGER,
  activated_at TEXT,
  expires_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_licenses_hash ON licenses(license_hash);
CREATE INDEX IF NOT EXISTS idx_licenses_machine ON licenses(machine_id);
CREATE INDEX IF NOT EXISTS idx_licenses_active ON licenses(active);

CREATE TABLE IF NOT EXISTS incidents (
  id TEXT PRIMARY KEY,
  at TEXT,
  received_at TEXT NOT NULL,
  license_id TEXT NOT NULL,
  license_hint TEXT NOT NULL,
  role TEXT NOT NULL,
  machine_id TEXT,
  app_version TEXT,
  level TEXT NOT NULL,
  message TEXT NOT NULL,
  fields_json TEXT,
  FOREIGN KEY (license_id) REFERENCES licenses(id)
);

CREATE INDEX IF NOT EXISTS idx_incidents_received ON incidents(received_at DESC);
CREATE INDEX IF NOT EXISTS idx_incidents_license ON incidents(license_id);
CREATE INDEX IF NOT EXISTS idx_incidents_level ON incidents(level);
