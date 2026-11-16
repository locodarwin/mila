# The desktop app

The desktop app shows Mila in a window of its own, on Windows and Linux. It works in two ways: connected to a Mila server, or keeping your work on this computer with no server at all.

## Getting it

- On Windows, use the installer, or the portable zip if you would rather not install anything. Unzip it anywhere, keep its two programs together, and run `mila-desktop.exe`. Your work is kept in the same folder either way.
- On Linux, it is an AppImage.

The app needs Microsoft Edge WebView2 on Windows. Windows 11 includes it, and the installer sets it up for you. The installer is not signed yet, so Windows may warn you the first time.

There is no Mac version.

## Where your work lives

The first time it opens, the app asks "Where should your work live?":

- Connect to a server: your workspaces as they are, shared with others and in step on all your devices.
- Keep it on this computer: just you, with no server and no account. Nothing leaves this computer.

You can have both, several servers, and several places on this computer.

Each time the app opens it shows "Choose a place": every place with how it is (Ready, Stopped, Offline, or Sign in again), who you are there, its workspaces and when you last opened it. The last one you opened is outlined and in focus, so `Enter` opens it. Tick "Open my last place straight away", at the foot, to go there at once instead.

One place shows at a time. The Mila menu at the top of the window lists every place, to go straight to another, and its first item, "All places…" (`Ctrl+Shift+P`, or `Cmd+Shift+P` on a Mac), comes back to the list. "Connect to a server…" and "New place on this computer…" are there too.

Each place's menu, the dots beside Open, has:

- Rename: a name of your own ("Work", "Home"), kept in the app only.
- Sign out, for a server: the place stays, and Open signs in again.
- Show its folder, for a place on this computer.
- Remove from this app.

## Signing in to a server

1. Choose Connect to a server, and type its address.
2. Choose Continue in my browser. The window says "Waiting for your browser".
3. Your own browser opens at the server. Sign in there the way you usually do, with a passkey or an emailed link.
4. The browser asks "Sign in to the Mila app?" and names the server and your account. Choose Sign in to the app.
5. When the browser says you can close its tab, the app's window opens your workspaces. A message at the bottom says which server it signed in to and as whom, such as "Signed in to mila.example.com as Ada Lovelace (ada@example.com)".

If you did not just ask for this in the app, choose Cancel in the browser instead.

If the message names someone else, choose "Not you? Sign out" on it. On a computer other people sign in to as well, one of them could get the app signed in to their account first. To try again, choose All places in the Mila menu, then Sign in… beside the server.

This needs a sign-in in the browser from the last 10 minutes, with a passkey if you have one. If yours is older, the browser asks you to sign in again first.

The app's window is a device like any other. It shows in Settings, Security, and its sign-in lasts as long as any other. See [How long you stay signed in](signing-in.md#how-long-you-stay-signed-in).

Some account changes ask for a recent sign-in. If you have passkeys, make those changes in your browser: a sign-in passed on to the app does not count as using a passkey, and the app's window on Linux cannot use passkeys at all. See [Signing in again for sensitive changes](signing-in.md#signing-in-again-for-sensitive-changes).

## Using a server in the app

The window shows the server's own pages, so it always has the Mila your server has. Everything in this guide works there as it does in a browser.

It works offline as a browser does, once it has opened with a connection. See [Offline and sync](offline-and-sync.md).

Links to anywhere else open in your browser. Files you download go in your downloads folder, and the app says when one is saved. A place can start 10 downloads a minute at most, so no page can fill your disk.

When the server you connect to is newer than the app, the Mila menu shows "A newer version of this app exists…", which opens the page to get it. The app does not update itself.

To stop using a server, choose Remove from this app in its menu on the list. Your work stays on the server.

## Keeping it on this computer

With this, there is no server and no account. The app runs Mila privately from when you first open the place until you quit the app. It is just you, so there are no invitations, no guests, and nothing that needs email: no sign-in links, digest, email to task, calendar feeds, API tokens or webhooks. Settings leaves those out.

You can have more than one, each with its own name and folder, for work you would rather keep apart. "New place on this computer" on the list makes another.

Reminders show while the app is open, even while another place is on the screen. Any missed while it was closed show when it next opens.

Backups are made while a place runs, in its folder. The first place's is:

- `%LOCALAPPDATA%\Mila` on Windows
- `~/.local/share/mila` on Linux

Each place after it has a folder in `places` there. Show its folder, in the place's menu, opens it.

Removing a place from the app keeps its work in its folder, unless you tick "Also delete its work", type the place's name, and then choose "Delete its work" again in the box your system shows (OK on Windows). Deleting cannot be undone. Removing the app itself keeps every folder.

To move a workspace to a server, export it in Settings, Import and export, and import it on the server. See [Exports and imports](exports-and-imports.md). The two are not kept in step afterward.

## Quick add from anywhere

`Ctrl+Shift+Space` brings Mila forward and opens [quick add](quick-add.md), whichever program you are in.

You can change it in Settings, Keyboard. It has to be two modifier keys and a letter, a digit or Space, since it takes that key from every other program. It cannot be one that other programs use, such as a terminal's copy and paste.

On Linux this needs X11. Under Wayland, which many systems now use, set a shortcut in your system's own settings that opens the app instead.

## Desktop notifications

While the app is open, what the bell tells you also shows as a notification from your system. One from a server names the server's address, even a server on this computer such as `localhost:8790`, and one from a place on this computer that is not on the screen names that place. See [Notifications](notifications.md).

Closing the window quits the app.

## Next

- [Signing in](signing-in.md)
- [Offline and sync](offline-and-sync.md)
