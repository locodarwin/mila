#!/bin/sh
# Before the package is removed, not upgraded: Mila stopped and disabled.
set -e
case ${1-} in
remove | 0)
	if [ -d /run/systemd/system ]; then
		systemctl disable --now mila >/dev/null 2>&1 || true
	fi
	;;
esac
