FROM python:3.12-slim

RUN apt-get update && apt-get install -y --no-install-recommends git openssh-client && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir ansible==9.5.1

# Default playbooks bundled in the image.
# Custom EE images just need to follow the same entrypoint contract:
#   ENV: JOB_ID, PLAYBOOK (path inside container), EXTRA_VARS (JSON string)
COPY playbooks/ /playbooks/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
