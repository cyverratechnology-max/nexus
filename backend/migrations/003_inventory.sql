CREATE TABLE IF NOT EXISTS device_inventory (
    device_id UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    payload JSONB NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS device_metrics (
    id BIGSERIAL PRIMARY KEY,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    cpu_percent DOUBLE PRECISION,
    memory_used_bytes BIGINT,
    memory_total_bytes BIGINT,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS device_metrics_device_time_idx ON device_metrics (device_id, collected_at DESC);