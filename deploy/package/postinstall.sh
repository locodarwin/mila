#!/bin/sh
# After the package is installed or upgraded: the user Mila runs as, its
# settings readable by it, and Mila restarted if it was running. A first
# install does not start it, since its settings need a domain first.
set -e

# The first argument: deb's "configure" with the version before (empty on a
# first install), or rpm's count of versions installed (2 on an upgrade).
upgrade=''
case ${1-} in
configure) [ -n "${2-}" ] && upgrade=yes ;;
[2-9]) upgrade=yes ;;
esac

# Whether the user mila is one an install made: a system account that cannot
# sign in, as install.sh checks too. A person's own account of that name
# would own every workspace's data, so the package stops rather than use it.
system_mila() {
	uid=$(id -u mila)
	max=$(sed -n 's/^SYS_UID_MAX[[:space:]]\{1,\}\([0-9]\{1,\}\).*/\1/p' /etc/login.defs 2>/dev/null | tail -n 1)
	[ "$uid" -le "${max:-999}" ] || return 1
	shell=$(getent passwd mila | cut -d: -f7)
	case $shell in */nologin | */false) return 0 ;; esac
	return 1
}

if ! getent passwd mila >/dev/null; then
	useradd --system --home-dir /var/lib/mila --no-create-home \
		--shell "$(command -v nologin || echo /bin/false)" mila
elif ! system_mila; then
	echo "mila: a user called mila already exists and is not a system account; Mila would own its files. Rename it, or remove it, then install the package again." >&2
	exit 1
fi
chgrp mila /etc/mila/mila.toml

[ -d /run/systemd/system ] || exit 0
systemctl daemon-reload
if [ -n "$upgrade" ]; then
	systemctl try-restart mila
	exit 0
fi
systemctl is-enabled --quiet mila 2>/dev/null && exit 0
cat <<'MSG'

Mila is installed. Before starting it, set its domain in /etc/mila/mila.toml
(and email, if you have it). Then:

  sudo systemctl enable --now mila
  journalctl -u mila

The log gives the link that makes the first account. Mila listens on
127.0.0.1:8080 for a proxy in front of it; for Mila's own HTTPS instead,
the settings file says what to change. Data goes in /var/lib/mila.

MSG
