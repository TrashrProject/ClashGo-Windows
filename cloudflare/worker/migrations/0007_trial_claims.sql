CREATE TABLE IF NOT EXISTS trial_claims (
  machine_id TEXT PRIMARY KEY,
  license_id TEXT NOT NULL,
  claimed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trial_claims_license ON trial_claims(license_id);
