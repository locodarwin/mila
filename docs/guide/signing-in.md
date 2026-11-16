# Signing in

Mila has no passwords. You sign in with a link sent to your email, or with a passkey on a device you have set up.

## Getting an account

Accounts come by invitation. Someone who runs a workspace sends an invitation to your email address, and the link in it works for 7 days.

What happens when you open it depends on who you are:

- If you are new to this Mila, you give your name and choose Join. You are signed in straight away, with an account for the address the invitation went to.
- If you have an account and this browser is signed in, you join as that account, whichever address the invitation named. The page says who you will join as.
- An invitation to be an admin or an owner is the exception: it is for the address it was sent to. If this browser is signed in to another address, Mila says so and asks you to sign in as the invited address, or to ask for an invitation to your own.
- If you have an account and this browser is not signed in, you join, and then sign in the usual way.

An invitation stops working if it is canceled, or if the person who sent it can no longer invite you. Ask them for a new one.

## Signing in with an email link

1. On the sign-in page, type your address and choose Email me a sign-in link.
2. Open the link in the email.

The link works once and expires after 15 minutes. You can ask for five links an hour. Past that, the page looks the same but no email goes out, so wait a while before trying again.

The page gives the same answer whether or not the address has an account. If no email arrives, check that you typed the address you were invited with, and look in your spam folder.

A link opened in the browser that asked for it in the last 15 minutes signs you in as it opens. Anywhere else, such as another browser or a link someone sent you, the page asks "Sign in to Mila?" and names the account, with a button to sign in to it. Choose it only if you asked for that link yourself.

If the browser is already signed in to a different account, the page asks "Switch accounts?" first. Switching deletes what this device holds for the account signed in now, as signing out does, and the page says how many of its changes are not sent yet. Choose Stay signed in to keep that account, and nothing changes.

## Passkeys

A passkey lets a device sign you in with your fingerprint, face or screen lock. Once you have one, Sign in with a passkey shows on the sign-in page.

To add one, go to Settings, Security and choose Add a passkey. The same page lets you rename or remove your passkeys. Removing one takes a second click to confirm, needs a recent sign-in (see [Signing in again for sensitive changes](#signing-in-again-for-sensitive-changes)), and sends you an email saying it is gone.

Email links always work too, so removing a passkey never locks you out.

Passkeys need your Mila to have a domain name. If it is reached by an IP address, Settings, Security says they are off.

## How long you stay signed in

A sign-in on a device lasts 30 days from the last time you used Mila there. Each use starts the 30 days again, up to 90 days after you signed in. After 90 days you sign in again, however often you use it.

When a session ends, push notifications to that device stop too. See [Phone push and the 30-day rule](notifications.md#phone-push-and-the-30-day-rule).

## Signing in again for sensitive changes

Some actions need a sign-in from the last 10 minutes:

- adding or removing a passkey
- changing your email address
- deleting your account
- making a calendar feed or an API token
- adding a webhook, giving it a new signing secret, or turning it back on
- making an email-to-task address, giving it a new one, or adding senders to it
- signing in the [desktop app](desktop-app.md)
- making someone an instance admin

If your account has a passkey, that sign-in must have been with a passkey.

If you were signed in longer ago, Mila refuses and says why. The message offers Sign in again, which brings you back to where you were. Making someone an instance admin shows the message without that button: sign in again, then come back to Settings, Admin.

## Signing out

Sign out is at the foot of the sidebar. It needs a connection.

Signing out deletes this device's copy of your workspaces and the files kept for offline use, and turns off push here. It also clears everything else Mila kept in this browser: preferences, themes and layouts. Your preferences and themes come back when you sign in again, since they are kept on the server, but what each device remembers on its own does not. See [What stays with each device](preferences.md#what-stays-with-each-device).

Changes that have not been sent yet are deleted with it. Mila tries to send them first, and if it cannot, it says how many there are and offers Sign out anyway.

If your session ends while Mila is open, from another device or because it ran out, the device's copy is deleted the same way. Your unsent changes alone stay on the device. Mila tells you how many, and sends them when you sign in again. If someone else signs in on that browser instead, they are deleted.

## Seeing where you are signed in

Settings, Security lists your signed-in devices, with when each was last active and from what address. Sign out beside a device ends its session at once.

Recent activity on the same page lists sign-ins and changes to how you sign in, for the past year. A sign-in from a browser you have not used before is also emailed to you. If something there was not you, sign that device out.

The [desktop app](desktop-app.md) shows in the list like any other device.

If an instance admin signs you out everywhere, every session ends at once. So do your API tokens, calendar feeds and email-to-task addresses, any change of address or sign-in link you had asked for, and your passkeys. Mila emails you to say so. Sign in again with an email link, then add your passkeys again.

## Your name, picture and address

Settings, Profile holds your name and picture. Your picture can be chosen, dropped or pasted, and you crop it before it is saved. Changing it needs a connection.

To change your email address, type the new one under Email address and choose Send link. Open the link sent to the new address within 24 hours. Until you do, nothing changes, so a mistyped address cannot lock you out. Mila tells your old address about the change.

The link works only while the session that asked for it lasts. If you sign out on that device, or sign it out from Settings, Security, the link stops working, and you ask for a new one.

## Next

- [Your first project](first-project.md)
- [Offline and sync](offline-and-sync.md)
