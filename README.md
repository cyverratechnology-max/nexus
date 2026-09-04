# Cyverra Nexus UEM/RMM

Cyverra Nexus is being bootstrapped as a multi-tenant UEM, RMM, monitoring, automation, and endpoint-security platform.

## Current vertical slice

- PostgreSQL core schema for organizations, users, enrollment tokens, devices, and audit logs.
- Go API with health, enrollment, heartbeat, device listing, and enrollment-token endpoints.
- Native Go agent for enrollment and heartbeat with local device identity.
- React/Vite dashboard for device visibility and enrollment-token creation.
- Interactive Devices and Monitoring views with inventory details and live metric refresh.
- Disk/SSD, installed software, running service inventory, and audited remote commands.

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

`update.sh` refuses to continue with uncommitted local changes, creates a timestamped `.env` and PostgreSQL backup, pulls with `git pull --ff-only`, validates Compose, builds the dashboard/agents, applies migrations through the API startup, restarts services, and checks `/health`.

## Add a device

1. Sign in to the dashboard.
2. Select **Add device** and generate a one-time enrollment token.
3. Run the native agent on the endpoint with the displayed API URL and token.
4. The device appears after enrollment, then sends heartbeat, inventory, and metrics every 30 seconds.
5. Select the device for inventory details or open **Monitoring** for CPU and memory samples.

## Install on Ubuntu 22.04

Copy the project to the server and run:

```bash
sudo bash install.sh
```

The installer installs Docker Engine/Compose, Nginx, Node.js, Certbot, OpenSSL, UFW rules, PostgreSQL, and the API. Enter a public DNS name to obtain a Let's Encrypt certificate, or press Enter to use `localhost` with a self-signed certificate. Generated application secrets and the administrator password are stored in `/opt/cyverra-nexus/.env` with mode `600`.

## Roadmap

The architecture and phased delivery plan are documented in `docs/ARCHITECTURE.md`. Privileged remote execution, file operations, self-update, and remote desktop remain disabled until their authorization, signing, and audit controls are implemented and tested.
