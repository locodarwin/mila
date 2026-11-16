# Your account and backups

## Deleting your account

Settings, Profile, at the end of the page. Type your email address to confirm, then choose Delete my account for good. It cannot be undone. It needs a sign-in from the last 10 minutes, with a passkey if you have one, and the message offers Sign in again if yours is older.

What happens:

- Your personal workspace goes, with everything in it.
- So does every shared workspace where you are the only member.
- In workspaces you share, what you wrote stays, credited to "Deleted user", and you are taken off the tasks assigned to you.
- Your reminders, My Day and personal saved views go.
- Your API tokens, calendar feeds, email-to-task addresses and exports go, and automations that run as you stop.
- Every session ends, on every device.

Mila refuses in two cases, and says which:

- You are the only owner of a workspace with other people in it. Make someone else an owner, or remove the other members, first.
- You are the only admin of this Mila. Make someone else an admin first.

Backups the server keeps may hold your data until they expire.

To take your work with you first, make an [export](exports-and-imports.md).

## Backups, as a user sees them

Backups are the server's business, and most people never see them. Whoever runs your Mila sets them up, and they cover the whole server rather than one person or workspace.

A few things are useful to know:

- You cannot restore a backup yourself. For something you deleted in the last 30 days, use the [trash](trash-and-archive.md). For your own copy of a workspace, make an [export](exports-and-imports.md).
- If the server is restored from a backup, everything goes back to how it was then. Your devices load their copies afresh, and any edits they had not sent are kept and sent on top. Work done on the server since that backup, and already synced, may be gone.
- If you are an instance admin, Settings, Admin shows when the last backup ran and which backups are kept, and has Back up now. You get an email if one fails.

Your own copy on each device is not a backup. It is deleted when you sign out.

If you use the [desktop app](desktop-app.md) with your work on your own computer, there is no server, and the app makes its backups in your own folder while it is open. See [Keeping it on this computer](desktop-app.md#keeping-it-on-this-computer).

[`docs/self-hosting.md`](../self-hosting.md#backups) covers backups for whoever runs Mila.

## Next

- [Running Mila](running-mila.md)
