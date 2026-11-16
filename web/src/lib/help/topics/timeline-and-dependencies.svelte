<script lang="ts">
	import Shot from '../../components/help/Shot.svelte';
	import Tip from '../../components/help/Tip.svelte';
	import HelpLink from '../../components/help/HelpLink.svelte';
	import Keys from '../../components/help/Keys.svelte';
</script>

<h2 id="dependencies" tabindex="-1">Tasks that wait for others</h2>
<p>
	Some work cannot start until other work is done: the walls are painted after the drywall, and the
	tile goes on after the countertops. Mark that by making one task wait for another.
</p>
<p>
	Open the task that waits, and under Blocked by choose Add a task it waits for. Type part of a
	title, pick the task, and it is added. Under Blocking, a task lists the tasks that wait for it.
	Remove either kind with the button at the end of its line.
</p>
<p>
	A task can wait for tasks in other projects of the workspace. Mila refuses a dependency that would
	go in a circle, such as a task waiting for one that already waits for it, and says which tasks are
	involved.
</p>
<p>Guests see a dependency only when they can see both of its tasks.</p>

<h2 id="blocked" tabindex="-1">Blocked tasks</h2>
<p>
	A task that waits for one that is not done shows a Blocked mark in lists, on boards and in tables.
	Point at the mark to see what it waits for. Once every task it waits for is done, the mark goes.
</p>
<p>
	To find these tasks, filter a view by Dependencies: Is blocked shows tasks that are waiting, and
	Is blocking shows unfinished tasks that hold others up. <HelpLink
		topic="views-and-filters"
		section="filters">More about filters</HelpLink
	>
</p>

<h2 id="conflicts" tabindex="-1">Dates that clash</h2>
<p>
	A task should start after the tasks it waits for are due. When it starts on or before that day,
	its panel says so in red, and on a timeline the arrow between the two turns red. A task with only
	a due date counts that date as its start and its end.
</p>
<Tip>Mila never moves dates by itself. Change them when you are ready.</Tip>

<h2 id="unblocked" tabindex="-1">When a task can start</h2>
<p>
	When a task is done, the people assigned to each task that waited for it are told: that it can
	start, or what it still waits for. You are not told about a task you finished yourself. Choose
	where these go under Ready to start in Settings, Notifications. <HelpLink
		topic="reminders-and-notifications"
		section="channels">More about notifications</HelpLink
	>
</p>

<h2 id="timeline" tabindex="-1">The timeline</h2>
<p>
	A timeline shows each task as a bar from its start date to its due date. A task with only a due
	date is a filled diamond on that day, and one with only a start date an open diamond. Arrows run
	from the end of a task to the start of each task that waits for it. A repeating task shows its
	next few dates as faint diamonds.
</p>
<Shot
	name="timeline"
	alt="A timeline: a bar for each task with a start and a due date, a diamond for a task with only a due date, across the weeks of this month."
/>
<p>
	Choose Timeline at the top of a project, or as a saved view's type. Zoom to show days, weeks,
	months or quarters; Today brings today, marked by a line, into view. Subtasks sit under their
	task: fold them away with the arrow beside it, and a bracket shows the dates they span. A saved
	view keeps its zoom; a project keeps it on the device.
</p>
<p>
	Drag the right edge of the Task heading to make the task names wider or narrower, or focus it and
	use the arrow keys. The width is kept on this device, for every timeline.
</p>
<p>On a phone the titles sit on the bars, and the timeline shows weeks or months.</p>

<h2 id="moving" tabindex="-1">Moving tasks on the timeline</h2>
<p>
	Drag a bar to move the task, keeping its length, or drag either end to change one date. Dragging a
	diamond moves its one date.
</p>
<p>
	On the keyboard, focus a task in the timeline and move it earlier or later<Keys
		id={['timeline.earlier', 'timeline.later']}
		lead=" with "
		trail=";"
		otherwise=";"
	/> it moves a day at Day zoom, a week at Week zoom and a month at Month and Quarter zoom. Move only
	its due date<Keys
		id={['timeline.due-earlier', 'timeline.due-later']}
		lead=" with "
		trail="."
		otherwise=" from its panel."
	/>
</p>
<p>
	When a task moves later than a task waiting for it starts, Mila offers to move every task down the
	chain by the same number of days. Undo puts everything back.
</p>

<h2 id="linking" tabindex="-1">Linking tasks on the timeline</h2>
<p>
	Point at a bar to show a dot before it and a dot after it. Drag the dot after a task onto another
	to make that one wait for it; drag the dot before a task onto another to make it wait for that
	one. On a phone, add dependencies from the task's panel.
</p>

<h2 id="unscheduled" tabindex="-1">Tasks with no dates</h2>
<p>
	Tasks with no dates wait in the Unscheduled tray under the timeline. Drag one onto the timeline to
	make it due on that day, or choose it to make it due today.
</p>
