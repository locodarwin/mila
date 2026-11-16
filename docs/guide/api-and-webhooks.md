# API tokens and webhooks

These let a script or another app work with Mila. Nothing else in Mila needs them.

## API tokens

A token lets something else use Mila as you.

1. Open Settings, API tokens.
2. Give it a name, choose Read only or Read and write, one workspace or All my workspaces, now and later, and when it expires.
3. Press Make a token, and copy it. Mila shows it only this once.

Making a token needs a sign-in from the last 10 minutes, with a passkey if you have one. It goes in your security log, with an email to you. You can have 20 tokens.

A token's name shows in task history beside its changes, so it cannot be "email" or start with "automation", which history uses for changes made by [email to task](email-to-task.md) and by [automations](automations.md).

Send it with each request, as a header:

```
curl https://your-mila/api/v1/workspaces/<id>/tasks \
  -H "Authorization: Bearer mila_..."
```

The API reference, at `/api/docs`, lists every endpoint.

A token can never do more than you can, and stops reaching a workspace you leave. It works on tasks and the rest of your work. It cannot sign in, manage your account or other tokens, change who is in a workspace, or use admin powers. An export takes a token that can write. Each change it makes is yours in a task's history, marked with its name.

If one might have got out, revoke it in Settings, API tokens. It stops working at once. An admin signing you out everywhere ends all your tokens too.

## Actions for scripts and agents

Besides changing a row at a time, the API has actions that do what the app does in one step, under `/api/v1/workspaces/<id>/actions/`:

- `add-task`, with its project, status, labels, assignees and checklist given by name or ID.
- `complete-task`, `reopen-task` and `set-status`, which keep a task's status and completion in step and make a repeating task's next one.
- `move-task`, which takes a task's subtasks with it, to another project or the Inbox.
- `create-project`, with the usual five statuses or your own, and `delete-status`, which moves its tasks first.
- `append-notes` and `replace-section`, which change a task's notes or a wiki page without losing what someone else is typing.
- `find-tasks`, which lists tasks from the Inbox, My Day, Upcoming, a saved view or a project, by due date, words, and open or done. A read-only token may use it.

A name that matches nothing, or more than one thing, is refused with what it could have meant, so nothing is guessed.

Send a `Mila-Batch` header with a name of your own on every change of one run, and `undo-batch` puts the whole run back for 30 days, leaving anything someone has changed since. An action sent without one names its own batch in its answer.

The `mila` program can offer all of this to an AI agent as tools (`mila mcp`), or run it from a shell (`mila api`). An agent that takes tools by address can reach the same ones on the server, at `/api/v1/mcp`, with the token. Where the server allows it, an app that signs in by OAuth can connect there instead: it sends you to Mila to say yes, choose a workspace and what it may do, and its token then shows in Settings, API tokens, marked as an app's. To hand work to an agent, assign it tasks: `mila queue` runs beside it, gives it each one, and answers with what it says as a comment. It takes only tasks that the workspace's members alone have written to, subtasks included, so a guest or an email can't give the agent work, and if someone else writes to the task while the agent works, it posts no answer. [The guide for agents](../agents.md) shows how to set them up.

A program that works for you is best given an account of its own, as a guest on just the projects it needs, with a token for that workspace alone. Its changes then carry its own name. [The guide for agents](../agents.md) has the details.

## Webhooks

A webhook tells another app when something changes. Owners and admins add them in Settings, Webhooks, for the workspace picked under Workspace in Settings. Choose an address, what it hears about, and every project or just one, then choose Add webhook and copy the signing secret. It shows only this once. A workspace can have 10 webhooks.

Mila then sends a POST with JSON for each change: its event, such as `task.completed`, its time, who did it, the row as it stands now, and which fields changed. My Day, reminders and personal views never go out, and neither does typing in notes or a wiki page.

Adding a webhook, Make a new secret and Turn back on each need a sign-in from the last 10 minutes, with a passkey if you have one.

Send a test, in a webhook's tools, sends a ping to its address. You can send 10 in a row, then 20 an hour. Past that, Mila asks you to try again in a few minutes.

To check a delivery came from Mila, read the `X-Mila-Signature` header, `t=<time>,v1=<signature>`. Make an HMAC-SHA256 of the time, a dot and the raw body with the signing secret, and compare it in hex with `v1`. Refuse one whose time is more than a few minutes old.

`X-Mila-Delivery` is the same on every try of a delivery, so use it to act only once. Deliveries can arrive out of order, so go by the time each one carries.

Any 2xx answer within 10 seconds counts as delivered. Anything else is tried again, six tries in all over about a day. Redirects are not followed. Open a webhook to see its log, kept for 30 days, and resend what failed. A webhook stops after 3 days of nothing but failures, and whoever it belongs to is told. Turn it back on once it is fixed.

A workspace's webhooks make at most 2000 tries an hour between them. Past that, deliveries wait their turn and go later.

A webhook belongs to whoever made it, until someone else changes its address (through the API), makes it a new secret or turns it back on. Then it is theirs, and whoever had it is told.

A webhook is paused when the person it belongs to leaves the workspace or stops being an admin, and the owners and admins are told. Check its address before turning it back on.

Addresses must start with https and must not point inside the server's own network. If you run your own Mila, `MILA_WEBHOOK_ALLOW_PRIVATE=on` allows those.
