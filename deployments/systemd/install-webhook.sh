#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/cyverra-nexus}"
SECRET_FILE="/etc/cyverra/webhook.env"
[[ "${EUID}" -eq 0 ]] || { echo 'Run with sudo.' >&2; exit 1; }
if [[ ! -x "${APP_DIR}/bin/cyverra-webhook" ]]; then
    command -v go >/dev/null 2>&1 || { echo "Missing ${APP_DIR}/bin/cyverra-webhook and Go is not installed." >&2; exit 1; }
    install -d -m 755 "${APP_DIR}/bin"
    (cd "${APP_DIR}/deployments/webhook" && go build -o "${APP_DIR}/bin/cyverra-webhook" .)
    chmod 0755 "${APP_DIR}/bin/cyverra-webhook"
fi
if [[ ! -f "${SECRET_FILE}" ]]; then
    install -d -m 700 /etc/cyverra
    cat > "${SECRET_FILE}" <<'EOF'
GITHUB_WEBHOOK_SECRET=REPLACE_WITH_A_RANDOM_SECRET
APP_DIR=/opt/cyverra-nexus
UPDATE_SCRIPT=/opt/cyverra-nexus/update.sh
WEBHOOK_ADDR=127.0.0.1:9090
EOF
    chmod 600 "${SECRET_FILE}"
    echo "Edit ${SECRET_FILE}, replace GITHUB_WEBHOOK_SECRET, then rerun this script."
    exit 0
fi
grep -q '^GITHUB_WEBHOOK_SECRET=' "${SECRET_FILE}" || { echo 'GITHUB_WEBHOOK_SECRET is missing.' >&2; exit 1; }
secret="$(sed -n 's/^GITHUB_WEBHOOK_SECRET=//p' "${SECRET_FILE}")"
[[ "${secret}" != 'REPLACE_WITH_A_RANDOM_SECRET' && ${#secret} -ge 16 ]] || { echo 'Set a random webhook secret of at least 16 characters.' >&2; exit 1; }
install -m 0644 "${APP_DIR}/deployments/systemd/cyverra-webhook.service" /etc/systemd/system/cyverra-webhook.service
systemctl daemon-reload
systemctl enable --now cyverra-webhook
NGINX_SITE=/etc/nginx/sites-available/cyverra-nexus
if [[ -f "${NGINX_SITE}" ]] && ! grep -q 'location /hooks/github' "${NGINX_SITE}"; then
    sed -i '/^[[:space:]]*location \/ {/i\    location /hooks/github {\n        proxy_pass http://127.0.0.1:9090;\n        proxy_set_header Host $host;\n        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n    }' "${NGINX_SITE}"
fi
nginx -t
systemctl reload nginx
systemctl --no-pager --full status cyverra-webhook
