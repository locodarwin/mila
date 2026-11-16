# Running Mila

Most people use a Mila someone else runs, and can skip this page.

Running Mila means keeping one program going on a machine that is always on, such as a small server or a NAS, reachable at a domain or subdomain of its own over HTTPS. By default it keeps its data in one folder. Email is optional but needed for sign-in links to reach people's inboxes.

If Mila is just for you, on one computer, the [desktop app](desktop-app.md) can keep your work there with no server to run.

## The ways to run it

- On a machine of its own, with Docker or Podman. Caddy comes along and gets the HTTPS certificate.
- Behind a proxy you already run, such as Nginx Proxy Manager, Traefik or nginx. Mila then answers on the machine itself only, and the proxy passes visitors on.
- As the program on its own, with the systemd unit that comes with it, behind your own proxy. The unit keeps Mila's data folder readable by Mila alone, and Mila makes an older install's database private when it starts.

Run on its own, Mila listens on the machine itself only unless you tell it otherwise, so visitors reach it through your proxy. In a container it listens inside the container, and the compose files decide what reaches it.

## First start

On its first start, Mila prints a one-time link in its log. Open it to make the first account, which becomes the instance's admin. After that, people join by invitation.

## Email

Mila sends sign-in links, invitations, reminders and the daily digest through any SMTP service. It insists on an encrypted connection to it. A plain connection is allowed only to a mail server on the same machine, or when you say so in the setting.

Given a mailbox it can read over IMAP, Mila can also turn email into tasks. Without one, Settings, Email to task says the server has no mailbox. See [Email to task](email-to-task.md).

## More

[`docs/self-hosting.md`](../self-hosting.md) has the steps: installing, email, email to task, backups, upgrades and settings.
