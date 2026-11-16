#!/bin/bash
# What `make desktop` runs inside its container: tests, the AppImage, and a try of it.
set -eu
command -v cargo >/dev/null || curl -sSf https://sh.rustup.rs | sh -s -- -y --profile minimal >/dev/null 2>&1
command -v node >/dev/null || { curl -fsSL https://nodejs.org/dist/v24.11.0/node-v24.11.0-linux-x64.tar.xz | tar xJ -C /tmp; export PATH=/tmp/node-v24.11.0-linux-x64/bin:$PATH; }
npm ci --no-audit --no-fund
(cd src-tauri && cargo test)
APPIMAGE_EXTRACT_AND_RUN=1 NO_STRIP=1 npx tauri build --bundles appimage
scripts/smoke.sh /target/release/mila-desktop /smoke
