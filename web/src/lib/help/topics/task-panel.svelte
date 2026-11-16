<script lang="ts">
	import Shot from '../../components/help/Shot.svelte';
</script>

<h2 id="layouts" tabindex="-1">Labeled boxes or summary lines</h2>
<p>
	The task panel can show its sections two ways. Choose between them in Settings, Preferences, under
	Task panel. Your choice follows you to every device.
</p>
<p>
	Labeled boxes show everything at once, grouped under clear names, so you can see every field a
	task has.
</p>
<p>
	Summary lines put Status, Priority, Assignees and Labels on top, then give each other section one
	line that says what is inside. Select a line to open it in place. Each device remembers which
	lines you left open.
</p>
<div class="pictures">
	<Shot
		name="task-boxes"
		alt="A task panel as labeled boxes: its Details and Schedule in boxes under those names, every field showing."
	/>
	<Shot
		name="task-summary"
		alt="The same task as summary lines: status, priority, assignees and labels on top, then Schedule, Notes and more as lines that open in place."
	/>
</div>

<h2 id="sections" tabindex="-1">What each section holds</h2>
<ul>
	<li>Details: the task's status, priority, assignees and labels.</li>
	<li>Schedule: its start and due dates, how it repeats, and your reminders.</li>
	<li>Fields: the custom fields of its project and workspace, when there are any.</li>
	<li>Notes, Subtasks and Checklist: what the work is.</li>
	<li>Dependencies: the tasks it waits for, and the tasks waiting for it.</li>
	<li>Time: its estimate, the time logged on it, and the timer.</li>
	<li>Files, Comments and History: what has been added to it, said about it and changed on it.</li>
</ul>

<h2 id="comments" tabindex="-1">Comments</h2>
<p>
	Type @ in a comment to choose from the people who can reach this task: the workspace's members,
	and any guests the project is shared with. The person you pick is told about it.
</p>
<p>
	Type /task to refer to another task, or /wiki to refer to a wiki page. Choose a page and you can
	then choose one of its sections, or the whole page. The reference is a link, and it follows the
	title if the task or page is renamed. Someone who cannot open what you referred to sees that it is
	a task or a page, and not its name.
</p>
<p>A web address you type or paste becomes a link when the comment shows.</p>

<h2 id="help" tabindex="-1">The question marks</h2>
<p>
	A question mark after a name opens the help for that part of the panel. Rest the pointer on it to
	see what it explains.
</p>
