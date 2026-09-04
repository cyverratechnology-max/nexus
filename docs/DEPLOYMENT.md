# Deployment

For local development, copy `.env.example` to `.env`, set a strong `JWT_SECRET` and `ADMIN_PASSWORD`, then run `docker compose up -d postgres` and start the API and frontend as described in the root README.

For a single Ubuntu 22.04 server, use `sudo bash install.sh`. The installer copies the application to `/opt/cyverra-nexus`, builds the Vite dashboard, binds the API only to `127.0.0.1:8080`, serves the dashboard through Nginx, and provisions either Let's Encrypt or a local self-signed certificate. DNS for a public domain must already point to the server and inbound ports 80/443 must be reachable for Let's Encrypt validation.

## Updating an existing installation

Use `update.sh`, not `install.sh`, for GitHub releases:

```bash
cd /opt/cyverra-nexus
sudo bash update.sh
```

The update script does not recreate the PostgreSQL volume or overwrite `/etc/nginx` certificate configuration. It makes backups under `/var/backups/cyverra-nexus/<timestamp>`, preserves `.env`, rejects dirty Git worktrees, uses fast-forward-only pulls, and runs the existing migrations when the API starts. Review the migration and backup before production updates. Additive migrations are expected; destructive schema changes require a separately tested migration and rollback plan.

The update also builds the native agents and publishes them at `/downloads/cyverra-agent-windows-amd64.exe` and `/downloads/cyverra-agent-linux-amd64`. These binaries are generated deployment artifacts and are ignored by Git. Users still need a one-time enrollment token from the dashboard.

## GitHub automatic deployment

Build and install the webhook receiver with `sudo bash deployments/systemd/install-webhook.sh`. It listens only on `127.0.0.1:9090`, validates GitHub HMAC `X-Hub-Signature-256`, accepts only push events for `main`, prevents concurrent deployments, and runs `update.sh` with a 45-minute timeout. Fill `/etc/cyverra/webhook.env` yourself; no GitHub credential is committed to the repository. Set the GitHub webhook URL to `https://your-domain/hooks/github` and use the same `GITHUB_WEBHOOK_SECRET`.

Agent auto-update is checksum-verified. A release changes `agentVersion`, then `update.sh` rebuilds the binaries and writes `agent-manifest.json`. Existing agents check that manifest every 30 seconds. Keep the download paths behind HTTPS; do not replace the manifest or binaries manually without regenerating SHA-256 values.

The Compose API image expects migration files at `backend/migrations` during its build/runtime layout. Put TLS termination, a WAF, and a reverse proxy in front of the API before exposing it. Use managed PostgreSQL, secret injection, backups, TLS, and separate object/metrics storage for production.

## Ubuntu 22.04 agent

Install the Linux binary at `/usr/local/bin/cyverra-agent`, then run `sudo bash deployments/systemd/install-agent.sh https://your-api-host ONE_TIME_TOKEN /usr/local/bin/cyverra-agent`. The script enrolls once, stores the credential under `/var/lib/cyverra`, and installs/enables the systemd service. The token is not kept in the service unit. The service restarts after transient failures.

## Windows background service

Open PowerShell as Administrator in the folder containing the downloaded `.exe` and `install-agent.ps1`, then run `Set-ExecutionPolicy -Scope Process Bypass` followed by `./install-agent.ps1 -ApiUrl https://your-api-host -EnrollmentToken ONE_TIME_TOKEN -AgentPath ./cyverra-agent-windows-amd64.exe`. The script enrolls once and installs the `CyverraNexusAgent` Windows service with automatic startup. Logs are stored under `C:\ProgramData\Cyverra\Agent\agent.json.log`.
