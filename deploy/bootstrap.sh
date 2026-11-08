#!/bin/bash
# One-time droplet set-up, run as root from a copy of this directory:
#
#   scp -r deploy root@droplet:/root/pony-deploy-files
#   ssh root@droplet bash /root/pony-deploy-files/bootstrap.sh "<CI public key>"
#
# Safe to re-run. Afterwards: write /etc/pony/pony.env and do-ca.crt, include
# the Nginx snippet, and run the first deploy (deploy/README.md).
set -euo pipefail
cd "$(dirname "$0")"
ci_key=$1

# pony runs the API; deploy is what CI logs in as. Neither has a password
# or a shell login of its own.
id pony >/dev/null 2>&1 || useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin pony
id deploy >/dev/null 2>&1 || useradd --create-home --shell /bin/bash deploy
passwd -l deploy >/dev/null

install -d -o deploy -g deploy -m 700 /home/deploy/.ssh
# restrict: no port forwarding, agent forwarding or pty for the CI key.
echo "restrict $ci_key" > /home/deploy/.ssh/authorized_keys
chown deploy:deploy /home/deploy/.ssh/authorized_keys
chmod 600 /home/deploy/.ssh/authorized_keys

# deploy may run exactly one command as root.
echo 'deploy ALL=(root) NOPASSWD: /usr/local/bin/pony-deploy' > /etc/sudoers.d/pony-deploy
chmod 440 /etc/sudoers.d/pony-deploy
visudo -cf /etc/sudoers.d/pony-deploy

install -d -o root -g pony -m 750 /etc/pony /opt/pony /opt/pony/releases
install -o root -g root -m 755 pony-deploy /usr/local/bin/pony-deploy
install -o root -g root -m 644 systemd/pony.service /etc/systemd/system/pony.service
install -o root -g root -m 644 nginx/pony-proxy.conf /etc/nginx/snippets/pony-proxy.conf
install -o root -g root -m 644 nginx/pony-upstream.conf /etc/nginx/conf.d/pony-upstream.conf
systemctl daemon-reload
systemctl enable pony

# The pre-Postgres API ran as this unit on the same port.
if systemctl list-unit-files the-dancing-pony-api.service >/dev/null 2>&1; then
    systemctl disable --now the-dancing-pony-api.service || true
fi

echo "bootstrap done"
