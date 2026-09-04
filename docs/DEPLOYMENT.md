# Deployment

For local development, copy `.env.example` to `.env`, set a strong `JWT_SECRET` and `ADMIN_PASSWORD`, then run `docker compose up -d postgres` and start the API and frontend as described in the root README.

For a single Ubuntu 22.04 server, use `sudo bash install.sh`. The installer copies the application to `/opt/cyverra-nexus`, builds the Vite dashboard, binds the API only to `127.0.0.1:8080`, serves the dashboard through Nginx, and provisions either Let's Encrypt or a local self-signed certificate. DNS for a public domain must already point to the server and inbound ports 80/443 must be reachable for Let's Encrypt validation.

The Compose API image expects migration files at `backend/migrations` during its build/runtime layout. Put TLS termination, a WAF, and a reverse proxy in front of the API before exposing it. Use managed PostgreSQL, secret injection, backups, TLS, and separate object/metrics storage for production.

## Ubuntu 22.04 agent

Install the Linux binary at `/usr/local/bin/cyverra-agent`, create `/etc/cyverra/agent.env` with `CYVERRA_API_URL=https://your-api-host` and a single-use `CYVERRA_ENROLLMENT_TOKEN`, restrict that file to root (`chmod 600`), copy `deployments/systemd/cyverra-agent.service` to `/etc/systemd/system/`, then run `systemctl daemon-reload && systemctl enable --now cyverra-agent`. The unit stores its credential under `/var/lib/cyverra` and restarts after transient failures.
