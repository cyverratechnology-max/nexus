CREATE TABLE IF NOT EXISTS remote_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'REQUESTED' CHECK (status IN ('REQUESTED','APPROVED','CONNECTING','ACTIVE','CLOSED','EXPIRED','FAILED')),
    protocol TEXT NOT NULL DEFAULT 'WEBRTC',
    stun_url TEXT,
    turn_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '30 minutes'),
    started_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    close_reason TEXT
);
CREATE INDEX IF NOT EXISTS remote_sessions_device_status_idx ON remote_sessions (device_id, status, created_at DESC);