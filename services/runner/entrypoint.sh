#!/bin/bash
set -euo pipefail

PLAYBOOK="${PLAYBOOK:-/playbooks/sample.yml}"
EXTRA_VARS="${EXTRA_VARS:-}"
GIT_URL="${GIT_URL:-}"
GIT_REF="${GIT_REF:-main}"
GIT_TOKEN="${GIT_TOKEN:-}"
GIT_SSH_KEY="${GIT_SSH_KEY:-}"
GIT_SSH_PASSPHRASE="${GIT_SSH_PASSPHRASE:-}"

echo "=== Bosun Runner ==="
echo "Job ID  : ${JOB_ID:-unknown}"

if [[ -n "$GIT_URL" ]]; then
  echo "Source  : git"
  echo "Git URL : $GIT_URL"
  echo "Git Ref : $GIT_REF"
  echo "Playbook: $PLAYBOOK (relative to repo root)"
  echo "=============================="
  echo ""

  # ── Auth setup ──────────────────────────────────────────────────────────────
  if [[ -n "$GIT_SSH_KEY" ]]; then
    echo "$GIT_SSH_KEY" | base64 -d > /tmp/bosun_id_rsa
    chmod 600 /tmp/bosun_id_rsa

    if [[ -n "$GIT_SSH_PASSPHRASE" ]]; then
      eval "$(ssh-agent -s)"
      # Askpass script reads passphrase from a separate env var so special
      # characters in the passphrase don't break shell quoting.
      cat > /tmp/bosun_askpass.sh << 'ASKPASS'
#!/bin/bash
printf '%s' "$BOSUN_SSH_PASSPHRASE"
ASKPASS
      chmod +x /tmp/bosun_askpass.sh
      export BOSUN_SSH_PASSPHRASE="$GIT_SSH_PASSPHRASE"
      SSH_ASKPASS=/tmp/bosun_askpass.sh SSH_ASKPASS_REQUIRE=force ssh-add /tmp/bosun_id_rsa
      export GIT_SSH_COMMAND="ssh -o StrictHostKeyChecking=no"
    else
      export GIT_SSH_COMMAND="ssh -i /tmp/bosun_id_rsa -o StrictHostKeyChecking=no"
    fi

  elif [[ -n "$GIT_TOKEN" ]]; then
    export GIT_TOKEN
    git config --global credential.helper \
      '!f() { echo username=x-access-token; echo "password=$GIT_TOKEN"; }; f'
  fi

  # ── Clone ───────────────────────────────────────────────────────────────────
  # SHA refs (7-40 hex chars) don't work with --branch; fall back to full clone.
  if echo "$GIT_REF" | grep -qE '^[0-9a-f]{7,40}$'; then
    git clone "$GIT_URL" /workspace
    git -C /workspace checkout "$GIT_REF"
  else
    git clone --depth 1 --branch "$GIT_REF" "$GIT_URL" /workspace
  fi
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
