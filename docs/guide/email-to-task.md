# Email to task

Email to task gives you private addresses that turn email into tasks. Forward a message to one, or write to it yourself, and it becomes a task within a minute or so.

It is in Settings, Email to task, for the workspace picked under Workspace in Settings. If the page says the server has no mailbox set up to receive email, whoever runs your Mila has not turned it on. See [Running Mila](running-mila.md#email).

## Your addresses

Choose New address to make one. It shows as a card with the address, and Copy beside it.

Each address has its own settings, saved as you change them:

- Tasks go to: the Inbox, or one of the workspace's projects.
- Assign them to me: on to begin with. Turn it off and the tasks are made unassigned.
- Accept mail from: who may send to it. See [Who can send](#who-can-send).

You can have up to 10 addresses in each workspace. An address per project is an easy way to send mail straight to the right place.

The tasks are made by you, as if you had added them yourself.

Making an address needs a sign-in from the last 10 minutes, with a passkey if you have one. It goes in your security log, with an email to you. See [Signing in again for sensitive changes](signing-in.md#signing-in-again-for-sensitive-changes).

## What the task holds

The subject becomes the title, without Fwd: or Re:. With no subject, the title is the first line of the message. The subject is taken as it is, so [quick add](quick-add.md)'s shortcuts do not apply.

The message becomes the notes, after a line saying who it was from and when. It comes across as plain text, so a message only in HTML loses its formatting and keeps its words and where its links lead.

Attachments come along, up to 25 MB each. One too large is named in the notes instead. Small pictures in the text of a message, such as those in a signature, stay behind.

The same message arriving twice makes one task. The task's history says it came through email.

An [automation](automations.md) can start when a task arrives by email, to label it, set its due date or assign it, for example.

## Who can send

Mail has to come to your address, and from an address listed under Accept mail from. Your sign-in address is always listed. You can add up to 10 more for each address, such as a work address you forward from. Adding one needs a recent sign-in, as making the address does.

The X beside a listed address takes it off.

The sender of an email can be forged, so treat your address like a password. Anyone who has it and sends from a listed address can add tasks as you.

If you think someone else has it, choose Make a new one, then Confirm new address. The old address stops working at once, and everything else about it stays. Delete, also with a second click, stops it working for good.

## Recent mail

Mila never replies to email, since a reply could reach someone whose address was forged. When a task does not appear, look under Recent mail, which keeps the last 30 days: when each message came, who it was from, its subject, and what happened. Open goes to the task it made.

For mail it refuses, Mila keeps who it was from and the subject, not the message.

Mail is refused when:

- its sender is not listed for that address
- it was sent automatically, such as an out-of-office reply or a mailing list
- the mail server marked it as spam, or it failed its sender checks
- the address has already made 50 tasks that day
- the address goes to a project that has been archived or deleted
- it is too large, over 64 MB

To let a sender through, add them under Accept mail from and have them send it again.

When a task is made, the bell tells you. Tasks from email in Settings, Notifications changes that. See [Notifications](notifications.md).

## When an address ends

Guests have no addresses. Yours end if you leave the workspace or become a guest in it.

If an instance admin signs you out everywhere, your addresses end too, and you make new ones.

The [desktop app](desktop-app.md) has no email to task when your work is kept on your own computer.

## Next

- [Automations](automations.md)
- [Notifications](notifications.md)
