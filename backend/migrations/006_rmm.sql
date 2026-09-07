ALTER TABLE commands ADD COLUMN IF NOT EXISTS command_type TEXT NOT NULL DEFAULT 'execute_command'
    CHECK (command_type IN (
        'execute_command',
        'execute_script',
        'powershell',
        'cmd',
        'bash',
        'kill_process',
        'start_process',
        'restart_service',
        'stop_service',
        'start_service',
        'reboot',
        'shutdown',
        'lock_workstation',
        'logoff_user',
        'retrieve_file',
        'upload_file',
        'delete_file',
        'rename_file',
        'browse_filesystem'
    ));
ALTER TABLE commands ADD COLUMN IF NOT EXISTS params JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE commands ADD COLUMN IF NOT EXISTS file_content BYTEA;
ALTER TABLE commands ADD COLUMN IF NOT EXISTS file_name TEXT;
ALTER TABLE commands ADD COLUMN IF NOT EXISTS file_path TEXT;
ALTER TABLE commands ADD COLUMN IF NOT EXISTS file_size BIGINT;

CREATE INDEX IF NOT EXISTS commands_type_status_idx ON commands (command_type, status, created_at DESC);
