<script lang="ts">
	import Keys from '../../components/help/Keys.svelte';
</script>

<h2 id="estimates" tabindex="-1">Estimates</h2>
<p>
	Every task has an estimate of how long its own work should take. Type it in the Time section of
	the task's panel: 1h 30m, 90m, 1.5h and 1:30 all mean an hour and a half, and a number on its own
	counts hours. Clear the box to take the estimate away. An estimate can be up to 999 hours.
</p>
<p>
	A subtask has an estimate of its own. A parent shows its own figures, and below them its subtasks'
	added in, so nothing is counted twice when a view adds up a column.
</p>

<h2 id="logging" tabindex="-1">Logging time</h2>
<p>
	Choose Log time in the task's panel, write how long, pick the day (today unless you change it, and
	never a day still to come) and add a note if you like. One entry holds up to 24 hours; log the
	rest as another.
</p>
<p>
	The panel shows the time logged, what is left of the estimate, or how far past it the task has
	gone, in red. Below are the entries, newest first. Change or delete your own with the pencil
	beside each; admins can change anyone's. Every change shows in the task's history.
</p>
<p>
	Everyone who sees a task sees the time logged on it, guests too. Guests can log time on the tasks
	they see.
</p>

<h2 id="timer" tabindex="-1">The timer</h2>
<p>
	Choose Start a timer in the task's panel, or search the command palette for "timer". The timer
	runs at the foot of the sidebar, or above the tabs on a phone, on every page and every device you
	use, and the task's row shows it ticking. Stop it there or in the panel, and the time is logged on
	the day it started.
</p>
<p>
	You have one timer at a time: starting another stops the one running and logs it. A timer stopped
	before a minute logs nothing.
</p>
<p>
	A timer left running for 10 hours asks what to do the next time you open Mila: stop it when you
	last did something in Mila, stop it at a time you choose, or keep it going.
</p>

<h2 id="project-time" tabindex="-1">Time on a project itself</h2>
<p>
	Some work belongs to a project but to none of its tasks, such as a meeting, planning or travel.
	Log it on the project's Time tab, beside Tasks and Project Wiki: Log time and Start a timer there
	go to the project itself.
</p>
<p>
	Log time on the Time page does the same for any project: choose the project, then the time, the
	day and a note. While the page shows one project, the form starts on it. A message says where the
	time went, with Undo.
</p>
<p>
	The Time tab shows this month's time in all, on the project's tasks and on the project itself, and
	every entry of both kinds, newest first. Change or delete your own time on the project with the
	pencil beside it; admins can change anyone's. Time on a task opens its task.
</p>
<p>
	The command palette has Log time to the project and Start a timer for the project while you are on
	its page, and so does the project's menu in the sidebar. Everyone who sees the project sees its
	time, and guests can log time to the projects they see.
</p>

<h2 id="views" tabindex="-1">Time in views</h2>
<p>
	Rows and board cards show the time logged against the estimate, such as 3h 20m / 6h, in red once
	past it, and board columns add them up. Tables have Estimate, Time logged and Remaining columns,
	which sort and sum. Choose which show in a view's Display panel, and sort by any of them there.
</p>
<p>
	A view's filter can ask for tasks with an estimate or none, tasks over their estimate, tasks with
	time logged in the last seven days, or time you logged. Every time condition chosen must hold.
</p>
<p>On the timeline, a bar fills as time is logged against its estimate, and turns red past it.</p>

<h2 id="report" tabindex="-1">The Time page</h2>
<p>
	Time in the sidebar<Keys id="go.time" lead=", or " trail="," /> adds up what was logged in the Period
	you choose: this week, last week, this month, last month, the last 8 or 12 weeks, this quarter, last
	quarter, or between two days. Show a row for each person, project or label, for every project or one.
	A week has a column for each day; anything longer has one for each week, starting on the day set in
	your preferences.
</p>
<p>
	With a column for each week, By week charts the hours logged each week, stacked by the rows: the
	five with the most each in a color of their own, and the rest together as Others. Each week's
	total is on top. The week under way is lighter, and its total says so far. Point at a week, or tap
	it, to see its parts; from the keyboard, move to the chart and the arrow keys step through the
	weeks.
</p>
<p>
	A task with two labels counts under both, so the label rows can add up to more than the total,
	which counts each piece of time once. Time on a project itself counts with its project and its
	person, and under Not on a task when the rows are labels.
</p>
<p>
	Estimates against what they took sets each project's estimates for the tasks finished in the
	period beside all the time logged on them, which leaves out time on a project itself. Guests see
	only their own time on this page.
</p>
<p>
	Export CSV saves This table, as the page shows it: a line for each row, a column for each day or
	week, and the totals, in hours. Or Every entry, one line each.
</p>
<p>
	Time logged goes into the workspace's JSON and CSV exports and comes back with an import; time on
	a project itself has no task there. The daily email digest says what you logged yesterday, on
	tasks and on projects, and which of your tasks are past their estimate. The weekly report counts
	both too.
</p>
