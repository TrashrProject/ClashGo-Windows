ALTER TABLE customers ADD COLUMN payment_status TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE customers ADD COLUMN total_paid_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE customers ADD COLUMN next_due_at TEXT;

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
