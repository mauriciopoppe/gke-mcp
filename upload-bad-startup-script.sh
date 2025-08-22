#!/bin/bash

set -euxo pipefail

# Uploads a bad startup script to make a node fail.

cat >bad-startup-script.sh <<'EOF'
#! /bin/bash

while true; do
  if [[ -f /etc/kubernetes/kubelet-config.yaml ]]; then
    echo "[mauricio] moving kubelet config file!"
    mv /etc/kubernetes/kubelet-config.yaml /etc/kubernetes/kubelet-config.yaml.backup || true
  fi
  sleep 1
done

EOF

gcloud storage cp ./bad-startup-script.sh gs://$USER-gke-dev/node-hackathon-2025/bad-startup-script.sh
