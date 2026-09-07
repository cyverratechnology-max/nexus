ALTER TABLE remote_sessions ADD COLUMN IF NOT EXISTS vnc_port INTEGER;
ALTER TABLE remote_sessions ADD COLUMN IF NOT EXISTS vnc_password TEXT;
ALTER TABLE remote_sessions ADD COLUMN IF NOT EXISTS rdp_port INTEGER DEFAULT 3389;
ALTER TABLE remote_sessions ADD COLUMN IF NOT EXISTS rdp_username TEXT;
ALTER TABLE remote_sessions ADD COLUMN IF NOT EXISTS rdp_password TEXT;
ALTER TABLE remote_sessions ADD COLUMN IF NOT EXISTS connection_url TEXT;
ALTER TABLE remote_sessions ADD COLUMN IF NOT EXISTS relay_token TEXT;

UPDATE remote_sessions SET protocol = 'WEBRTC' WHERE protocol NOT IN ('WEBRTC', 'VNC', 'RDP');
