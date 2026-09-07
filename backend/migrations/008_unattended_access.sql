ALTER TABLE devices ADD COLUMN IF NOT EXISTS unattended_token_hash TEXT;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS unattended_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS unattended_port INTEGER DEFAULT 5900;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS unattended_protocol TEXT DEFAULT 'VNC' CHECK (unattended_protocol IN ('VNC', 'RDP'));
ALTER TABLE devices ADD COLUMN IF NOT EXISTS unattended_last_used TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS devices_unattended_token_hash_idx ON devices (unattended_token_hash) WHERE unattended_token_hash IS NOT NULL;

CREATE TABLE IF NOT EXISTS unattended_access_logs (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    ip_address INET,
    token_hash TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS unattended_logs_org_idx ON unattended_access_logs (organization_id, created_at DESC);
