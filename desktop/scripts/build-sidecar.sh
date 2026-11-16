#!/bin/sh
# Builds the Mila program the app runs, with its pages inside, under the name Tauri
# wants beside the app. The web app comes first, as for any build of Mila.
set -eu
cd "$(dirname "$0")/../.."
triple=$(rustc -vV | sed -n 's/^host: //p')
ext=
case $triple in *windows*) ext=.exe ;; esac
if [ "${SKIP_WEB:-}" != 1 ]; then
	(cd web && npm ci --no-audit --no-fund && npm run build)
	dist=server/internal/webui/dist
	find "$dist" -mindepth 1 ! -name .gitkeep -delete
	cp -r web/build/. "$dist/"
fi
mkdir -p desktop/src-tauri/binaries
cd server
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/locodarwin/mila/server/internal/version.Version=${MILA_VERSION:-dev} -X github.com/locodarwin/mila/server/internal/version.Commit=${MILA_COMMIT:-unknown}" \
	-o "../desktop/src-tauri/binaries/mila-$triple$ext" ./cmd/mila
