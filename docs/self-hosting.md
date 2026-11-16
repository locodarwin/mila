# Self-hosting Mila

Mila is one program with everything in it: the app, the server and its database. By default it keeps its data in SQLite, a single file, so a whole instance is one folder. This guide gets one running in about five minutes and covers what to do afterward: email, backups, upgrades.

## What you need

- **A machine to run it.** Anything that runs Docker or Podman with Compose, such as a small VPS, a home server or a NAS. Mila needs very little: about 50 MB of memory when idle, and room for your files.
- **A domain or subdomain** for it, such as `mila.example.com`. Mila needs one of its own: it cannot live under a path like `example.com/mila`. Passkeys, the way Mila prefers you sign in, also need a name and HTTPS, not a bare IP address.

  The desktop app needs none of this when it keeps your work on its own computer: that mode has no server, no account and no sign-in. Connected to a server, it signs in through that server, so the server still needs its own domain and HTTPS.
- **Email, optionally.** Mila sends sign-in links, invitations, reminders and the daily digest by email through any SMTP service. Without one, it writes sign-in links to its log, which is enough to try it out.

## Trying it in a minute

To see Mila before setting it up, run it with Docker or Podman on your own computer:

```sh
docker run --rm -p 127.0.0.1:8080:8080 ghcr.io/locodarwin/mila:latest
```

It prints a setup link: open it, make an account, and look around at <http://localhost:8080>. Nothing is kept: stopping it with Ctrl+C deletes everything, so do not start real work there.

## Option 1: a machine of its own

Caddy comes along and gets the HTTPS certificate. Ports 80 and 443 must be free and open to the internet, and the domain must point at the machine.

1. Copy `deploy/compose.yaml`, `deploy/Caddyfile` and `deploy/.env.example` into a folder on the machine, and rename `.env.example` to `.env`.
2. In `.env`, set `MILA_DOMAIN` to your domain. Leave the rest for now.
3. Start it:

   ```sh
   docker compose up -d
   ```

4. Open the setup link from Mila's log (see [The first account](#the-first-account)).

## Option 2: behind a proxy you already run

If the machine already runs Nginx Proxy Manager, Traefik, nginx or a Caddy of its own, Mila goes behind it and brings no Caddy.

1. Copy `deploy/compose.proxy.yaml` and `deploy/.env.example` into a folder, and rename `.env.example` to `.env`.
2. In `.env`, set `MILA_DOMAIN`.
3. Start it:

   ```sh
   docker compose -f compose.proxy.yaml up -d
   ```

   Mila now answers on `127.0.0.1:8080`, on this machine only. `MILA_BIND` and `MILA_PORT` in `.env` change that, for a proxy that runs elsewhere.

4. Point the proxy at it:
   - Forward the domain to `http://127.0.0.1:8080` (or wherever `MILA_BIND` and `MILA_PORT` put it).
   - Turn WebSockets on. Mila keeps every open app up to date over one, at `/api/v1/sync/ws`.
   - Let request bodies through up to 256 MB, the largest import. Attachments are up to 25 MB.
   - Pass `X-Forwarded-For`, as proxies do by default.

5. Open the setup link from Mila's log (see [The first account](#the-first-account)).

Mila believes `X-Forwarded-For` only from the addresses in `MILA_TRUSTED_PROXIES`. That is how it knows each visitor's address for its sign-in limits and the security log. If Mila's log says a request came through a proxy that `MILA_TRUSTED_PROXIES` does not name, add the address it gives to `MILA_TRUSTED_PROXIES` in `.env` and start Mila again. Until then, every visitor counts as one, so one of them could use up the sign-in limit for all.

`compose.proxy.yaml` puts Mila on a network of its own, `172.30.80.0/24` (`MILA_SUBNET` and `MILA_GATEWAY` change it), and trusts only its gateway, `172.30.80.1`, which is this machine: a proxy running here works as it is, and so does anything else running here, so give the machine to nobody you would not trust to say where visitors come from. A proxy in a container has an address of its own; name that instead.

## Option 3: the program on its own

Mila is also one file you can run without containers, getting its own HTTPS certificate or behind a reverse proxy.

Every release lists its files' checksums in `SHA256SUMS`, which catches a broken download. Each release also carries GitHub's attestation, `mila.intoto.jsonl`, that Mila's release workflow built its files on GitHub's own machines from the release's tag, which nobody can make for a file built anywhere else. With `gh` 2.49 or later, check any file you downloaded against the release you meant to download (here v3.0.0; use the release's own tag), with no sign-in:

```sh
gh attestation verify <file> --bundle mila.intoto.jsonl --repo locodarwin/mila \
  --signer-workflow locodarwin/mila/.github/workflows/desktop.yml \
  --source-ref refs/tags/v3.0.0 --deny-self-hosted-runners
```

Keep all four checks: `--repo` alone accepts a file attested by any workflow of the repository, from any branch or tag. The install script checks the program this way by itself when `gh` is installed. Releases cannot be changed once published, and `gh release verify-asset v3.0.0 <file> --repo locodarwin/mila` checks that a file is one the release was published with.

### With the install script

On Linux with systemd (Debian, Ubuntu, Fedora, Raspberry Pi OS and most others, on amd64 or arm64), `install.sh` from the release does it all: it downloads the program for the machine and checks it against `SHA256SUMS`, makes the `mila` user, installs the program and the systemd unit, asks for the domain and whether Mila gets its own certificate or runs behind a proxy, starts Mila and prints the link that makes the first account.

1. Download the script and the release's attestation, check the script, and read it before running it: it runs as root. Use the latest release's tag in place of v3.0.0 (the releases page shows it).

   ```sh
   curl -fsSLO https://github.com/locodarwin/mila/releases/download/v3.0.0/install.sh
   curl -fsSLO https://github.com/locodarwin/mila/releases/download/v3.0.0/mila.intoto.jsonl
   gh attestation verify install.sh --bundle mila.intoto.jsonl --repo locodarwin/mila \
     --signer-workflow locodarwin/mila/.github/workflows/desktop.yml \
     --source-ref refs/tags/v3.0.0 --deny-self-hosted-runners
   less install.sh
   ```

   Without `gh`, nothing shows where the script came from but GitHub's address; read all of it. The script itself checks the program it downloads by its checksum, and with `gh` by its attestation too, against the latest release's tag, or the one `--version` names. It never replaces Mila with an older release unless `--version` names that release.
2. Run it:

   ```sh
   sudo sh install.sh
   ```

   On a computer without the internet, download `SHA256SUMS`, the `mila_<version>_linux_<arch>.tar.gz` for it and `mila.intoto.jsonl` into a folder, and run `sudo sh install.sh --from <folder> --version v3.0.0`.

   It asks for the domain, then whether Mila should get its own HTTPS certificate (the domain must point at the machine, with ports 80 and 443 open) or run behind a proxy you have, on `127.0.0.1:8080` as in option 2. `--help` lists its options; with `--domain`, `--proxy` and `--yes` it asks nothing.
3. Open the link it prints.

The answers go in `/etc/mila/mila.toml`, with any other settings you add (each is the setting's name in lower case without `MILA_`, so `MILA_SMTP_URL` is `smtp_url`). Only root and Mila can read that file, so passwords belong there; apply a change with `sudo systemctl restart mila`. The data is in `/var/lib/mila`. Behind a proxy on the same computer, the script also trusts that proxy alone to say who is asking, so each visitor has a sign-in limit of their own. To upgrade, run the script again: it replaces the program and the unit, keeps the settings and the data, and restarts Mila, which backs up the database before changing it. `sudo sh install.sh --uninstall` removes Mila and keeps the data and `/etc/mila/mila.toml`; installing again uses that file as it is, and stops first if it says otherwise than Mila's own HTTPS needs (another address, or a proxy to trust). `--uninstall --purge` deletes them too.

### On a Raspberry Pi

A Raspberry Pi 4, 5, 400 or 500 runs Mila well, with the install script above or the `.deb`. It needs the 64-bit Raspberry Pi OS (Lite is enough), since Mila is built for arm64 only, and 2 GB of memory or more. It was tried on a Raspberry Pi 400 with the 64-bit Raspberry Pi OS (Debian 13) and Mila 2.11.0, set up for the home network; say if a step differs on yours.

1. Use a USB drive or SSD for the system rather than the SD card if you can. Mila writes its database often, and SD cards wear out; the data is in `/var/lib/mila`.
2. Download and check `install.sh` as above, on the Pi itself, and run it there. On a Pi without the internet, download the files on another computer and copy them over:

   ```sh
   gh release download v3.0.0 --repo locodarwin/mila --pattern install.sh --pattern SHA256SUMS --pattern 'mila_*_linux_arm64.tar.gz' --pattern mila.intoto.jsonl
   scp install.sh SHA256SUMS mila_*_linux_arm64.tar.gz mila.intoto.jsonl pi@raspberrypi.local:
   ```

   Use the Pi's address in place of `raspberrypi.local` if your computer doesn't find `.local` names. Then, on the Pi, `sudo sh install.sh --from . --version v3.0.0`. On a home network any domain will do when it asks, such as the Pi's own name; step 3 sets the address people use.
3. Choose how it is reached.
   - **With its own certificate,** when the domain points at your home and the router forwards ports 80 and 443 to the Pi: answer yes to the certificate, and nothing more is needed.
   - **Otherwise,** answer that it runs behind a proxy, then let Mila listen on the network instead of only on the Pi itself. After the install script, that address is the service's own setting, which wins over `mila.toml`: run `sudo systemctl edit mila` and add these lines. (After the `.deb`, set `listen = "0.0.0.0:8080"` in `/etc/mila/mila.toml` instead.)

     ```ini
     [Service]
     Environment=MILA_LISTEN=0.0.0.0:8080
     ```

     Then change the settings in `/etc/mila/mila.toml` for how people reach it, and apply them with `sudo systemctl restart mila`:
     - **Behind a proxy you already run elsewhere,** such as on a NAS: trust the proxy's address, as `trusted_proxies = "192.168.1.50"`, and point the proxy at `http://<the Pi's address>:8080`, with WebSockets on.
     - **On your home network only:** take out `trusted_proxies`, and set `base_url = "http://<the Pi's address>:8080"`. Only email links sign in there, since passkeys need a domain and HTTPS. Until you set up [email](#email), Mila writes its emails, sign-in links among them, to its log: `sudo journalctl -u mila`.

### With a package

Each release has a `.deb` (Debian, Ubuntu, Raspberry Pi OS) and an `.rpm` (Fedora, RHEL, openSUSE) for amd64 and arm64. They come from the release, not from an apt or dnf repository, so an upgrade is the next release's package, installed the same way.

1. Download the one for the machine, and check it:

   ```sh
   gh release download v3.0.0 --repo locodarwin/mila --pattern 'mila_*_amd64.deb' --pattern SHA256SUMS --pattern mila.intoto.jsonl
   sha256sum --ignore-missing -c SHA256SUMS
   gh attestation verify mila_*_amd64.deb --bundle mila.intoto.jsonl --repo locodarwin/mila \
     --signer-workflow locodarwin/mila/.github/workflows/desktop.yml \
     --source-ref refs/tags/v3.0.0 --deny-self-hosted-runners
   ```

2. Install it: `sudo apt install ./mila_*_amd64.deb`, or `sudo dnf install ./mila-*.x86_64.rpm`. It makes the `mila` user and installs the program as `/usr/bin/mila`, the systemd unit, and the settings in `/etc/mila/mila.toml`. It does not start Mila yet.
3. In `/etc/mila/mila.toml`, set `base_url` to your domain. Mila listens on `127.0.0.1:8080` for a proxy, as in option 2. For Mila's own HTTPS instead, the file's comments say what to change, and which drop-in to add.
4. Start it, and find the first account's link in its log:

   ```sh
   sudo systemctl enable --now mila
   journalctl -u mila
   ```

An upgrade keeps the settings file as you left it and restarts Mila. Removing the package stops Mila and keeps its data in `/var/lib/mila`, and the user.

### By hand

1. Download it from the release: `mila_<version>_linux_amd64.tar.gz` (or `linux_arm64`, for a Raspberry Pi or another ARM machine), with `SHA256SUMS` and `mila.intoto.jsonl` beside it. The `gh` tool downloads them and checks the file:

   ```sh
   gh release download v3.0.0 --repo locodarwin/mila --pattern 'mila_*_linux_amd64.tar.gz' --pattern SHA256SUMS --pattern mila.intoto.jsonl
   sha256sum --ignore-missing -c SHA256SUMS
   gh attestation verify mila_*_linux_amd64.tar.gz --bundle mila.intoto.jsonl --repo locodarwin/mila \
     --signer-workflow locodarwin/mila/.github/workflows/desktop.yml \
     --source-ref refs/tags/v3.0.0 --deny-self-hosted-runners
   tar -xzf mila_*_linux_amd64.tar.gz
   ```

   Each holds the program, `mila`, the systemd unit, `mila.service`, and its drop-in for Mila's own HTTPS, `mila-https.conf`. There is a zip for Windows too, and tar files for macOS, which are untested. To build it yourself instead, `make build` makes `bin/mila`, with Go 1.26 and Node 24.
2. Install it with the systemd unit. Its comments give the commands. Its `Environment=` lines hold the settings, or a drop-in in `/etc/systemd/system/mila.service.d/` does, which an upgrade leaves alone; but systemd shows those lines to anyone on the computer (`systemctl show mila`), so put passwords (`MILA_SMTP_URL`, `MILA_IMAP_URL`, `MILA_DATABASE_URL`) in a TOML file only root and Mila can read instead, and name it in the unit: `Environment=MILA_CONFIG=/etc/mila/mila.toml`, with the file owned by `root:mila` and mode 640. The unit keeps the data directory to the `mila` user alone, since it holds everything, webhook secrets among it, and limits what the program may do on the computer. If you installed an earlier unit, copy this one over it and run `sudo systemctl daemon-reload && sudo systemctl restart mila`; Mila makes a database other users could read readable by its owner only as it starts.
3. Point your proxy at `127.0.0.1:8080` as in option 2, and set `MILA_TRUSTED_PROXIES` to the proxy's address. Or, with no proxy, let Mila do its own HTTPS, below.

### Mila's own HTTPS

With `MILA_TLS_DOMAIN` set, Mila gets a certificate for that domain from Let's Encrypt, renews it before it runs out, and serves HTTPS itself, so it needs no proxy. It answers on port 443, and on port 80 it answers Let's Encrypt's checks and sends everyone else to the HTTPS address. The domain must point at the machine, with both ports open to the internet. The certificate is kept in `certs` in the data directory.

The install script sets this up when you say yes to Mila's own certificate. By hand, the download holds a drop-in for the systemd unit, `mila-https.conf` (or `deploy/mila-https.conf`). Its comments give the commands: it goes in `/etc/systemd/system/mila.service.d/`, with your domain and email in it. It lets Mila bind ports below 1024 and nothing more.

Mila says in its log once the certificate is ready, or why it is not: most often the domain does not point here yet, or a firewall closes port 80 or 443. It tries again on the next visit. Let's Encrypt allows only a few certificates for one domain a week, so while trying things out, `MILA_TLS_CA=https://acme-staging-v02.api.letsencrypt.org/directory` uses its staging service, whose certificates browsers do not trust; delete `certs` in the data directory before going back.

## Option 4: a NAS

Mila runs in the container app of a NAS, with its data in a folder of the NAS's, using `deploy/nas/compose.yaml`. The NAS's own reverse proxy, where it has one, gives it a domain and HTTPS; without one, Mila answers on port 8080 on your network, where only email links sign in, since passkeys need a domain and HTTPS. Each of these is written from the NAS's own documentation, or for UGREEN from a working UGOS setup, and none has been tried on its NAS yet; say if a step differs.

Behind a reverse proxy, Mila needs to know the proxy's address, as in option 2, to tell visitors apart for its sign-in limits and the security log. The NAS files trust no one until told, since every container on the NAS, and on some networks other computers too, would otherwise be believed. Once the proxy works, open Mila through it once; Mila's log then says "a request came through a proxy that MILA_TRUSTED_PROXIES does not name", with the address after `proxy=`. Set `MILA_TRUSTED_PROXIES` to that address and start Mila again. Until then nothing breaks, but everyone coming through the proxy shares one sign-in limit. If the proxy's address ever changes, the log says so again.

### Synology (DSM 7.2, Container Manager)

1. In File Station, make a folder for Mila's data, such as `docker/mila` (`/volume1/docker/mila`).
2. Let the image's user own it. Mila runs as user 65532, which DSM's permissions cannot name, so once, by SSH: `sudo chown 65532:65532 /volume1/docker/mila`. Without SSH, a scheduled task in Control Panel, Task Scheduler, run once as root, does the same.
3. In Container Manager, Project, Create: name it `mila`, set its path to the folder above, and paste `deploy/nas/compose.yaml`. Set `MILA_BASE_URL`, and the volume's path if it differs.
4. For a domain with HTTPS: in Control Panel, Login Portal, Advanced, Reverse Proxy, send `https://mila.example.com` to `http://localhost:8080`, and under Custom Header add WebSocket. Control Panel, Security, Certificate gets the certificate from Let's Encrypt. In the compose file, publish the port as `"127.0.0.1:8080:8080"`, so Mila is reached only through the proxy.
5. The setup link is in the project's log, under the container's Log tab.
6. Behind the reverse proxy, open Mila through it once and set `MILA_TRUSTED_PROXIES` to the address the log gives, as above, then build the project again.

### TrueNAS SCALE (24.10 or later)

1. Make a dataset for Mila's data, such as `tank/apps/mila`, with the Apps permissions preset, which lets TrueNAS's apps user (568) write to it.
2. In Apps, Discover Apps, Install via YAML: name it `mila` and paste `deploy/nas/compose.yaml`, with `user: "568:568"` uncommented, `MILA_BASE_URL` set, and the volume pointing at the dataset (`/mnt/tank/apps/mila:/data`).
3. TrueNAS has no reverse proxy of its own: for a domain with HTTPS, put one in front (such as Nginx Proxy Manager from its apps), as in option 2.
4. The setup link is in the app's logs.
5. Behind the reverse proxy, open Mila through it once and set `MILA_TRUSTED_PROXIES` to the address the log gives, as above, then update the app.

### Unraid (6.12 or later)

`deploy/unraid/mila.xml` is a template for Unraid's Docker page. Until Mila is in Community Applications, copy it to `/boot/config/plugins/dockerMan/templates-user/my-mila.xml` on the flash drive, then in Docker, Add Container, choose the template `mila`.

1. Set the address (`MILA_BASE_URL`), and the email server if you have one. The data goes in `/mnt/user/appdata/mila`.
2. Apply. The template runs Mila as Unraid's `nobody:users` (99:100), who own appdata, so the New Permissions tool leaves it working.
3. For a domain with HTTPS, a reverse proxy container such as SWAG or Nginx Proxy Manager goes in front, as in option 2.
4. The setup link is in the container's log.
5. Behind the reverse proxy, open Mila through it once and put the address the log gives in the template's Trusted proxies, as above, then apply.

### UGREEN (UGOS Pro, the Docker app)

This follows the usual way of running apps on a UGREEN NAS: each app is a Project in the Docker app, its compose file kept in `docker/compose/<app>` and its data in `docker/appdata/<app>`, with Nginx Proxy Manager in front on a shared Docker network named `proxy`, holding the certificate. If you don't run a proxy yet, leave out the network changes in step 3 and Mila answers on port 8080 on your network.

1. In File Manager, make the folders `docker/compose/mila` and `docker/appdata/mila` (`/volume1/docker/...`; check the `docker` shared folder's path if your pool is elsewhere). The Docker app won't make a missing folder for you: Deploy fails with "NAS path not found".
2. Find who owns `appdata/mila`. On UGOS the first account is usually 1000:1000; by SSH, `id <your user>` says for certain. Mila runs as that user, set in step 3, so it can write its data.
3. In the Docker app, Project, Create: name it `mila`, set its path to `/volume1/docker/compose/mila`, and paste `deploy/nas/compose.yaml`, with these changes:
   - `user: "1000:1000"` uncommented, with the owner from step 2;
   - the volume as `/volume1/docker/appdata/mila:/data`;
   - `MILA_BASE_URL` as the address people will use, such as `https://mila.example.com`;
   - behind Nginx Proxy Manager, `container_name: mila` under the service, the `ports` lines taken out, and the shared network added:

   ```yaml
       networks: [proxy]

   networks:
     proxy:
       external: true
   ```

   Deploy, then save the same compose text in `docker/compose/mila`, with the rest of your compose files.
4. In Nginx Proxy Manager, Hosts, Proxy Hosts, Add Proxy Host:
   - Details: Domain Names `mila.example.com`; Scheme `http`; Forward Hostname `mila`; Forward Port `8080`; Block Common Exploits on; **Websockets Support on**, since Mila keeps every open page in step through a WebSocket.
   - SSL: your certificate, such as a wildcard for the domain; Force SSL and HTTP/2 Support on.
5. The setup link is in the project's log: the Docker app, Project, `mila`, the container's log.
6. Open Mila through `https://mila.example.com` once. The log then names Nginx Proxy Manager's address on the `proxy` network (`proxy=172.20.0.2`, say): set `MILA_TRUSTED_PROXIES` to it in the compose text, as above, and deploy again. Without published ports, Mila is reached only over that network. Should Nginx Proxy Manager be made again and get another address, the log names the new one.

For use only at home and over a private network such as Tailscale, an internal name such as `mila.int.example.com` works the same way, as long as the certificate covers it: passkeys need the domain and HTTPS, not a public address. If uploading a file or importing an export fails with "413 Request Entity Too Large", add `client_max_body_size 100m;` under the proxy host's Advanced tab. Everything Mila keeps, its nightly backups included, is in `appdata/mila`: back that folder up with the NAS.

## The first account

On its first start, Mila prints a one-time link that creates the first account, which is the instance's admin. Find it in the log:

```sh
docker compose logs mila | grep -o 'https://[^"]*/setup?token=[^"]*'
```

Open it, give your name and email address, and add a passkey when Mila offers. If the link is lost, start Mila again: until someone has used one, every start prints a new link.

Sign-up is by invitation only. Invite people from Settings, People.

If you are ever locked out, the server can make you a sign-in link or an admin:

```sh
docker compose exec mila mila user login-link you@example.com
docker compose exec mila mila user make-admin you@example.com
```

(For option 3, run `mila user login-link ...` with the same settings as the service.)

## Email

Set two lines in `.env`, then run `docker compose up -d` again:

```sh
MILA_SMTP_URL=smtp://user:password@smtp.example.com:587
MILA_MAIL_FROM=Mila <mila@example.com>
```

- `smtp://` upgrades the connection with STARTTLS, usually on port 587; `smtps://` is TLS from the start, usually on port 465. A server that offers no STARTTLS is refused, since someone on the way could have taken it out of the answer, unless it is on this computer or the URL ends `?tls=none` (a relay on a network you trust). Characters such as `@`, `:` or `/` in the user name or password must be percent-encoded.
- The sender address must be on a domain your email service lets you send from, which usually means adding its DKIM and return-path records to your DNS.
- With Postmark, the server's API token is both the user name and the password: `smtp://<token>:<token>@smtp.postmarkapp.com:587`.

To check it, use Settings, Notifications, Send a test email.

## Email to task

People can forward an email to a private address and have it become a task. For that, Mila reads a mailbox you give it over IMAP, about once a minute, so nothing needs to reach Mila from outside. Set two lines in `.env`, then run `docker compose up -d` again:

```sh
MILA_IMAP_URL=imaps://user:password@imap.example.com:993/INBOX
MILA_INBOUND_ADDRESS=mila+{token}@example.com
```

Without them, Settings, Email to task says the server has no mailbox, and everything else works as usual. To see how it's going, look at Settings, Admin.

**The address.** `MILA_INBOUND_ADDRESS` is what each person's address looks like, with `{token}` standing for its private part. It must be an address that delivers to the mailbox in `MILA_IMAP_URL`. There are two ways to do that, and any mail provider that does either will work:

- **Plus addresses:** `mila+{token}@example.com`, for a mailbox `mila@example.com` whose provider delivers `mila+anything@` to it. Most do. Send yourself a test first, since a few use another character instead, in which case write that one.
- **A catch-all domain:** `{token}@in.example.com`, for a domain or subdomain that delivers everything to one mailbox.

**The mailbox.** `imaps://` is TLS from the start, usually on port 993; `imap://` upgrades with STARTTLS, usually on port 143. Characters such as `@` in the user name or password must be percent-encoded (`%40`), as in `MILA_SMTP_URL`. The path names the folder, `INBOX` if left out. Mila signs in with a password, so:

| Your mail is with | What to know |
| --- | --- |
| Your own server (Dovecot and the like) | Works with an ordinary password. With a certificate no authority signed, add `?verify=off`. On the same machine with no TLS at all, add `?tls=none`; Mila refuses that for any other host. |
| Fastmail, iCloud, Yahoo, Gmail | These generally ask for an app password rather than your own, made in the provider's account settings. Check their instructions, and that IMAP is on. |
| Proton | Reached through Proton Mail Bridge, which offers IMAP on your own machine, with a certificate of its own: `imap://user:password@127.0.0.1:1143/INBOX?verify=off`. |
| Microsoft (Outlook.com, Microsoft 365) | These take no password over IMAP: Mila signs in through Microsoft instead. See [A Microsoft mailbox](#a-microsoft-mailbox) below. |

### A Microsoft mailbox

Outlook.com, Hotmail and Microsoft 365 take no password over IMAP, so Mila signs in to them through Microsoft, with an app you register once in Microsoft Entra. It asks only to read and delete mail over IMAP, and to stay signed in; nothing else of the account. But that reaches every mailbox the account you sign in as can open, not only Mila's: sign in as the mailbox's own account. A 365 shared mailbox works too, such as `tasks@contoso.com`, made in the Exchange admin center (Recipients, Mailboxes, Add a shared mailbox). A shared mailbox cannot sign in itself: sign in as an account with full access to it and to nothing else. Settings, Admin warns when the account you signed in as is not the mailbox's own. Mila keeps the sign-in in its database, sealed with a key in the data directory, `secret.key`: a backup of the database alone can't use it, but a copy of the whole data directory can, so keep such copies as you would the mailbox's password.

1. Sign in to [Microsoft Entra](https://entra.microsoft.com) as an admin of your 365 organization (for Outlook.com, with any Microsoft account), and register the app: Applications, App registrations, New registration. Name it Mila. For a 365 mailbox, choose accounts in this organizational directory only; for Outlook.com, personal Microsoft accounts. Leave the redirect address empty.
2. In the app's Authentication, turn on Allow public client flows: Mila signs in with a code, and keeps no secret.
3. In its API permissions, add Microsoft Graph's delegated permissions `IMAP.AccessAsUser.All` and `offline_access`, and for a 365 organization, grant admin consent.
4. For a 365 mailbox, check that IMAP is on for it: in the Exchange admin center, Recipients, Mailboxes, choose the mailbox, then Manage email apps.
5. Set Mila's settings, with the mailbox's address in `MILA_IMAP_URL` and no password, and the two IDs from the app's Overview:

   ```sh
   MILA_IMAP_URL=imaps://tasks%40contoso.com@outlook.office365.com:993/INBOX
   MILA_INBOUND_ADDRESS=tasks+{token}@contoso.com
   MILA_IMAP_AUTH=microsoft
   MILA_MICROSOFT_CLIENT_ID=<Application (client) ID>
   MILA_MICROSOFT_TENANT=<Directory (tenant) ID>
   ```

   For Outlook.com, `MILA_MICROSOFT_TENANT=consumers`. Outlook.com and 365 deliver plus addresses, so `+{token}` needs nothing more.
6. Restart Mila, then in Settings, Admin, choose Connect a Microsoft mailbox. Open the address it shows (microsoft.com/devicelogin) on any device, enter the code, and sign in as the mailbox's account. Microsoft then says you have signed in to the app by its name on your device, and Settings, Admin shows the mailbox as Connected, with the account it is signed in as. Or, at the server, `mila mailbox connect` prints the code and waits.

Mila keeps the sign-in Microsoft hands back in its database, sealed and never shown again; `mila mailbox disconnect`, or Disconnect on Settings, Admin, forgets it, and removing Mila from the account's apps at Microsoft ends it there. If Microsoft stops taking it (the password changed, an admin revoked it, or it went unused for months), email to task pauses, Settings, Admin says so with Connect again, and each instance admin gets one email. Mail waits in the mailbox meanwhile, and is read once someone signs in again. If Mila can't open the sign-in (a database restored on another server without the data directory's `secret.key`, a key file it may not read, or Mila run with another data directory), email to task pauses and Settings, Admin says so, with the reason. Putting the key back where Mila can read it starts it again at the next check, with no new sign-in; or choose Connect again. Run `mila mailbox connect` and `mila config check` as the server's user (`sudo -u mila ...`): run as root, `mila mailbox connect` refuses rather than make a key the server couldn't read. `mila config check` changes nothing, even when the sign-in fails. These steps were tried against a Microsoft 365 organization with a shared mailbox (2026-11-13); Outlook.com has not been tried yet.

A mailbox of its own is best. If it shares one with other mail, Mila reads the headers of everything to find its own addresses, and downloads the rest of a message only if it is addressed to someone there from a sender they listed. It changes nothing: it leaves all of it where it is, and marks none of it read. A folder of its own, filled by a filter in your mail provider, keeps the rest out of its sight.

When a mailbox is first set, Mila starts from the newest mail, so what is already there stays out of it. It remembers how far it has read, so mail that arrives while Mila is stopped is read when it starts again. Sign-in failures slow its checks down, up to one every half hour, so that a wrong password doesn't get an account locked.

Mail stays in the mailbox unless you set `MILA_IMAP_AFTER=delete`, which deletes what was addressed to Mila once dealt with, and nothing else. A server that can't delete one message alone keeps it, and says so in the log.

## Settings

Each setting is an environment variable, a flag of the same name to `mila serve`, or a line in a TOML file named by `MILA_CONFIG` or `--config`. Flags win over the environment, which wins over the file. `mila config check` prints the settings in effect, with secrets hidden, and connects to the database. Then it tries what else must work, without changing anything: it signs in to the email server as sending would, and sends nothing; signs in to email to task's mailbox and opens it to read only; and asks `MILA_BASE_URL` for Mila's health, to see that the address reaches this Mila and not another, or a proxy with nothing behind it. Run it with the same settings as the server, while the server runs, which for a systemd install is the unit's: `sudo -u mila env $(systemctl show mila -p Environment --value) mila config check`. It ends with OK, or with the problems to fix. Settings, Admin lists what is not set up yet: email, HTTPS, a domain for passkeys, nightly backups, and a Microsoft mailbox Mila is not signed in to.

| Environment | Flag | Default | |
|---|---|---|---|
| `MILA_BASE_URL` | `--base-url` | `http://localhost:8080` | The address people use, a domain or subdomain. The compose files set it from `MILA_DOMAIN`. |
| `MILA_LISTEN` | `--listen` | `127.0.0.1:8080` | Where the server listens: this computer alone by default. The image listens on `:8080` inside its container. |
| `MILA_DATA_DIR` | `--data-dir` | `data` (`/data` in the image) | The database, attachments, pictures, exports and backups. |
| `MILA_DATABASE_URL` | `--database-url` | SQLite in the data directory | `sqlite:///path/to/mila.db` or `postgres://user:password@host/mila`. |
| `MILA_SMTP_URL` | `--smtp-url` | none: email goes to the log | See [Email](#email). `file:///some/dir` writes each email to a file instead. |
| `MILA_MAIL_FROM` | `--mail-from` | `mila@` the base URL's host | The sender of Mila's email. |
| `MILA_IMAP_URL` | `--imap-url` | none: email to task is off | The mailbox it reads. See [Email to task](#email-to-task). `file:///some/dir` reads `.eml` files from a folder instead, and deletes them, for trying it out. |
| `MILA_INBOUND_ADDRESS` | `--inbound-address` | none | What a person's address looks like, with `{token}` in it. Set both this and `MILA_IMAP_URL`, or neither. |
| `MILA_IMAP_POLL` | `--imap-poll` | `60` | How often to check the mailbox, in seconds. |
| `MILA_IMAP_AFTER` | `--imap-after` | `keep` | What becomes of mail addressed to Mila once dealt with: `keep` or `delete`. |
| `MILA_IMAP_AUTH` | `--imap-auth` | `password` | How Mila signs in to the mailbox: the password in `MILA_IMAP_URL`, or `microsoft`, through Microsoft. See [A Microsoft mailbox](#a-microsoft-mailbox). |
| `MILA_MICROSOFT_CLIENT_ID` | `--microsoft-client-id` | none | With `microsoft`, the Application (client) ID of the app registered in Microsoft Entra. |
| `MILA_MICROSOFT_TENANT` | `--microsoft-tenant` | `common` | With `microsoft`, the Directory (tenant) ID of a 365 organization, or `consumers` for Outlook.com. |
| `MILA_TLS_DOMAIN` | `--tls-domain` | none: HTTPS is a proxy's | The domain Mila gets its own HTTPS certificate for. See [Mila's own HTTPS](#milas-own-https). With it set, `MILA_BASE_URL` is `https://` and the domain, and `MILA_LISTEN` is not used. |
| `MILA_TLS_EMAIL` | `--tls-email` | none | An address Let's Encrypt writes to about the certificate, such as when renewing it keeps failing. |
| `MILA_TLS_CA` | `--tls-ca` | Let's Encrypt | The ACME directory certificates come from. |
| `MILA_TLS_LISTEN` | `--tls-listen` | `:443` | Where HTTPS is served, with `MILA_TLS_DOMAIN` set. |
| `MILA_TLS_HTTP_LISTEN` | `--tls-http-listen` | `:80` | Where plain HTTP answers the certificate's checks and sends people to HTTPS. Let's Encrypt checks port 80, so change it only behind a forward to it. |
| `MILA_UPDATE_CHECK` | `--update-check` | `on` | Ask GitHub once a day whether a newer Mila is out, for Settings, Admin, sending nothing else. Always off in the desktop app. |
| `MILA_TRUSTED_PROXIES` | `--trusted-proxies` | none (`compose.proxy.yaml` sets its network's gateway, `172.30.80.1`) | Comma-separated addresses or networks, such as `172.18.0.5` or `10.0.0.0/24`, whose `X-Forwarded-For` Mila believes. An address alone is that one machine. |
| `MILA_SIGN_IN_RATE` | `--sign-in-rate` | `10` | Sign-in attempts a minute from one address, after a burst of twice as many. |
| `MILA_INVITE_RATE` | `--invite-rate` | `2` | Invitations a person may send an hour, after a burst of ten times as many (at least 20). |
| `MILA_WEBHOOK_ALLOW_PRIVATE` | `--webhook-allow-private` | `off` | `on` lets webhooks reach addresses inside this server's own network, and plain http. Off, one can only reach the public internet, so nobody who can add a webhook can use it to probe what sits beside Mila. |
| `MILA_OAUTH` | `--oauth` | `off` | `on` lets apps that sign in by OAuth, such as hosted assistants, connect to the agents' MCP endpoint at `/api/v1/mcp` once a person says yes in Mila. Off, agents use API tokens only, and the tokens apps were given stop working. See [the guide for agents](agents.md). |
| `MILA_IMPORT_MAX_MB` | `--import-max-mb` | `256` | The largest JSON export an import takes, in megabytes. |
| `MILA_BACKUP` | `--backup` | `on` | The nightly backup of the database, `on` or `off`. |
| `MILA_BACKUP_DIR` | `--backup-dir` | `backups` in the data directory | Where backups go. |
| `MILA_BACKUP_HOUR` | `--backup-hour` | `3` | The hour, UTC, of the nightly backup. |
| `MILA_LOG_FORMAT` | `--log-format` | `json` | `json` or `text`. |
| `MILA_LOG_LEVEL` | `--log-level` | `info` | `debug`, `info`, `warn` or `error`. |

## Backups

Every night, and just before an upgrade changes the database, Mila copies its database into `backups` in the data directory. It keeps the newest of each of the last 7 days, 4 weeks and 6 months. Each backup is one SQLite file, whichever database Mila runs on. Settings, Admin shows the last one, and the instance's admins get an email if one fails.

Three things the backups do not cover:

- **Another machine.** The backups sit beside the database, so a failed disk takes both. Copy them elsewhere.
- **Files.** Attachments and people's pictures live in the data directory's `attachments` folder, not in the database.
- **The key.** `secret.key` in the data directory seals the sign-in to a Microsoft mailbox. A database restored without it works, but the mailbox needs connecting again, or the key put back.

So copy the whole data directory off the machine now and then. With Docker:

```sh
docker run --rm -v mila_mila-data:/data:ro -v "$PWD":/out alpine \
  tar czf /out/mila-data-$(date +%F).tgz -C /data .
```

(`mila_mila-data` is the volume's name when the folder holding `compose.yaml` is called `mila`; `docker volume ls` lists it.)

The commands, run beside a running Mila:

```sh
docker compose exec mila mila backup now
docker compose exec mila mila backup list
```

To restore one, stop Mila first. The database as it is gets backed up before it is replaced, and every device loads its copy afresh, keeping edits it had not sent.

```sh
docker compose stop mila
docker compose run --rm mila restore <file from backup list>
docker compose start mila
```

## Upgrading

Settings, Admin says when a newer Mila is out, with a link to what changed. To know, Mila asks GitHub for its latest release once a day and sends nothing else; `MILA_UPDATE_CHECK=off` stops it. It never updates anything itself.

With Docker:

```sh
docker compose pull
docker compose up -d
```

A new version updates the database when it starts, after backing it up. Mila refuses to start on a database a newer version has updated, so going back means restoring the backup made just before. To choose when to upgrade, name a version in `.env`, such as `MILA_IMAGE=ghcr.io/locodarwin/mila:1.0.0`, and change it when you are ready. Open apps notice a new version and offer to reload.

Installed with the install script, run it again. Installed with a package, install the new release's package. Installed by hand, replace the program and restart Mila.

## Postgres

SQLite suits Mila well, for one person or a team. To use Postgres instead, set `POSTGRES_PASSWORD` in `.env` to a long random one (`openssl rand -hex 24` makes one), set `MILA_DATABASE_URL=postgres://mila:<that password>@postgres:5432/mila?sslmode=disable`, and start with `docker compose --profile postgres up -d` (option 1's `compose.yaml` has the Postgres service). To move an existing instance across, stop Mila and copy it with `mila migrate-db --to <url>`; it works in either direction.

## Keeping it safe

- Serve Mila over HTTPS whenever people reach it from the internet. Every option above can do this, through a reverse proxy or with Mila's own certificate, and the base URL must then start with `https://`. Plain HTTP on port 8080 is only for a home network you trust.
- Set `MILA_TRUSTED_PROXIES` to your proxy's own address and nothing wider. Mila takes a trusted proxy's word for where each visitor comes from, so a wide range lets any machine in it fake that address and get around the sign-in limits.
- Keep the data directory and its backups private. The backups hold sign-in secrets. The data directory also holds `secret.key`, which unlocks Mila's sign-in to a Microsoft mailbox. A database backup on its own can't use that sign-in, but a copy of the whole data directory can, so store those copies as carefully as the mailbox's password.
- Upgrade when a new version comes out. Settings, Admin tells you when one is available; [Upgrading](#upgrading) above has the steps.
- Each person can see which devices their account is signed in on, and sign any of them out, in Settings, Security. An admin can sign someone out of every device at once from Settings, Admin.
