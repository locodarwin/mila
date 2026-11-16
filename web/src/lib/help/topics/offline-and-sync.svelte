<script lang="ts">
	import Tip from '../../components/help/Tip.svelte';
	import Keys from '../../components/help/Keys.svelte';
</script>

<h2 id="dot" tabindex="-1">The sync dot</h2>
<p>
	The dot at the foot of the sidebar shows how your changes are doing, and says so beside it: Synced
	means everything is sent, and a pulsing dot means Mila is syncing. Offline shows how many changes
	are pending, waiting to be sent. On a phone it is in the top bar, where a dot alone means
	everything is synced.
</p>

<h2 id="offline" tabindex="-1">Working offline</h2>
<p>
	Your workspaces are kept on each device, so Mila keeps working without a connection. Add, edit,
	complete and move tasks as usual: the changes are saved on the device and sent once the connection
	is back.
</p>
<Tip>Once Mila has been opened on a device, it starts there offline too.</Tip>
<p>
	A few things need the server and wait for a connection: the trash, workspace members and invites,
	exports and JSON imports, and the Security, Notifications, Calendar and Admin settings. Importing
	a CSV file or a file from another app works offline, like any other edit.
</p>
<p>
	Changing your picture needs a connection too. People's pictures that this device has shown before
	still show offline; any it has not show their initials until the connection is back.
</p>
<p>
	A file you have opened on a device stays on that device, so it opens offline too. Any other file
	needs a connection, and says so. Mila keeps up to 500 MB of files on a device, letting go of the
	oldest first, and signing out removes them. Signing in as someone else on the same device, by a
	sign-in link, removes them too, with the rest of what the device held for you.
</p>
<p>
	The server takes a great deal from one person in an hour, but not without end: far more than a day
	of writing, less than writing a long page over and over. Past that, and while the server is low on
	disk space, a message says your changes are waiting. They are kept on the device and sent a few
	minutes later, without you doing anything.
</p>

<h2 id="conflicts" tabindex="-1">When two people edit</h2>
<p>
	Changes are merged field by field. If you change a task's due date while someone else renames it,
	both changes stay. Only when two people change the same field does one win: the later change.
</p>
<p>
	A task's notes are different: they merge as you type. Two people can write in the same notes at
	once, on different devices or with one of them offline, and everything each of them typed stays. A
	task's history keeps one entry for each stretch of writing, so it shows how the notes looked after
	each. Undo while you type in the notes takes back your own typing, never anyone else's.
</p>
<p>
	While someone else has the same task open, their picture and name show above its notes, and their
	cursor shows in the notes in their color, with their name, as they type.
</p>
<p>Wiki pages merge the same way, and show who else is in them the same way.</p>
<p>
	Notes written before an update to Mila show as they are, without editing, until the device has
	synced once; then they can be edited as usual.
</p>

<h2 id="undo" tabindex="-1">Undo</h2>
<p>
	<Keys
		id="undo"
		trail=" takes back your last change when you are not typing in a field, and pressing it again goes further back. "
	/>A toast that offers Undo takes back the change it names. Deleted tasks and projects stay in the
	trash for 30 days, where you can restore them.
</p>

<h2 id="updates" tabindex="-1">New versions</h2>
<p>
	When a new version of Mila is out, a message offers to reload into it. If the server needs the new
	version before it takes more changes, syncing stops and the sync dot says Update needed. Your
	changes are kept on the device and sent after you reload.
</p>
