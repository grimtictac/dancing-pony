#!/bin/bash
# One-time set-up of Grafana Alloy on the droplet, run as root from a copy of
# the deploy directory:
#
#   scp -r deploy root@droplet:/root/pony-deploy-files
#   ssh root@droplet bash /root/pony-deploy-files/alloy/setup.sh
#
# Write /etc/alloy/cloud.env first (see cloud.env.example). Safe to re-run:
# it updates the config and restarts Alloy.
set -euo pipefail
cd "$(dirname "$0")"

[[ -f /etc/alloy/cloud.env ]] || {
    echo "write /etc/alloy/cloud.env first (see cloud.env.example)"; exit 1; }

if ! command -v alloy >/dev/null; then
    install -d -m 755 /etc/apt/keyrings
    curl -fsSL https://apt.grafana.com/gpg.key | gpg --dearmor --yes -o /etc/apt/keyrings/grafana.gpg
    echo "deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com stable main" \
        > /etc/apt/sources.list.d/grafana.list
    apt-get update -q
    apt-get install -y -q alloy
fi

# Reading the journal needs systemd-journal; Nginx's error log needs adm.
usermod -aG systemd-journal,adm alloy

install -o root -g alloy -m 640 config.alloy /etc/alloy/config.alloy
chown root:alloy /etc/alloy/cloud.env
chmod 640 /etc/alloy/cloud.env

# The package's default listens on all interfaces; keep Alloy's own UI local
# (reach it with ssh -L 12345:127.0.0.1:12345).
cat > /etc/default/alloy <<'EOF'
CONFIG_FILE="/etc/alloy/config.alloy"
CUSTOM_ARGS="--server.http.listen-addr=127.0.0.1:12345 --disable-reporting"
RESTART_ON_UPGRADE=true
EOF

# Secrets from cloud.env, and a memory cap so the agent can never crowd out
# the API on a 1 GB box.
install -d /etc/systemd/system/alloy.service.d
cat > /etc/systemd/system/alloy.service.d/pony.conf <<'EOF'
[Service]
EnvironmentFile=/etc/alloy/cloud.env
Environment=GOMEMLIMIT=110MiB
MemoryMax=150M
EOF

systemctl daemon-reload
systemctl enable alloy
systemctl restart alloy
sleep 3
systemctl is-active alloy
journalctl -u alloy -n 20 --no-pager | grep -iE "error|warn" || echo "no errors in Alloy's log so far"
