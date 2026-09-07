-- MeshCentral integration configuration
CREATE TABLE IF NOT EXISTS meshcentral_config (
    id              TEXT PRIMARY KEY DEFAULT 'default',
    server_url      TEXT NOT NULL DEFAULT '',
    api_key         TEXT NOT NULL DEFAULT '',
    agent_group     TEXT NOT NULL DEFAULT '',
    enabled         BOOLEAN NOT NULL DEFAULT false,
    sync_interval   INT NOT NULL DEFAULT 60,
    last_sync_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO meshcentral_config (id) VALUES ('default') ON CONFLICT (id) DO NOTHING;

UPDATE meshcentral_config SET
    server_url = 'https://api.cyverratech.my.id',
    enabled = true,
    sync_interval = 60
WHERE id = 'default' AND server_url = '';

-- MeshCentral devices synced to NEXUS
CREATE TABLE IF NOT EXISTS meshcentral_devices (
    id              TEXT PRIMARY KEY,
    mc_id           TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    host            TEXT NOT NULL DEFAULT '',
    ip              TEXT NOT NULL DEFAULT '',
    domain          TEXT NOT NULL DEFAULT '',
    agent_version   TEXT NOT NULL DEFAULT '',
    last_seen       TIMESTAMPTZ,
    platform        TEXT NOT NULL DEFAULT '',
    architectures   TEXT NOT NULL DEFAULT '',
    state           INT NOT NULL DEFAULT 0,
    tags            TEXT NOT NULL DEFAULT '',
    mesh_id         TEXT NOT NULL DEFAULT '',
    nexus_device_id TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mc_devices_mc_id ON meshcentral_devices(mc_id);
CREATE INDEX IF NOT EXISTS idx_mc_devices_nexus ON meshcentral_devices(nexus_device_id);
