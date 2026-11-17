# Mila: one Go binary with the SvelteKit frontend embedded.
#
#   make build      build web/ and bin/mila
#   make dev        Go server with live reload plus the Vite dev server
#   make test       Go tests, unit tests, browser tests
#   make test-pg    Go tests against a throwaway Postgres in podman/docker
#   make lint       gofmt, go vet, prettier, eslint, svelte-check
#   make generate   regenerate query code, the API server and the API types
#   make budgets    the performance budgets
#   make visual     the style guide's screenshots against their baselines,
#                   in the pinned Playwright image (visual-update remakes them)
#   make shots      the pictures in help and the user guide, from a seeded
#                   workspace, in the pinned Playwright image (help-shots too)

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w \
	-X github.com/locodarwin/mila/server/internal/version.Version=$(VERSION) \
	-X github.com/locodarwin/mila/server/internal/version.Commit=$(COMMIT)

DIST       := server/internal/webui/dist
CONTAINER  ?= $(shell command -v podman || command -v docker)
PG_NAME    := mila-pg-test
PG_PORT    := 55432
PG_URL     := postgres://mila:mila@127.0.0.1:$(PG_PORT)/mila?sslmode=disable
PW_IMAGE   := mcr.microsoft.com/playwright:v1.64.1-noble
AIR        := github.com/air-verse/air@v1.67.4
SQLC       := github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

.PHONY: build web server dev test test-go test-pg test-web test-e2e lint generate clean \
	budgets budgets-size budgets-go budgets-memory budgets-browser lighthouse visual visual-update shots help-shots \
	release-bins release-packages

build: web server

web: web/node_modules
	cd web && npm run build
	find $(DIST) -mindepth 1 ! -name .gitkeep -delete
	cp -r web/build/. $(DIST)/

web/node_modules: web/package-lock.json
	cd web && npm ci
	touch $@

server:
	cd server && CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o ../bin/mila ./cmd/mila

# The server program for each system a release offers, in bin/release: a tar.gz for
# Linux and macOS (macOS untested, with no Mac here) and a zip for Windows, each holding
# the program, and for Linux the systemd unit and its drop-in for Mila's own HTTPS;
# and the install script for Linux beside them. Version tags only: the names carry it.
# The version and commit (a tag's name, which may hold any character) reach the shell
# through the environment, never as text in the recipe, and must look like versions.
RELEASE_SYSTEMS := linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64
release-bins: export RELEASE_VERSION := $(VERSION)
release-bins: export RELEASE_COMMIT := $(COMMIT)
release-bins: web
	set -e; for x in "$$RELEASE_VERSION" "$$RELEASE_COMMIT"; do \
		case $$x in '' | [!A-Za-z0-9]* | *[!A-Za-z0-9.+_-]*) \
			echo "release-bins: \"$$x\" is not a version or commit, such as v3.0.0 or 1a2b3c4" >&2; exit 1 ;; \
		esac; \
	done
	rm -rf bin/release && mkdir -p bin/release
	set -e; v=$${RELEASE_VERSION#v}; \
	ldflags="-s -w -X github.com/locodarwin/mila/server/internal/version.Version=$$RELEASE_VERSION -X github.com/locodarwin/mila/server/internal/version.Commit=$$RELEASE_COMMIT"; \
	for sys in $(RELEASE_SYSTEMS); do \
		os=$${sys%/*}; arch=$${sys#*/}; name=mila_$${v}_$${os}_$${arch}; dir=bin/release/$$name; \
		mkdir -p $$dir; exe=mila; [ $$os = windows ] && exe=mila.exe; \
		(cd server && CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$$ldflags" -o ../$$dir/$$exe ./cmd/mila); \
		cp LICENSE $$dir/; \
		[ $$os = linux ] && cp deploy/mila.service deploy/mila-https.conf $$dir/; \
		if [ $$os = windows ]; then (cd bin/release && zip -qr $$name.zip $$name); \
		else tar -C bin/release --owner=0 --group=0 -czf bin/release/$$name.tar.gz $$name; fi; \
		rm -rf $$dir; \
	done
	cp deploy/install.sh bin/release/install.sh
	command ls -l bin/release

# The .deb and .rpm of each Linux program in bin/release (make release-bins first),
# made with dpkg-deb and rpmbuild by deploy/package/build.sh.
release-packages:
	set -e; for t in bin/release/mila_*_linux_*.tar.gz; do \
		n=$$(basename $$t .tar.gz); arch=$${n##*_}; v=$${n#mila_}; v=$${v%_linux_*}; \
		tar -xzf $$t -C bin/release; \
		sh deploy/package/build.sh bin/release/$$n/mila $$arch $$v bin/release; \
		rm -rf bin/release/$$n; \
	done
	command ls -l bin/release

dev: web/node_modules
	@echo "Go server on :8790 (live reload), Vite on :5173 proxying /api to it"
	@trap 'kill 0' INT TERM EXIT; \
	(cd server && go run $(AIR) -c .air.toml) & \
	(cd web && npm run dev) & \
	wait

test: test-go test-web test-e2e

test-go:
	cd server && go test ./...

test-local: web/node_modules
	cd web && npx playwright test -c playwright.local.config.ts

test-pg:
	$(CONTAINER) rm -f $(PG_NAME) >/dev/null 2>&1 || true
	$(CONTAINER) run -d --rm --name $(PG_NAME) -p 127.0.0.1:$(PG_PORT):5432 \
		-e POSTGRES_USER=mila -e POSTGRES_PASSWORD=mila -e POSTGRES_DB=mila \
		docker.io/library/postgres:17-alpine >/dev/null
	@until $(CONTAINER) exec $(PG_NAME) pg_isready -U mila >/dev/null 2>&1; do sleep 0.5; done
	cd server && MILA_TEST_REQUIRE_POSTGRES=1 MILA_TEST_POSTGRES_URL='$(PG_URL)' go test -count=1 ./...; \
		status=$$?; $(CONTAINER) rm -f $(PG_NAME) >/dev/null; exit $$status

test-web: web/node_modules
	cd web && npx vitest --run

test-e2e: web/node_modules
	cd web && npx playwright test

# The performance budgets, every one, on this machine: initial JavaScript,
# the server's API p95 and bootstrap on SQLite and Postgres, idle memory,
# first sync, warm start, interactions and board drag in the browser, and
# Lighthouse's first loads. Each prints what it measured.
budgets: build budgets-size budgets-go budgets-memory budgets-browser lighthouse

budgets-size:
	cd web && npm run budget:size

budgets-go:
	$(CONTAINER) rm -f $(PG_NAME) >/dev/null 2>&1 || true
	$(CONTAINER) run -d --rm --name $(PG_NAME) -p 127.0.0.1:$(PG_PORT):5432 \
		-e POSTGRES_USER=mila -e POSTGRES_PASSWORD=mila -e POSTGRES_DB=mila \
		docker.io/library/postgres:17-alpine >/dev/null
	@until $(CONTAINER) exec $(PG_NAME) pg_isready -U mila >/dev/null 2>&1; do sleep 0.5; done
	cd server && MILA_BUDGETS=1 MILA_TEST_REQUIRE_POSTGRES=1 MILA_TEST_POSTGRES_URL='$(PG_URL)' \
		go test -count=1 -run TestBudgets -v ./internal/httpapi/; \
		status=$$?; $(CONTAINER) rm -f $(PG_NAME) >/dev/null; exit $$status

budgets-memory:
	scripts/budget-memory.sh

budgets-browser: web/node_modules
	cd web && npx playwright test -c playwright.budgets.config.ts

lighthouse: web/node_modules
	cd web && node scripts/lighthouse.mjs

# Screenshots differ between machines, so they are taken only in the
# Playwright image whose version @playwright/test pins (CI uses it too).
VISUAL = $(CONTAINER) run --rm --ipc=host --security-opt label=disable \
	-v $(CURDIR):/w -w /w/web $(PW_IMAGE) npx playwright test -c playwright.visual.config.ts

visual: build
	$(VISUAL)

visual-update: build
	$(VISUAL) --update-snapshots

# The pictures in the help topics, in the same image. By hand, after a screen in one
# changes; then `make build` puts them in the binary.
shots: build
	$(CONTAINER) run --rm --ipc=host --security-opt label=disable \
		-v $(CURDIR):/w -w /w/web $(PW_IMAGE) npx playwright test -c playwright.shots.config.ts

help-shots: shots

lint: web/node_modules
	@test -z "$$(gofmt -l server)" || (gofmt -l server; echo "gofmt: files above need formatting"; exit 1)
	cd server && go vet ./...
	cd web && npm run lint && npm run check

generate: web/node_modules
	cd server/internal/store && go generate
	cd server/internal/httpapi/api && go generate
	cd web && npm run generate

# The desktop app, built and tried with no screen in a container of Ubuntu 22.04, so
# nothing need be installed here. Windows builds on GitHub: Actions, Desktop, Run workflow.
desktop: build
	mkdir -p tmp/desktop/cargo tmp/desktop/target tmp/desktop/smoke
	cd server && CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o ../desktop/src-tauri/binaries/mila-x86_64-unknown-linux-gnu ./cmd/mila
	$(CONTAINER) build -q -t mila-desktop-build -f desktop/scripts/Containerfile desktop/scripts
	$(CONTAINER) run --rm --network=host --device /dev/fuse --security-opt label=disable \
		-v "$$PWD/desktop:/app" -v "$$PWD/tmp/desktop/cargo:/cargo" -v "$$PWD/tmp/desktop/target:/target" -v "$$PWD/tmp/desktop/smoke:/smoke" \
		-e CARGO_TARGET_DIR=/target -w /app mila-desktop-build bash scripts/in-container.sh

clean:
	rm -rf bin web/build web/.svelte-kit
	find $(DIST) -mindepth 1 ! -name .gitkeep -delete
