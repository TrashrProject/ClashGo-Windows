CREATE TABLE IF NOT EXISTS customers (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  contact TEXT,
  notes TEXT,
  payment_status TEXT NOT NULL DEFAULT 'unknown',
  total_paid_cents INTEGER NOT NULL DEFAULT 0,
  next_due_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_customers_name ON customers(display_name);
CREATE INDEX IF NOT EXISTS idx_customers_contact ON customers(contact);

CREATE TABLE IF NOT EXISTS licenses (
  id TEXT PRIMARY KEY,
  customer_id TEXT,
  license_hash TEXT NOT NULL UNIQUE,
  hint TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('member','developer','admin')),
  active INTEGER NOT NULL DEFAULT 1,
  machine_id TEXT,
  machine_name TEXT,
  created_at TEXT NOT NULL,
  last_seen_at TEXT,
  app_version TEXT,
  plan TEXT NOT NULL DEFAULT 'lifetime',
  duration_days INTEGER,
  activated_at TEXT,
  expires_at TEXT,
  denied_activations INTEGER NOT NULL DEFAULT 0,
  last_denied_at TEXT,
  last_denied_machine TEXT,
  FOREIGN KEY (customer_id) REFERENCES customers(id)
);

CREATE INDEX IF NOT EXISTS idx_licenses_hash ON licenses(license_hash);
CREATE INDEX IF NOT EXISTS idx_licenses_customer ON licenses(customer_id);
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


CREATE TABLE IF NOT EXISTS license_events (
  id TEXT PRIMARY KEY,
  license_id TEXT NOT NULL,
  customer_id TEXT,
  event_type TEXT NOT NULL,
  plan TEXT,
  amount_cents INTEGER,
  payment_status TEXT,
  note TEXT,
  created_at TEXT NOT NULL,
  expires_at TEXT,
  FOREIGN KEY (license_id) REFERENCES licenses(id),
  FOREIGN KEY (customer_id) REFERENCES customers(id)
);

CREATE INDEX IF NOT EXISTS idx_license_events_license ON license_events(license_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_license_events_customer ON license_events(customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_license_events_type ON license_events(event_type);
