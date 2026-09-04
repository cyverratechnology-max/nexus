#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/cyverra-nexus}"
BACKUP_ROOT="${BACKUP_ROOT:-/var/backups/cyverra-nexus}"
ENV_FILE="${APP_DIR}/.env"
COMPOSE=(docker compose --env-file "${ENV_FILE}")
STAMP="$(date -u '+%Y%m%dT%H%M%SZ')"
BACKUP_DIR="${BACKUP_ROOT}/${STAMP}"

log() { printf '\n[%s] %s\n' "$(date '+%H:%M:%S')" "$*"; }
fail() { printf '\nERROR: %s\n' "$*" >&2; exit 1; }
trap 'fail "Update gagal pada baris ${LINENO}. Backup tetap tersedia di ${BACKUP_DIR}."' ERR

[[ "${EUID}" -eq 0 ]] || fail "Jalankan dengan sudo: sudo bash update.sh"
[[ -d "${APP_DIR}" ]] || fail "Direktori aplikasi tidak ditemukan: ${APP_DIR}"
[[ -f "${ENV_FILE}" ]] || fail "File secret tidak ditemukan: ${ENV_FILE}"
[[ -f "${APP_DIR}/docker-compose.yml" ]] || fail "docker-compose.yml tidak ditemukan."
command -v docker >/dev/null 2>&1 || fail "Docker belum terpasang."
cd "${APP_DIR}"

mkdir -p "${BACKUP_DIR}"
chmod 700 "${BACKUP_ROOT}" "${BACKUP_DIR}"
cp -p "${ENV_FILE}" "${BACKUP_DIR}/.env"

log "Memeriksa repository"
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "${APP_DIR} bukan Git repository."
if [[ -n "$(git status --porcelain)" ]]; then
    log "File yang membuat working tree kotor:"
    git status --short --untracked-files=all
    fail "Working tree memiliki perubahan lokal. Commit/stash/restore file tersebut dahulu agar update tidak menimpa perubahan."
fi
OLD_COMMIT="$(git rev-parse HEAD)"
printf '%s\n' "${OLD_COMMIT}" > "${BACKUP_DIR}/previous-commit"

log "Memeriksa status database"
if "${COMPOSE[@]}" ps --status running postgres >/dev/null 2>&1; then
    "${COMPOSE[@]}" exec -T postgres pg_dump -U cyverra -d cyverra --format=custom > "${BACKUP_DIR}/postgres.dump"
else
    log "PostgreSQL belum running; backup database dilewati"
fi

log "Mengambil perubahan GitHub"
git pull --ff-only
NEW_COMMIT="$(git rev-parse HEAD)"
printf '%s\n' "${NEW_COMMIT}" > "${BACKUP_DIR}/new-commit"

log "Memvalidasi konfigurasi Compose"
"${COMPOSE[@]}" config --quiet

if [[ -d "frontend" && -f "frontend/package.json" ]]; then
    log "Memasang dependency dashboard"
    (cd frontend && npm ci --no-audit --no-fund)
fi

if command -v go >/dev/null 2>&1; then
    log "Membuild agent Linux dan Windows"
    mkdir -p bin
    unset GOOS GOARCH
    go build -o bin/cyverra-agent-linux-amd64 ./agent/cmd/agent
    GOOS=windows GOARCH=amd64 go build -o bin/cyverra-agent-windows-amd64.exe ./agent/cmd/agent
    mkdir -p frontend/public/downloads
    cp bin/cyverra-agent-linux-amd64 frontend/public/downloads/cyverra-agent-linux-amd64
    cp bin/cyverra-agent-windows-amd64.exe frontend/public/downloads/cyverra-agent-windows-amd64.exe
    chmod 0644 frontend/public/downloads/cyverra-agent-*
    LINUX_AGENT_SHA256="$(sha256sum bin/cyverra-agent-linux-amd64 | awk '{print $1}')"
    WINDOWS_AGENT_SHA256="$(sha256sum bin/cyverra-agent-windows-amd64.exe | awk '{print $1}')"
    cat > frontend/public/downloads/agent-manifest.json <<EOF
{"version":"0.2.0","linux_amd64":"/downloads/cyverra-agent-linux-amd64","linux_amd64_sha256":"${LINUX_AGENT_SHA256}","windows_amd64":"/downloads/cyverra-agent-windows-amd64.exe","windows_amd64_sha256":"${WINDOWS_AGENT_SHA256}"}
EOF
    chmod 0644 frontend/public/downloads/agent-manifest.json
fi

log "Membuild dashboard"
(cd frontend && VITE_API_URL="" npm run build)

log "Menerapkan migration dan restart API"
"${COMPOSE[@]}" up -d --build postgres api

log "Memastikan API sehat"
for attempt in 1 2 3 4 5 6 7 8 9 10; do
    if curl --fail --silent --show-error http://127.0.0.1:8080/health >/dev/null; then
        log "API health check berhasil"
        break
    fi
    if [[ "${attempt}" -eq 10 ]]; then
        fail "API tidak sehat setelah update. Lihat: ${COMPOSE[*]} logs api"
    fi
    sleep 3
done

if command -v nginx >/dev/null 2>&1; then
    nginx -t
    systemctl reload nginx
fi

log "Update selesai"
printf 'Commit sebelumnya: %s\nCommit aktif: %s\nBackup: %s\n' "${OLD_COMMIT}" "${NEW_COMMIT}" "${BACKUP_DIR}"
printf 'Untuk rollback source: cd %s && git reset --hard %s\n' "${APP_DIR}" "${OLD_COMMIT}"
printf 'Untuk rollback service setelah source rollback: docker compose --env-file %s up -d --build postgres api\n' "${ENV_FILE}"
