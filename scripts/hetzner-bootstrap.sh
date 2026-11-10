#!/bin/bash
# Secret-free cloud-init bootstrap, run as root on Ubuntu 24.04.
# Pin and verify a Linux hev binary in a snapshot or extend this script with
# your release download/checksum before provisioning. Do not put tokens here.
set -euo pipefail
# Override this in your secret-free bootstrap copy with a reviewed commit.
KIT_REF=main
if ! command -v hev >/dev/null; then
  apt-get update
  apt-get install -y golang-go npm
  GOBIN=/usr/local/bin go install github.com/hev/kit/cmd/hev@"$KIT_REF"
fi
if ! command -v codex >/dev/null && ! command -v claude >/dev/null; then
  apt-get update
  apt-get install -y npm
  npm install -g @openai/codex
fi
/usr/local/bin/hev dispatch --help >/dev/null
install -d -o user -g user /home/user/.hev /home/user/.config/hev /home/user/dispatch
cat > /home/user/.hev/config.toml <<'CONFIG'
[layer]
endpoint = "https://gcp-us-central1.turbopuffer.com"
[capture]
scan_interval = "5s"
CONFIG
chown user:user /home/user/.hev/config.toml
cat > /etc/systemd/system/hevd.service <<'SERVICE'
[Unit]
Description=hev trace capture
After=network-online.target
ConditionPathExists=/run/hev-capture.env
[Service]
User=user
Environment=HOME=/home/user
EnvironmentFile=/run/hev-capture.env
ExecStart=/usr/local/bin/hev daemon
Restart=on-failure
[Install]
WantedBy=multi-user.target
SERVICE
# This separate file is supplied by the operator through pinned SSH or a secret
# manager. It is never part of cloud-init, records, or command-line arguments.
cat > /etc/systemd/system/hev-cloud-recovery.service <<'SERVICE'
[Unit]
Description=Recover dispatch cloud queues and terminal leftovers
After=network-online.target
ConditionPathExists=/home/user/.config/hev/fleet.toml
[Service]
User=user
Environment=HOME=/home/user
EnvironmentFile=-/etc/hev/fleet.env
ExecStart=/usr/local/bin/hev dispatch cloud-recover --watch --config /home/user/.config/hev/fleet.toml
Restart=always
RestartSec=30
StandardOutput=null
StandardError=null
[Install]
WantedBy=multi-user.target
SERVICE
systemctl daemon-reload
systemctl enable hevd.service hev-cloud-recovery.service
