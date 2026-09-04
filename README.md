# Cyverra Nexus UEM/RMM

Cyverra Nexus is being bootstrapped as a multi-tenant UEM, RMM, monitoring, automation, and endpoint-security platform.

## Current vertical slice

- PostgreSQL core schema for organizations, users, enrollment tokens, devices, and audit logs.
- Go API with health, enrollment, heartbeat, device listing, and enrollment-token endpoints.
- Native Go agent for enrollment and heartbeat with local device identity.
- React/Vite dashboard for device visibility and enrollment-token creation.

## Run locally

1. Copy `.env.example` to `.env` and change `JWT_SECRET`.
2. Start PostgreSQL: `docker compose up -d postgres`.
3. Start the API: `go run ./backend/cmd/api`.
4. Start the dashboard: `cd frontend; npm install; npm run dev`.
5. Build the agent: `go build -o bin/cyverra-agent ./agent/cmd/agent`.

The API listens on `http://localhost:8080`; the dashboard listens on `http://localhost:5173`.

## Install on Ubuntu 22.04

Copy the project to the server and run:

```bash
sudo bash install.sh
```

The installer installs Docker Engine/Compose, Nginx, Node.js, Certbot, OpenSSL, UFW rules, PostgreSQL, and the API. Enter a public DNS name to obtain a Let's Encrypt certificate, or press Enter to use `localhost` with a self-signed certificate. Generated application secrets and the administrator password are stored in `/opt/cyverra-nexus/.env` with mode `600`.

## Roadmap

The architecture and phased delivery plan are documented in `docs/ARCHITECTURE.md`. Privileged remote execution, file operations, self-update, and remote desktop remain disabled until their authorization, signing, and audit controls are implemented and tested.
