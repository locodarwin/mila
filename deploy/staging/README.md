# The staging server

Staging runs the newest commit on main whose tests passed, at the address
in its `.env`'s `MILA_DOMAIN`, with test data only. In
`.github/workflows/ci.yml`, once the Go, web and browser tests pass, the
`image` job builds the image (`image.yml`) and the `staging` job deploys it;
this is the server's side of that.

The server is a Fedora Server VPS with Docker CE and Compose from Docker's
repository. It is reached over SSH by key only, with root login off, and the
firewall (firewalld) lets in SSH, HTTP and HTTPS. `dnf5-automatic` installs
updates every morning and reboots when one needs it; the containers come
back on their own.

## Layout

- `/srv/mila`, owned by the `deploy` user: `compose.yaml` and `Caddyfile`
  copied from `deploy/`, and `.env` (mode 600) with `MILA_DOMAIN`,
  `MILA_IMAGE` (written by each deploy) and the mail settings.
- `/usr/local/bin/mila-deploy` and `/usr/local/bin/mila-set-mail`: this
  folder's scripts of those names.
- `deploy` is in the `docker` group. Its one key, in
  `~deploy/.ssh/authorized_keys`, may run only the deploy script:

  ```
  command="/usr/local/bin/mila-deploy",restrict ssh-ed25519 AAAA... github-actions-mila-staging
  ```

  The private half is the secret `STAGING_SSH_KEY` of the repository's
  `staging` environment, which only `main` may deploy to, so only the
  deploy job can read it.

## A deploy

The workflow sends the image, by digest, and its own registry token on
stdin. `mila-deploy` logs in to GHCR in a throwaway folder, pulls the image,
logs out, writes the image into `.env`, runs `docker compose up -d` and waits
for `/readyz`. The workflow then checks that `/api/v1/instance` reports the
version it built.

## By hand

As an admin, `sudo -u deploy bash -c 'cd /srv/mila && docker compose logs -f mila'`
follows the log, and `... docker compose up -d` starts what `.env` names.
`mila-set-mail` sets the Postmark settings from a server token read on stdin,
so the token stays out of command lines and shell history, and restarts Mila.
From a machine with the admin key:

```
read -rsp "Postmark server token: " T; echo; printf '%s\n' "$T" | ssh <admin>@<host> sudo mila-set-mail; unset T
```

When `deploy/compose.yaml` or `deploy/Caddyfile` change, copy them to
`/srv/mila` again; deploys do not. On a new server, the first start prints a
one-time setup link to the log.
