#!/bin/bash
# Runs the app with no screen and has it try itself: it starts Mila on this computer,
# signs its window in, loads the page, and asks the page to use the desktop bridge.
set -u
app=${1:?the app to try}
out=${2:-/tmp/mila-smoke}
mkdir -p "$out"
export MILA_DESKTOP_SMOKE=1 MILA_DESKTOP_HOME="$out/home" HOME="$out/user"
mkdir -p "$HOME" "$MILA_DESKTOP_HOME"
Xvfb :77 -screen 0 1280x800x24 >/dev/null 2>&1 &
xvfb=$!
export DISPLAY=:77
sleep 1
dbus-launch --exit-with-session "$app" > "$out/output.txt" 2>&1 &
pid=$!
if [ -n "${SHOT:-}" ]; then
	sleep "${SHOT}"
	import -window root "$out/shot.png" 2>/dev/null || xwd -root -silent | convert xwd:- "$out/shot.png"
fi
wait "$pid"
code=$?
kill "$xvfb" 2>/dev/null
cat "$out/output.txt"
echo "smoke exit $code"
exit "$code"
