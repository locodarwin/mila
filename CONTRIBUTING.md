# Contributing to Mila

Thanks for wanting to help. Bug reports and ideas go in [Issues](https://github.com/locodarwin/mila/issues). Security problems go privately, as [SECURITY.md](SECURITY.md) says, never in a public issue.

## Building it

You need Go 1.26 and Node 24.

```sh
make build
./bin/mila serve
```

Then open http://localhost:8080. Data goes in `./data` unless you set `--data-dir`. Or build the container:

```sh
podman build -t mila .
podman run -p 8080:8080 -v mila-data:/data mila
```

`make desktop` builds the Linux desktop app, in a container.

## Working on it

```sh
make dev       # Go on :8790 with live reload, Vite on :5173
make test      # Go, unit and browser tests
make test-pg   # Go tests against Postgres in a container
make lint
make budgets   # the performance budgets (docs/PLAN.md section 13)
make visual    # the style guide's screenshots in every theme, in a container
make generate  # code generated from the API spec and the queries
```

`mila dev generate --email you@example.com --tasks 5000` fills a new workspace with made-up work, to see how Mila does with a large one.

## Sending a change

- [docs/PLAN.md](docs/PLAN.md) is the plan of record: how Mila is built and why. Read the part your change touches first, and change it with your change when a decision changes.
- The API is defined first in `shared/openapi.yaml`; the Go and TypeScript code is generated from it with `make generate`, and committed.
- Every database change needs a migration for both SQLite and Postgres.
- A change in what people see or do updates its help topic (`web/src/lib/help/topics/`) and the user guide (`docs/guide/`) in the same change.
- Keep the tests passing (`make test` and `make lint`), and add one that would have caught the bug you fix.
- US spelling, and no em dashes in docs.
