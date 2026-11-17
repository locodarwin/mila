#!/usr/bin/env bash
# The server's idle-memory budget: bin/mila,
# serving a generated 5,000-task workspace, must sit under 100 MB of
# resident memory once it has served that workspace's bootstrap and gone
# quiet. Linux only (it reads /proc). Run after `make build`, or through
# `make budgets`.
#
#   scripts/budget-memory.sh [seconds to wait for idle, default 30]
set -euo pipefail

BUDGET_KB=$((100 * 1024))
IDLE=${1:-30}
PORT=${BUDGET_PORT:-4180}
BASE="http://127.0.0.1:$PORT"
ROOT=$(cd "$(dirname "$0")/.." && pwd)
MILA="$ROOT/bin/mila"
DATA=$(mktemp -d)
JAR="$DATA/cookies"
PID=""

cleanup() {
	if [ -n "$PID" ]; then kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; fi
	rm -rf "$DATA"
}
trap cleanup EXIT

[ -x "$MILA" ] || { echo "no $MILA; run make build first" >&2; exit 1; }
flags=(--data-dir "$DATA/data" --base-url "$BASE" --log-format text --log-level warn)

ws=$("$MILA" dev generate --email perf@example.com --tasks 5000 --seed 1 "${flags[@]}" 2>/dev/null | awk '/^workspace/ {print $2}')
"$MILA" serve --listen "127.0.0.1:$PORT" "${flags[@]}" >"$DATA/server.log" 2>&1 &
PID=$!
for _ in $(seq 100); do
	curl -fs "$BASE/healthz" >/dev/null 2>&1 && break
	sleep 0.1
done
curl -fs "$BASE/healthz" >/dev/null || { cat "$DATA/server.log" >&2; exit 1; }

# Sign in as the generated workspace's owner, then sync it once, as a
# first visit would.
link=$("$MILA" user login-link perf@example.com "${flags[@]}" 2>/dev/null)
token=${link##*token=}
curl -fs -c "$JAR" -b "$JAR" -H 'Content-Type: application/json' -H "Origin: $BASE" \
	-d "{\"token\":\"$token\"}" "$BASE/api/v1/auth/magic-link/verify" >/dev/null
schema=$(sed -n 's/^export const SYNC_SCHEMA = \([0-9]*\);/\1/p' "$(dirname "$0")/../web/src/lib/sync/entities.ts")
bytes=$(curl -fs -b "$JAR" --compressed "$BASE/api/v1/workspaces/$ws/sync/bootstrap?schema=$schema" | wc -c)
peak=$(awk '/VmHWM/ {print $2}' "/proc/$PID/status")

sleep "$IDLE"
rss=$(awk '/VmRSS/ {print $2}' "/proc/$PID/status")
echo "idle memory after a ${bytes}-byte bootstrap and ${IDLE}s: $((rss / 1024)) MB resident (peak $((peak / 1024)) MB), budget $((BUDGET_KB / 1024)) MB"
if [ "$rss" -ge "$BUDGET_KB" ]; then
	echo "over the idle-memory budget" >&2
	exit 1
fi
