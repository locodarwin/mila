<script lang="ts">
	import Keys from '../../components/help/Keys.svelte';
	import Tip from '../../components/help/Tip.svelte';
</script>

<h2 id="menus" tabindex="-1">What a right-click does</h2>
<p>
	Right-click a task, a view or a project for what you can do to it, without opening it first. A
	task's menu works wherever the task shows: lists, boards, tables, calendars and timelines, My Day,
	Upcoming, subtasks and a view in a page. A view's and a project's work in the sidebar, on the
	Projects page, and on their own page's title.
</p>
<p>
	The same goes for a wiki page in the tree, a board column's heading when the board is by status, a
	comment, a file on a task, and a label on a task's row.
</p>
<Tip
	>Everything in a menu is somewhere else too, so a menu is a shortcut and never the only way.</Tip
>

<h2 id="tasks" tabindex="-1">A task's menu</h2>
<p>
	Select it, to change it with others, then open, complete, schedule, put in My Day, set the
	priority, status, labels and people, move to another project, start a timer, duplicate, copy a
	link, save as a template, or delete. The keys beside an item are the ones that do the same from
	the list, as Settings, Keyboard has them.
</p>
<p>
	A task that is selected with others has the selection's menu instead, which acts on all of them.
</p>
<p>
	Duplicate makes a copy just under the task, with its notes, checklist, labels, people, fields and
	open subtasks, on the same dates. Nothing that happened to the task comes along: not its comments,
	time or files. Delete asks nothing, and the message after it has Undo.
</p>

<h2 id="keys" tabindex="-1">From the keyboard</h2>
<p>
	<Keys
		id="menu.context"
		lead="With a task, view or project focused, "
		trail=" opens its menu beside it."
		otherwise="Focus a task, view or project and press the Menu key to open its menu."
	/>
	The arrow keys move through it, Right opens a list such as Priority, Left goes back, Escape closes it,
	and focus goes back where it was.
</p>

<h2 id="browser" tabindex="-1">Your browser's own menu</h2>
<p>
	In text, such as notes, a page, a field or words you have selected, a right-click is always the
	browser's, for spelling, cut, copy and paste.
</p>
<p>
	Anywhere else without a menu of Mila's, a right-click in a browser is the browser's own menu. In
	the desktop app it does nothing there, since that menu's Back and Reload would act on the app
	itself.
</p>

<h2 id="phones" tabindex="-1">On a phone</h2>
<p>
	Hold your finger still on a task, view or project for half a second, and the same menu comes up
	from the bottom of the screen. Moving your finger instead scrolls or swipes, as before.
</p>
