#!/usr/bin/env bash
set -Eeuo pipefail

API_URL="${1:?Usage: sudo bash install-agent.sh https://uem.example.com TOKEN /path/to/cyverra-agent}"
ENROLLMENT_TOKEN="${2:?Usage: sudo bash install-agent.sh https://uem.example.com TOKEN /path/to/cyverra-agent}"
AGENT_BINARY="${3:-/usr/local/bin/cyverra-agent}"
STATE_DIR="/var/lib/cyverra"
STATE_FILE="${STATE_DIR}/agent.json"

[[ "${EUID}" -eq 0 ]] || { echo 'Run with sudo.' >&2; exit 1; }
[[ -x "${AGENT_BINARY}" ]] || { echo "Agent binary is not executable: ${AGENT_BINARY}" >&2; exit 1; }
install -d -m 700 "${STATE_DIR}"
if [[ ! -f "${STATE_FILE}" ]]; then
    "${AGENT_BINARY}" --api "${API_URL}" --enrollment-token "${ENROLLMENT_TOKEN}" --state "${STATE_FILE}" --once
fi
install -m 0644 deployments/systemd/cyverra-agent.service /etc/systemd/system/cyverra-agent.service
systemctl daemon-reload
systemctl enable --now cyverra-agent
systemctl --no-pager --full status cyverra-agent
