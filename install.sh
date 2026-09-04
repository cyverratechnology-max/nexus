#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOMAIN="${DOMAIN:-}"
LETSENCRYPT_EMAIL="${LETSENCRYPT_EMAIL:-}"
INSTALL_DIR="/opt/cyverra-nexus"
ENV_FILE="${INSTALL_DIR}/.env"
NGINX_SITE="/etc/nginx/sites-available/cyverra-nexus"

log() { printf '\n[%s] %s\n' "$(date '+%H:%M:%S')" "$*"; }
fail() { printf '\nERROR: %s\n' "$*" >&2; exit 1; }
trap 'fail "Installation gagal pada baris ${LINENO}."' ERR

[[ "${EUID}" -eq 0 ]] || fail "Jalankan dengan sudo: sudo bash install.sh"

read -r -p "Domain dashboard (contoh: uem.example.com, kosong untuk localhost): " DOMAIN_INPUT
DOMAIN="${DOMAIN_INPUT:-${DOMAIN:-localhost}}"
[[ "${DOMAIN}" == "localhost" || "${DOMAIN}" =~ ^[A-Za-z0-9.-]+$ ]] || fail "Domain tidak valid."
if [[ "${DOMAIN}" != "localhost" && -z "${LETSENCRYPT_EMAIL}" ]]; then
    read -r -p "Email Let's Encrypt: " LETSENCRYPT_EMAIL
fi
[[ "${DOMAIN}" == "localhost" || -n "${LETSENCRYPT_EMAIL}" ]] || fail "Email Let's Encrypt wajib diisi untuk domain publik."

log "Memasang dependensi sistem"
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl gnupg nginx openssl certbot python3-certbot-nginx ufw rsync build-essential git

GO_VERSION="${GO_VERSION:-1.22.12}"
if ! command -v go >/dev/null 2>&1 || [[ "$(go version | awk '{print $3}' | cut -d. -f2)" -lt 22 ]]; then
    log "Memasang Go ${GO_VERSION} untuk build agent native"
    case "$(dpkg --print-architecture)" in
        amd64) GO_ARCH="amd64" ;;
        arm64) GO_ARCH="arm64" ;;
        *) fail "Arsitektur $(dpkg --print-architecture) belum didukung oleh installer Go." ;;
    esac
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -o /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    ln -sfn /usr/local/go/bin/go /usr/local/bin/go
    ln -sfn /usr/local/go/bin/gofmt /usr/local/bin/gofmt
fi

if ! command -v docker >/dev/null 2>&1; then
    log "Memasang Docker Engine dan Compose plugin"
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg
    . /etc/os-release
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" > /etc/apt/sources.list.d/docker.list
    apt-get update
    apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    systemctl enable --now docker
fi
docker compose version >/dev/null 2>&1 || fail "Docker Compose plugin tidak tersedia."

log "Menyalin source ke ${INSTALL_DIR}"
mkdir -p "${INSTALL_DIR}"
rsync -a --delete --exclude '.env' --exclude 'frontend/node_modules' --exclude 'frontend/dist' --exclude 'artifacts' "${PROJECT_DIR}/" "${INSTALL_DIR}/"
cd "${INSTALL_DIR}"

if [[ ! -f "${ENV_FILE}" ]]; then
    DB_PASSWORD="$(openssl rand -hex 24)"
    JWT_SECRET="$(openssl rand -hex 48)"
    ADMIN_PASSWORD="$(openssl rand -base64 24 | tr -dc 'A-Za-z0-9' | head -c 20)"
    ADMIN_EMAIL="${ADMIN_EMAIL:-admin@${DOMAIN}}"
    cat > "${ENV_FILE}" <<EOF
DB_PASSWORD=${DB_PASSWORD}
JWT_SECRET=${JWT_SECRET}
ADMIN_EMAIL=${ADMIN_EMAIL}
ADMIN_PASSWORD=${ADMIN_PASSWORD}
CORS_ORIGIN=https://${DOMAIN}
EOF
    chmod 600 "${ENV_FILE}"
else
    log ".env sudah ada, secret tidak diubah"
fi

log "Memasang Node.js 20"
if ! command -v node >/dev/null 2>&1 || [[ "$(node -p 'process.versions.node.split(".")[0]')" -lt 20 ]]; then
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y nodejs
fi
cd "${INSTALL_DIR}/frontend"
if [[ -f package-lock.json ]]; then
    npm ci --no-audit --no-fund
else
    npm install --no-audit --no-fund
fi

log "Membuild agent Linux dan Windows"
cd "${INSTALL_DIR}"
mkdir -p bin
unset GOOS GOARCH
go build -o bin/cyverra-agent-linux-amd64 ./agent/cmd/agent
GOOS=windows GOARCH=amd64 go build -o bin/cyverra-agent-windows-amd64.exe ./agent/cmd/agent
go build -o bin/cyverra-webhook ./deployments/webhook
mkdir -p frontend/public/downloads
cp bin/cyverra-agent-linux-amd64 frontend/public/downloads/cyverra-agent-linux-amd64
cp bin/cyverra-agent-windows-amd64.exe frontend/public/downloads/cyverra-agent-windows-amd64.exe
cp deployments/windows/install-agent.ps1 frontend/public/downloads/install-agent.ps1
cp deployments/systemd/install-agent.sh frontend/public/downloads/install-agent.sh
chmod 0644 frontend/public/downloads/cyverra-agent-*
chmod 0644 frontend/public/downloads/install-agent.*
LINUX_AGENT_SHA256="$(sha256sum bin/cyverra-agent-linux-amd64 | awk '{print $1}')"
WINDOWS_AGENT_SHA256="$(sha256sum bin/cyverra-agent-windows-amd64.exe | awk '{print $1}')"
cat > frontend/public/downloads/agent-manifest.json <<EOF
{"version":"0.2.0","linux_amd64":"/downloads/cyverra-agent-linux-amd64","linux_amd64_sha256":"${LINUX_AGENT_SHA256}","windows_amd64":"/downloads/cyverra-agent-windows-amd64.exe","windows_amd64_sha256":"${WINDOWS_AGENT_SHA256}"}
EOF
chmod 0644 frontend/public/downloads/agent-manifest.json

log "Membuild dashboard"
cd "${INSTALL_DIR}/frontend"
VITE_API_URL="" npm run build

log "Menyiapkan Nginx dan firewall"
mkdir -p /etc/cyverra/ssl
ufw allow OpenSSH >/dev/null 2>&1 || true
ufw allow 80/tcp >/dev/null 2>&1 || true
ufw allow 443/tcp >/dev/null 2>&1 || true

cat > "${NGINX_SITE}" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};
    root ${INSTALL_DIR}/frontend/dist;
    index index.html;
    location /.well-known/acme-challenge/ { allow all; }
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
    location /hooks/github {
        proxy_pass http://127.0.0.1:9090;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }
    location / { try_files \$uri \$uri/ /index.html; }
}
EOF
ln -sfn "${NGINX_SITE}" /etc/nginx/sites-enabled/cyverra-nexus
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl enable --now nginx
systemctl reload nginx

if [[ "${DOMAIN}" != "localhost" ]]; then
    log "Meminta sertifikat Let's Encrypt"
    certbot --nginx --non-interactive --agree-tos --redirect --email "${LETSENCRYPT_EMAIL}" -d "${DOMAIN}"
else
    log "Membuat sertifikat self-signed untuk localhost"
    if [[ ! -f /etc/cyverra/ssl/fullchain.pem ]]; then
        openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
            -keyout /etc/cyverra/ssl/privkey.pem \
            -out /etc/cyverra/ssl/fullchain.pem \
            -subj '/CN=localhost' >/dev/null 2>&1
    fi
    cat > "${NGINX_SITE}" <<EOF
server {
    listen 80;
    server_name localhost;
    return 301 https://\$host\$request_uri;
}
server {
    listen 443 ssl;
    server_name localhost;
    ssl_certificate /etc/cyverra/ssl/fullchain.pem;
    ssl_certificate_key /etc/cyverra/ssl/privkey.pem;
    root ${INSTALL_DIR}/frontend/dist;
    index index.html;
    location /api/ { proxy_pass http://127.0.0.1:8080; proxy_set_header Host \$host; proxy_set_header X-Forwarded-Proto https; }
    location / { try_files \$uri \$uri/ /index.html; }
}
EOF
    nginx -t && systemctl reload nginx
fi

cd "${INSTALL_DIR}"
log "Menjalankan PostgreSQL dan API"
docker compose --env-file "${ENV_FILE}" up -d --build postgres api
docker compose --env-file "${ENV_FILE}" ps

log "Instalasi selesai"
printf '\nDashboard: https://%s\nAPI health: https://%s/health\nProject: %s\n' "${DOMAIN}" "${DOMAIN}" "${INSTALL_DIR}"
printf 'Login email: %s\n' "${ADMIN_EMAIL:-lihat ${ENV_FILE}}"
printf 'Password admin tersimpan di: %s\n' "${ENV_FILE}"