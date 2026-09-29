CREATE TABLE IF NOT EXISTS customers (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  contact TEXT,
  notes TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_customers_name ON customers(display_name);
CREATE INDEX IF NOT EXISTS idx_customers_contact ON customers(contact);

ALTER TABLE licenses ADD COLUMN customer_id TEXT;
CREATE INDEX IF NOT EXISTS idx_licenses_customer ON licenses(customer_id);
