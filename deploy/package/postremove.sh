#!/bin/sh
# After the package is removed: systemd told, and where the data still is.
set -e
[ -d /run/systemd/system ] && systemctl daemon-reload || true
case ${1-} in
remove | purge | 0)
	if [ -d /var/lib/mila ]; then
		echo "Mila's data is still in /var/lib/mila, and the user mila is kept."
		echo "Delete them yourself if you are done with them: sudo rm -rf /var/lib/mila && sudo userdel mila"
	fi
	;;
esac
