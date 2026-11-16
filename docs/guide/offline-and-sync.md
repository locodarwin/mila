# Offline and sync

Mila keeps a copy of your workspaces on each device. Your edits go to that copy first and to the server after, so they show at once and do not depend on a connection.

## The sync dot

The dot at the foot of the sidebar tells you how your changes are doing, in words beside it:

| It says | It means |
|---|---|
| Synced | Everything is sent |
| Syncing | The dot pulses while Mila sends and fetches changes |
| Offline | No connection. With unsent changes it says how many, such as Offline · 3 pending |
| Update needed | The server needs a newer version of the app. See [New versions](#new-versions) |

On a phone the dot is in the top bar. A dot alone means everything is synced, and a short word shows otherwise.

## Working offline

Add, edit, complete and move tasks as usual. The changes are saved on the device and sent when the connection comes back. Once Mila has been opened on a device, it starts there offline too.

Notes and wiki pages can be written offline as well, and merge when you reconnect.

Some things need the server, and wait for a connection:

- the trash
- workspace members and invitations
- exports and JSON imports
- the Security, Notifications, Calendar, Email to task, API tokens, Webhooks and Admin settings
- an automation's recent runs
- a task's history and a wiki page's history
- adding a file or uploading an image
- changing your picture

Importing a CSV file works offline, like any other edit.

Pictures of people that this device has shown before still show offline. Any it has not shown appear as initials until you reconnect.

A file you have opened on a device stays on that device, so it opens offline too. Any other file needs a connection, and says so. Mila keeps up to 500 MB of files on a device, letting go of the oldest first. Signing out removes them, and so does signing in as someone else on the same device.

## When the server is busy with your changes

The server takes a great deal from one person in an hour, far more than a day of writing, but not without end. Past that, a message says "Mila has saved a great deal in the last hour." Your other changes are kept on the device and sent a few minutes later, without you doing anything.

While the server is almost out of disk space, it takes only small changes, and the rest wait on the device the same way. The message says so, and asks you to tell whoever runs it.

## When two people edit

Changes merge field by field. If you change a task's due date while someone else renames it, both changes stay. If two people change the same field, the later change wins.

A task's notes and a wiki page's body work differently: they merge as you type. Two people can write in the same text at once, on different devices or with one of them offline, and everything each of them typed stays.

If two devices complete the same repeating task while offline, only one next task is made.

## Seeing who else is there

While someone else has the same task or wiki page open, their picture and name show above the notes or page, such as "Dana is here". Their cursor shows in the text in their color, with their name, as they type.

Presence needs a connection and is not stored anywhere. Guests see it only for tasks and pages in projects shared with them.

## When an edit is refused

The server has the last word on what each person may do. Your device shows an edit right away, and if the server later refuses it, Mila quietly puts the old value back. This is rare for members. It can happen to someone whose access changed while offline, and to [guests](guests.md#what-you-cannot-change).

## Undo

`Ctrl+Z` takes back your last change when you are not typing in a field. Pressing it again goes further back, up to your last 100 changes.

A toast that offers Undo takes back the change it names, whatever you have done since.

While you type in notes or a page, undo takes back your own typing and never anyone else's. Deleted tasks, projects, pages and templates also wait in the [trash](trash-and-archive.md) for 30 days.

## New versions

When a new version of Mila is out, a message offers to reload into it.

If the server needs the new version before it takes more changes, syncing stops and the sync dot says Update needed. Your changes are kept on the device and sent after you reload.

Notes written before an update may show without editing until the device has synced once. The page says "This becomes editable once this device has synced."

## When your copy is reloaded

Sometimes a device loads a workspace afresh by itself: when you are given or lose a project as a guest, when your role changes to or from guest, after a long time away, or after the server is restored from a backup. Edits you had not sent are kept and sent on top.

Signing out is different: it deletes this device's copy, unsent changes included, after warning you about them. If your session ends while Mila is open here, the copy goes too, but your unsent changes wait for you to sign in again. See [Signing out](signing-in.md#signing-out).

Signing in as someone else on the same device, with a sign-in link, deletes the first person's copy in the same way, unsent changes included. The link's page warns first and says how many changes would go. See [Signing in with an email link](signing-in.md#signing-in-with-an-email-link).

## Next

- [Notifications](notifications.md)
- [Your account and backups](account-and-backups.md)
