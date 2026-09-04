# Cyverra Nexus UEM/RMM

Cyverra Nexus is being bootstrapped as a multi-tenant UEM, RMM, monitoring, automation, and endpoint-security platform.

## Current vertical slice

- PostgreSQL core schema for organizations, users, enrollment tokens, devices, and audit logs.
- Go API with health, enrollment, heartbeat, device listing, and enrollment-token endpoints.
- Native Go agent for enrollment and heartbeat with local device identity.
- React/Vite dashboard for device visibility and enrollment-token creation.
- Interactive Devices and Monitoring views with inventory details and live metric refresh.
- Disk/SSD, installed software, running service inventory, and audited remote commands.
- Audited remote desktop session lifecycle foundation with WebRTC/STUN/TURN configuration hooks.

## Run locally

1. Copy `.env.example` to `.env` and change `JWT_SECRET`.
2. Start PostgreSQL: `docker compose up -d postgres`.
3. Start the API: `go run ./backend/cmd/api`.
4. Start the dashboard: `cd frontend; npm install; npm run dev`.
5. Build the agent: `go build -o bin/cyverra-agent ./agent/cmd/agent`.

The API listens on `http://localhost:8080`; the dashboard listens on `http://localhost:5173`.

## Update existing Ubuntu installation

Do not rerun `install.sh` for normal releases. From the server, run:

```bash
cd /opt/cyverra-nexus
sudo bash update.sh
```

`update.sh` refuses to continue with uncommitted local changes and prints the files causing the refusal. It creates a timestamped `.env` and PostgreSQL backup, pulls with `git pull --ff-only`, validates Compose, installs the locked frontend dependencies with `npm ci`, builds the dashboard/agents, applies migrations through the API startup, restarts services, and checks `/health`.

## Add a device

1. Sign in to the dashboard.
2. Select **Add device** and generate a one-time enrollment token.
3. Run the native agent on the endpoint with the displayed API URL and token.
4. The device appears after enrollment, then sends heartbeat, inventory, and metrics every 30 seconds.
5. Select the device for inventory details or open **Monitoring** for CPU and memory samples.

The **Devices** view also provides downloads for the native Windows `.exe` and Linux amd64 agent. Downloading the binary is not enrollment: generate a one-time token, then run the command shown by the dashboard. For Windows first run, open PowerShell in the download folder instead of double-clicking the executable; diagnostics are written to `%AppData%\\cyverra\\agent.json.log`.

For background operation, use `deployments/windows/install-agent.ps1` as Administrator on Windows or `deployments/systemd/install-agent.sh` with sudo on Linux. These scripts enroll once and install the agent as an automatic-start service.

Agents check the published version manifest automatically. To release an agent update, change `agentVersion` in `agent/cmd/agent/main.go`, then run `sudo bash update.sh` on the server. The update script rebuilds both binaries, regenerates their SHA-256 manifest, and publishes it with the dashboard.

## Automatic GitHub deployment

The signed webhook receiver is built as `bin/cyverra-webhook`. Install it on Ubuntu with `sudo bash deployments/systemd/install-webhook.sh`. On first run it creates `/etc/cyverra/webhook.env`; fill `GITHUB_WEBHOOK_SECRET` with the same random secret configured in the GitHub repository webhook, then rerun the installer. Configure the webhook URL as `https://your-domain/hooks/github`, content type `application/json`, event `Just the push event`, and enable SSL verification.

## Install on Ubuntu 22.04

Copy the project to the server and run:

```bash
sudo bash install.sh
```

The installer installs Docker Engine/Compose, Nginx, Node.js, Certbot, OpenSSL, UFW rules, PostgreSQL, and the API. Enter a public DNS name to obtain a Let's Encrypt certificate, or press Enter to use `localhost` with a self-signed certificate. Generated application secrets and the administrator password are stored in `/opt/cyverra-nexus/.env` with mode `600`.

## Roadmap

The architecture and phased delivery plan are documented in `docs/ARCHITECTURE.md`. Privileged remote execution, file operations, self-update, and remote desktop remain disabled until their authorization, signing, and audit controls are implemented and tested.
