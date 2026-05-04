#!/bin/bash
set -euo pipefail

PLAYBOOK="${PLAYBOOK:-/playbooks/sample.yml}"
EXTRA_VARS="${EXTRA_VARS:-}"

echo "=== Bosun Runner ==="
echo "Job ID  : ${JOB_ID:-unknown}"
echo "Playbook: ${PLAYBOOK}"
echo "Image   : $(cat /etc/hostname 2>/dev/null || echo unknown)"
echo "=============================="
echo ""

if [[ -n "$EXTRA_VARS" && "$EXTRA_VARS" != "{}" && "$EXTRA_VARS" != "null" ]]; then
    ansible-playbook "$PLAYBOOK" --extra-vars "$EXTRA_VARS"
else
    ansible-playbook "$PLAYBOOK"
fi
