#!/bin/sh
# Builds the test package that stage.py publishes (see .stage.json). It runs
# inside a git-archive export of the commit, so it starts from a clean tree:
# the web app first, then Mila for Linux (x86 and ARM) and Windows, all with
# the app embedded, into stage/bin.
set -eu

# This runs as whoever commits, outside CI's sandbox: no package's
# install scripts run here (the build needs none; esbuild's binary comes as
# a package of its own).
(cd web && npm ci --no-audit --no-fund --ignore-scripts && npm run build)

dist=server/internal/webui/dist
find "$dist" -mindepth 1 ! -name .gitkeep -delete
cp -r web/build/. "$dist/"

# The export writes the commit into scripts/stage-commit (see
# .gitattributes); run anywhere else, the build says it does not know it.
commit=$(cat scripts/stage-commit)
case $commit in
'$Format'*) commit=unknown ;;
esac

mkdir -p stage/bin
cd server
for target in linux/amd64 linux/arm64 windows/amd64; do
	os=${target%/*}
	arch=${target#*/}
	ext=
	[ "$os" = windows ] && ext=.exe
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
		-ldflags "-s -w -X github.com/locodarwin/mila/server/internal/version.Version=staged -X github.com/locodarwin/mila/server/internal/version.Commit=$commit" \
		-o "../stage/bin/mila-$os-$arch$ext" ./cmd/mila
done
