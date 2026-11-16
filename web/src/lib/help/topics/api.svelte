<script lang="ts">
	import Tip from '../../components/help/Tip.svelte';
	import { resolve } from '$app/paths';
</script>

<p>
	A token lets a script or another app use Mila as you, and a webhook tells another app when
	something changes. Both are for people who build things; nothing else in Mila needs them.
</p>

<h2 id="tokens" tabindex="-1">API tokens</h2>
<p>
	Make one in Settings, API tokens. Mila shows it once, so copy it then. Send it with each request,
	in a header: <code>Authorization: Bearer</code>, then the token. The
	<a href={resolve('/api/docs')}>API reference</a> lists every endpoint.
</p>
<Tip>A token acts as you, and can never do more than you can.</Tip>
<p>
	Leave a workspace and it stops reaching that workspace. Each one is read only or read and write,
	for one workspace or all of yours, and each change it makes is yours in a task's history, marked
	with its name.
</p>
<p>
	A token works on tasks and the rest of your work. It cannot sign in, manage your account or other
	tokens, change who is in a workspace, or use admin powers. An export takes a token that can write.
</p>
<p>
	If one might have got out, revoke it in Settings, API tokens. It stops working at once. An admin
	signing you out everywhere ends your tokens too.
</p>

<h2 id="actions" tabindex="-1">Actions for scripts and agents</h2>
<p>
	Besides changing a row at a time, the API has actions that do what the app does in one step: add a
	task with its labels, assignees and checklist, complete or reopen one, set its status, move it
	with its subtasks, make a project with its statuses, and add to a task's notes or a wiki page
	without losing what someone else is typing. They take names as well as IDs, and say so when a name
	matches nothing or more than one thing.
</p>
<p>
	The find-tasks action lists tasks from the Inbox, My Day, Upcoming, a saved view or a project, by
	due date, words, and open or done. A read-only token may use it.
</p>
<p>
	Send a <code>Mila-Batch</code> header with a name of your own on every change of one run, and the
	undo-batch action puts the whole run back for 30 days, leaving anything someone has changed since.
	An action sent without one names its own batch in its answer. The
	<a href={resolve('/api/docs')}>API reference</a> describes each action.
</p>
<p>
	The <code>mila</code> program can offer all of this to an agent as tools, with
	<code>mila mcp</code>, or run it from a shell, with <code>mila api</code>. Both take the server's
	address and a file holding the token.
</p>
<p>
	An agent that takes tools by address can reach the same ones on the server itself, at
	<code>/api/v1/mcp</code>, sending the token with each request. Where the server allows it, an app
	that signs in by OAuth can connect there too: it sends you to Mila to choose a workspace and what
	it may do, and its token then shows in Settings, API tokens, marked as an app's.
</p>
<p>
	To hand work to an agent, assign it tasks. <code>mila queue</code> runs beside the agent, gives it each
	task assigned to it, and answers with what the agent says as a comment. It takes only tasks that the
	workspace's members alone have written to, subtasks included, so a guest or an email can't give the
	agent work. If someone else writes to the task while the agent works on it, the answer isn't posted.
</p>
<Tip>
	Give a program that works for you an account of its own, as a guest on just the projects it needs,
	with a token for that workspace alone. Its changes then carry its own name.
</Tip>

<h2 id="webhooks" tabindex="-1">Webhooks</h2>
<p>
	Owners and admins add them in Settings, Webhooks, for the workspace picked under Workspace in
	Settings. Choose an address, what it hears about, and every project or just one. Mila then sends a
	POST with JSON each time one of those things happens: its event, such as <code
		>task.completed</code
	>, its time, who did it, the row as it stands now, and which fields changed.
</p>
<p>
	My Day, reminders and personal views never go out. Typing in a task's notes or a wiki page does
	not send anything, or it would send every few seconds.
</p>

<h2 id="signatures" tabindex="-1">Checking a delivery came from Mila</h2>
<p>
	Each delivery has a header, <code>X-Mila-Signature</code>, of the form
	<code>t=1793750000,v1=…</code>. To check it, take the time <code>t</code>, a dot and the raw body,
	make an HMAC-SHA256 of them with the webhook's signing secret, and compare the result in hex with
	<code>v1</code>. Refuse one whose time is more than a few minutes old, so an old delivery cannot
	be replayed.
</p>
<p>
	<code>X-Mila-Delivery</code> is the same on every try of a delivery, so use it to act only once. Deliveries
	can arrive out of order, so go by the time each one carries.
</p>

<h2 id="failures" tabindex="-1">When deliveries fail</h2>
<p>
	Any 2xx answer within 10 seconds counts as delivered. Anything else is tried again, six tries in
	all over about a day. Redirects are not followed.
</p>
<p>
	Open a webhook in Settings to see its log, kept for 30 days, and resend what failed. A webhook
	stops after 3 days of nothing but failures, and whoever it belongs to is told. Turn it back on
	once it is fixed.
</p>
<p>
	A webhook belongs to whoever made it, until someone else changes its address, makes it a new
	secret or turns it back on: then it is theirs, and whoever had it is told. It is paused when the
	person it belongs to leaves the workspace or stops being an admin, and the owners and admins are
	told. Check its address before turning it back on.
</p>
<p>
	Addresses must start with https, and must not point inside the server's own network. Someone
	running their own Mila can allow those with the <code>MILA_WEBHOOK_ALLOW_PRIVATE</code> setting.
</p>
