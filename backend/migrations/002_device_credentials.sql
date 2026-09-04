ALTER TABLE devices ADD COLUMN IF NOT EXISTS credential_hash TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS devices_credential_hash_idx ON devices (credential_hash) WHERE credential_hash IS NOT NULL;
