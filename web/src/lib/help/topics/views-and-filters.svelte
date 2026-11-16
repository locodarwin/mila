<script lang="ts">
	import Shot from '../../components/help/Shot.svelte';
	import Tip from '../../components/help/Tip.svelte';
	import HelpLink from '../../components/help/HelpLink.svelte';
	import Keys from '../../components/help/Keys.svelte';
</script>

<h2 id="layouts" tabindex="-1">List, board, calendar, table and timeline</h2>
<p>
	A project shows its tasks as a list, a board, a calendar, a table or a timeline. Switch with the
	List, Board, Calendar, Table and Timeline buttons at the top of the project. <HelpLink
		topic="timeline-and-dependencies"
		section="timeline">More about the timeline</HelpLink
	>
</p>
<Shot
	name="list"
	alt="A project shown as a list: tasks grouped under its statuses, Later and This month, each with its priority, labels, time logged, assignee and due date."
/>
<p>
	The board has a column for each of the project's statuses (a saved board can have other columns,
	as Sort, group and show says). Drag a card to another column<Keys
		id={['board.move-left', 'board.move-right']}
		lead=", or focus it and move it with "
	/>.
</p>
<p>
	Drag the edge between two columns to make every column wider or narrower, or focus the first edge
	and use the arrow keys. Double-click an edge to put them back. The width is kept on this device,
	for every board.
</p>
<Shot
	name="board"
	alt="A project shown as a board: a column for each status, Later, This month, Doing and Waiting on supplier, with a card for each task."
/>
<p>
	The calendar puts each task on its due date. Drag a task to another day<Keys
		id={['calendar.earlier-day', 'calendar.later-day']}
		lead=", or move it with "
	/>. A day with more tasks than its square holds scrolls on its own. The coming dates of a task
	that repeats on a schedule show faintly; one that repeats after completion has no dates to show
	until you finish it. On a phone the calendar is a list of the days that have something on them.
</p>
<Shot
	name="calendar"
	alt="A project shown as a calendar: this month, with tasks on their due dates."
/>

<h2 id="tables" tabindex="-1">Tables</h2>
<p>
	A table shows a task on each row and a field in each column, with each task's subtasks indented
	under it. The arrow beside a task folds its subtasks away; on its title, Left folds them and Right
	shows them again.
</p>
<Shot
	name="table"
	alt="A project shown as a table: a row for each task, with columns for status, priority, assignees, due date and estimate, grouped under its statuses."
/>
<p>
	In a saved view, a subtask the filter finds shows under its task, even when the task itself does
	not match. A timeline works the same way; the other layouts filter only top-level tasks.
</p>
<p>
	Click a heading to sort by it: once for ascending, again for descending, and a third time to stop
	sorting by it. Shift-click a heading to add it as a second or third key; the small numbers say the
	order. The sort is the view's own, the same one Display shows.
</p>
<p>
	Drag a heading to move its column, or its right edge, which shows as you point at the heading, to
	make it wider or narrower<Keys
		id={['table.column-left', 'table.column-right']}
		lead=". On a heading, "
		trail=" moves its column too"
	/><Keys
		id={['table.narrower', 'table.wider']}
		lead=", and "
		trail=" makes it narrower or wider"
	/>. The task column stays first. Display, Reset order and widths puts the columns back.
</p>
<p>
	Click a cell to change it: status, priority, assignees, dates, project, labels and custom fields
	each open a picker, and Escape closes it. The title opens the task, and the pencil beside it
	renames it in place<Keys id="table.rename" lead=", as does " />. The keys for tasks in a list work
	on any cell of a row.
</p>
<p>
	Each group ends in a summary row. Click a summary to choose what it works out for its column: a
	count, a sum, an average, the smallest or largest number, a range of dates, the earliest or
	latest, how many are overdue, a bar of how the group divides up, or how many tasks have a value.
	Summaries count subtasks too. Estimates and time logged are each task's own, so a parent and its
	subtasks are never counted twice. A task that shows only because of a subtask under it is not
	counted.
</p>
<p>
	When the table is not sorted, drag a row by the handle at its start to move it<Keys
		id={['task.move-up', 'task.move-down']}
		lead=", or use "
	/>. Dropping it in another group gives it what that group stands for: its status, priority,
	project or choice. A subtask moves among its own siblings, and dragging never makes a task a
	subtask.
</p>
<p>
	A project's table keeps its layout on your device; Save as a view keeps it in a view for everyone.
	On a phone a table shows as a list, with the same sort, grouping and fields.
</p>

<h2 id="statuses" tabindex="-1">Statuses</h2>
<p>
	Each project has its own statuses, changed with Edit statuses at the top of the project. Every
	status belongs to one of five kinds: Backlog, To do, In progress, Done or Canceled.
</p>
<Tip>Moving a task to a Done or Canceled status completes it, and moving it back reopens it.</Tip>
<p>Checking a task off moves it to the project's first Done status.</p>

<h2 id="saved-views" tabindex="-1">Saved views</h2>
<p>
	A saved view keeps a layout and a filter under a name in the sidebar. The bookmark button on a
	project saves the project as a view. The plus beside Views makes a new view across every project,
	which starts out showing the tasks assigned to you.
</p>
<p>
	The arrow beside Views or Projects in the sidebar folds that list away. Each device remembers
	which are folded.
</p>
<p>
	A new view is yours alone. Share with the workspace, at the top of the view, makes it a shared
	view that everyone who can see its tasks finds in their sidebar. A view across every project can
	take any layout. Its board has a column for each priority, or for a select or person field every
	project shares, since statuses are a project's own.
</p>
<p>
	Changes to a shared view, its filter and how it is shown, are saved for everyone as you make them.
	A guest can change a shared view too, but only on their own screen: a note says the changes are
	not saved, and Reset puts the view back.
</p>

<h2 id="display" tabindex="-1">Sort, group and show</h2>
<p>
	Display, at the top of a saved list, board, table or timeline, chooses how its tasks are laid out.
	A calendar has no Display: it always puts tasks on their due dates, in order of time.
</p>
<p>
	Sort orders tasks by up to three things in turn, such as due date and then priority: title,
	priority, due date, start date, when a task was made, last changed or completed, status, project,
	estimate, time logged, time remaining, or one of your custom fields, except multi-select ones,
	which hold no one value to sort by. Tasks with nothing to sort by, such as no due date, always
	come last. Without a sort, tasks keep the order you give them.
</p>
<p>
	A sorted view has no order of its own to change, so moving a task up or down does nothing there<Keys
		id={['task.move-up', 'task.move-down']}
		lead=" ("
		trail=" only say so)"
	/>. Remove the sort to move tasks by hand again.
</p>
<p>
	Each project keeps its own order, so in a view across projects you can move tasks up or down only
	when it is grouped by project.
</p>
<p>
	Group by puts a list or a table under headings: project, status, priority, assignee, label, due
	date (Overdue, Today, Tomorrow, This week, Later and No date), or a select or person field. A task
	with two labels or two assignees shows under each of them.
</p>
<p>
	Columns sets a board's columns: the project's statuses, the priorities, the due date (Earlier,
	each day of the coming week, Later and No date), or the choices of a select or person field.
	Moving a card to another column gives it that status, priority, day or choice, and a task added in
	a column starts with it. Nothing moves into Earlier. On a sorted board, cards move between columns
	but not up or down within one.
</p>
<p>
	Show chooses what rows or cards carry beside the title, or a table's columns: status, project,
	priority, labels, subtasks, checklist, comments, repeat, reminders, assignees, start date, due
	date, estimate, time logged, time remaining and your custom fields. A table shows every custom
	field until you hide it. A row with its due date hidden still has its schedule button.
</p>

<h2 id="filters" tabindex="-1">Filters</h2>
<p>
	Filter, at the top of a saved view, narrows it by words in the title or notes, status, priority,
	label, assignee, due date, dependencies and time, and by project in a view across every project.
	Dependencies finds tasks that are blocked, waiting for one not done, and tasks that are blocking
	others. Time finds tasks with an estimate or none, tasks over their estimate, and tasks with time
	logged lately or by you. In my My Day keeps the tasks you picked for today or an earlier day, and
	the Due choice Has a due date keeps any task with one.
</p>
<p>
	Status offers the five kinds of status first (Backlog, To do, In progress, Done and Canceled),
	each covering every project's statuses of that kind. After them come the statuses of the projects
	in the view by name, such as Waiting on review, leaving out those named for their kind. A status
	that several projects have is one chip, which picks it in all of them. Pick a kind and a status
	together to see the tasks in either.
</p>
<p>
	A status chip shows a dash instead of a check when it is picked in only some of the projects that
	have it, which happens when another project adds a status of that name later. Click it to pick it
	in all of them.
</p>
<p>
	Where you pick from chips, a task matches if it has any of the chips you picked, except for Time,
	where it must have every one; the words match as one phrase. A task must match every part you set.
</p>
<p>
	Add a field condition to filter on a custom field. Text and links can contain words or not;
	numbers can be equal to, less than or more than a number; dates can be on, before, after or
	between days, given as a date or as today, tomorrow or a week away; choices and people can be any
	or none of those you pick, and a multi-select can have all of them; a checkbox can be checked or
	not. Any other field can also be empty or not empty.
</p>
<p>
	A condition on a field that was deleted, or that you cannot see, treats every task as having no
	value there, and shows as A deleted field with a Remove button. A choice that was deleted, or a
	person who left, stays in the condition as its own chip until you take it out.
</p>
<p>
	Finished tasks stay out unless you turn on Show finished tasks, pick the Done or Canceled kind, or
	pick a status of one of those kinds. A filter is saved with the view as you change it. A calendar
	app subscribed to the view follows its filter, field conditions included, but not its sort or
	grouping.
</p>
