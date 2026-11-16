<script lang="ts">
	import Tip from '../../components/help/Tip.svelte';
</script>

<h2 id="addresses" tabindex="-1">Your addresses</h2>
<p>
	In Settings, Email to task, you can make addresses that turn email into tasks. Forward a message
	to one, or write to it yourself, and it becomes a task within a minute or so. The page tells you
	if the server you use has no mailbox set up for this.
</p>
<p>
	You can have up to 10 addresses in each workspace, each going to the Inbox or a project. Tasks are
	assigned to you unless you turn that off for the address. An address per project is an easy way to
	send mail straight to the right place.
</p>
<p>
	Guests don't have addresses. Your addresses end if you leave the workspace or become a guest in
	it.
</p>

<h2 id="the-task" tabindex="-1">What the task holds</h2>
<p>
	The subject becomes the title, without Fwd: or Re:. With no subject, the title is the first line
	of the message. What you write in the subject is taken as it is, so quick add's shortcuts don't
	apply.
</p>
<p>
	The message becomes the notes, after a line saying who it was from and when. It comes across as
	plain text, so a message only in HTML loses its formatting and keeps its words and where its links
	lead.
</p>
<p>
	Attachments come along, up to 25 MB each. One too large is named in the notes. Small pictures in
	the text of a message, such as those in a signature, stay behind.
</p>
<p>
	The same message arriving twice makes one task. In the task's history, it says it came through
	email.
</p>

<h2 id="senders" tabindex="-1">Who can send</h2>
<p>
	Mail has to come to your address and from an address you've listed. Your sign-in address is always
	listed, and you can add up to 10 more for each address, such as a work address you forward from.
</p>
<Tip>The address of a sender can be forged, so treat your address like a password.</Tip>
<p>
	If you think someone else has it, choose Make a new one: the old address stops working at once,
	and everything else about it stays. Making an address goes in your security log, with an email to
	you.
</p>

<h2 id="refused" tabindex="-1">Mail that is refused</h2>
<p>
	Mila never replies to email, since a reply could reach someone whose address was forged. When a
	task doesn't appear, look under Recent mail in Settings, Email to task, which keeps 30 days. For
	mail it refuses, it keeps who it was from and the subject, not the message.
</p>
<p>Mail is refused when:</p>
<ul>
	<li>its sender isn't listed for that address</li>
	<li>it was sent automatically, such as an out-of-office reply or a mailing list</li>
	<li>the mail server called it spam, or said it failed its sender checks</li>
	<li>the address has already made 50 tasks in a day</li>
	<li>the address goes to a project that has been archived or deleted</li>
</ul>
<p>
	To let a sender through, add them and have them send it again. When a task is made, the bell tells
	you, which you can change in Settings, Notifications.
</p>
