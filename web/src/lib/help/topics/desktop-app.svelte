<script lang="ts">
	import Keys from '../../components/help/Keys.svelte';
</script>

<h2 id="places" tabindex="-1">Where your work lives</h2>
<p>
	The desktop app shows Mila in a window of its own, on Windows and Linux. The first time it opens,
	it asks where your work should live: on a server, or on this computer.
</p>
<p>
	You can have both, several servers, and several places on this computer. Each time the app opens
	it shows Choose a place: every place, how it is (Ready, Stopped, Offline or Sign in again), who
	you are there, its workspaces and when you last opened it. The last one you opened is first in
	focus, so Enter opens it. Tick Open my last place straight away to skip the list.
</p>
<p>
	One place shows at a time. To go to another, choose it in the Mila menu at the top of the window,
	or choose All places there to come back to the list, where you can add more.
</p>
<p>
	Each place's menu, the dots beside Open, renames it (the name is kept in the app only), signs out
	of a server, shows a place on this computer's folder, or removes it from the app.
</p>

<p>
	On Windows you can use the installer, or the portable zip if you'd rather not install anything:
	unzip it anywhere, keep its two programs together, and run <code>mila-desktop.exe</code>. It needs
	Microsoft Edge WebView2, which Windows 11 includes and the installer sets up for you. Your work is
	kept in the same folder either way.
</p>

<h2 id="signing-in" tabindex="-1">Signing in to a server</h2>
<p>
	Give the app your server's address, and your own browser opens for you to sign in the way you
	usually do, with a passkey or an emailed link. Say yes there, and the app's window is signed in.
	It shows in your security log like any other device.
</p>
<p>
	The window then says which server it signed in to and as whom. If that isn't you, choose Not you?
	Sign out on that message: on a computer other people sign in to as well, one of them could get the
	app signed in to their account first. To try again, choose All places in the Mila menu, then Sign
	in beside the server.
</p>
<p>
	Some account changes ask for a recent sign-in. If you have passkeys, make those changes in your
	browser: a sign-in passed on to the app doesn't count as using a passkey, and the app's window on
	Linux can't use them.
</p>
<p>
	Links to anywhere else open in your browser, and files you download go in your downloads folder.
	To stop using a server, choose Remove from this app in its menu on the list. Your work stays on
	the server.
</p>
<p>
	The window shows the server's own pages, so it always has the Mila your server has. It works
	offline as a browser does, once it has opened with a connection.
</p>

<h2 id="this-computer" tabindex="-1">Keeping it on this computer</h2>
<p>
	With this, there is no server and no account, and nothing leaves your computer. The app runs Mila
	privately from when you first open the place until you quit the app. It's just you, so there are
	no invitations, and nothing that needs email.
</p>
<p>
	You can have more than one, each with its own name and its own folder, for work you'd rather keep
	apart. New place on this computer, on the list, makes another.
</p>
<p>
	Reminders show while the app is open, even while another place is on the screen, and any missed
	while it was closed show when it next opens. Backups are made while it's running, in its folder:
	the first place's is <code>%LOCALAPPDATA%\Mila</code> on Windows and
	<code>~/.local/share/mila</code> on Linux, and each one after it has a folder in
	<code>places</code> there. Show its folder, in the place's menu, opens it.
</p>
<p>
	Removing a place from the app keeps its work in its folder, unless you tick Also delete its work,
	type the place's name, and then choose Delete its work again in the box your system shows (OK on
	Windows). Deleting can't be undone. Removing the app itself keeps every folder.
</p>
<p>
	To move a workspace to a server, export it in Settings, Import and export, and import it on the
	server. They aren't kept in step afterward.
</p>

<h2 id="shortcut" tabindex="-1">Quick add from anywhere</h2>
<p>
	<Keys
		id="desktop.quick-add"
		otherwise="A shortcut you can set in Settings, Keyboard"
		trail=" brings Mila forward and opens quick add, whichever program you're in."
	/>
	You can change it in Settings, Keyboard. It has to be two modifier keys and a letter, a digit or Space,
	since it takes that key from every other program, and it can't be one that other programs use, such
	as a terminal's copy and paste.
</p>
<p>
	On Linux this needs X11. Under Wayland, which many systems now use, set a shortcut in your
	system's own settings that opens the app instead.
</p>

<h2 id="notifications" tabindex="-1">Desktop notifications</h2>
<p>
	While the app is open, what the bell tells you also shows as a notification from your system. One
	from a server names the server's address, even a server on this computer, so a server's words
	can't pass for the app's own.
</p>
