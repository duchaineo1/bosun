#!/bin/bash
set -euo pipefail

PLAYBOOK="${PLAYBOOK:-/playbooks/sample.yml}"
EXTRA_VARS="${EXTRA_VARS:-}"
GIT_URL="${GIT_URL:-}"
GIT_REF="${GIT_REF:-main}"

echo "=== Bosun Runner ==="
echo "Job ID  : ${JOB_ID:-unknown}"

if [[ -n "$GIT_URL" ]]; then
  echo "Source  : git"
  echo "Git URL : $GIT_URL"
  echo "Git Ref : $GIT_REF"
  echo "Playbook: $PLAYBOOK (relative to repo root)"
  echo "=============================="
  echo ""
  git clone --depth 1 --branch "$GIT_REF" "$GIT_URL" /workspace
  cd /workspace
else
  echo "Source  : image"
  echo "Playbook: $PLAYBOOK"
  echo "=============================="
  echo ""
fi

if [[ -n "$EXTRA_VARS" && "$EXTRA_VARS" != "{}" && "$EXTRA_VARS" != "null" ]]; then
    ansible-playbook "$PLAYBOOK" --extra-vars "$EXTRA_VARS"
else
    ansible-playbook "$PLAYBOOK"
fi
