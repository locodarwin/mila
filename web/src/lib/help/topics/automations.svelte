<script lang="ts">
	import Tip from '../../components/help/Tip.svelte';
</script>

<h2 id="rules" tabindex="-1">Making a rule</h2>
<p>
	In Settings, Automations, New automation makes a rule. Say where it works, in one project or the
	whole workspace, then what starts it and what it does. The sentence above the builder is how it
	reads in the list.
</p>
<p>
	Only if narrows it to some tasks, with the same conditions as a saved view's filter. With none, it
	works on any task there.
</p>
<p>
	Under the builder, a test shows what the rule would do to your tasks if the When happened to them
	now. It changes nothing. A switch turns a rule off without losing it.
</p>

<h2 id="starts" tabindex="-1">What can start a rule</h2>
<ul>
	<li>A task is created, completed or reopened.</li>
	<li>It moves to a status, or its priority changes.</li>
	<li>Someone is assigned, or a label is added.</li>
	<li>A custom field changes, or a comment is added.</li>
	<li>Its last blocker finishes.</li>
	<li>It arrives by email.</li>
	<li>Its due date comes near, or passes. Mila checks every few minutes, once for each date.</li>
</ul>
<p>
	A rule across the workspace chooses a kind of status, such as a done status, since each project
	has statuses of its own.
</p>

<h2 id="changes" tabindex="-1">What a rule can do</h2>
<ul>
	<li>Set the status, priority or due date, or complete the task.</li>
	<li>
		Assign someone, or unassign everyone. "Whoever made the task" and "whoever did this" work too.
	</li>
	<li>Add or remove a label.</li>
	<li>Move the task to another project, with its subtasks.</li>
	<li>Add a comment or a subtask.</li>
	<li>Notify someone.</li>
</ul>
<p>
	A rule does up to 10 of these, in order. If one can't be done, such as assigning someone who can't
	reach the task, the others still are, and the rule's list says what happened. A rule can't delete
	anything.
</p>

<h2 id="who" tabindex="-1">Whose rule it is</h2>
<p>
	Members make rules for a project, and owners and admins make rules for the whole workspace. Guests
	don't see rules, though what they do can start one. A workspace holds up to 100.
</p>
<Tip>
	A rule runs as whoever last saved it or turned it on, and can only do what they can. Turning on
	someone else's rule makes it yours.
</Tip>
<p>
	If that person can no longer run it, because they left or became a guest, the rule stops and says
	so. Saving it makes it yours and starts it again.
</p>

<h2 id="limits" tabindex="-1">What to expect</h2>
<p>
	Rules run on the server, so they work whichever device made the change. A change made offline
	starts its rules when it reaches the server.
</p>
<p>
	What a rule does shows in the task's history under the rule's name. Undo takes back your own edit,
	and what the rule did then stays until you change it.
</p>
<p>
	Rules act from now on: turning one on changes nothing already there. A rule's own changes start no
	other rule, so put changes that belong together in one rule. Imports, making things from a
	template and a repeating task coming back start no rules either.
</p>
<p>
	A rule that runs 200 times in an hour stops and tells whoever saved it. Recent runs are listed on
	the page for 30 days. A project template keeps its project's rules, switched off in each project
	made from it.
</p>
