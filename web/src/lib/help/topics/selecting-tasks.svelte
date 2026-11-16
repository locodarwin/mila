<script lang="ts">
	import Keys from '../../components/help/Keys.svelte';
	import Tip from '../../components/help/Tip.svelte';
</script>

<h2 id="select" tabindex="-1">Selecting tasks</h2>
<p>
	Select tasks in a list, a board or a table, then change them all at once. It works in a project, a
	saved view, My Day, Upcoming, Today and the Inbox.
</p>
<p>
	Hold Ctrl as you click a task to add it to the selection, or to take it out; on a Mac, hold
	Command, since Ctrl and a click is a right-click there. Hold Shift as you click to select every
	task between the one you picked last and this one, as the list shows them.
</p>
<p>
	Once anything is selected, each task has a circle at its start. Click a circle to add the task or
	take it out, with no key held. A task's right-click menu has Select too, first.
</p>
<Tip
	>A plain click still does what it always does: the title opens the task, and the checkbox
	completes it.</Tip
>

<h2 id="keys" tabindex="-1">From the keyboard</h2>
<p>
	<Keys
		id="select.toggle"
		lead="With a task focused, "
		trail=" adds it to the selection or takes it out."
		otherwise="Set a key for selecting in Settings, Keyboard to add a focused task to the selection."
	/>
	<Keys
		id="select.down"
		lead="Hold Shift as you move with "
		trail=" and the arrow keys to stretch the selection; moving back takes off what you passed."
		otherwise="Shift with the arrow keys stretches the selection."
	/>
	<Keys
		id="select.all"
		lead=""
		trail=" selects every task the list holds, including ones further down that haven't been drawn yet."
		otherwise="Ctrl A selects every task the list holds."
	/>
	<Keys
		id="select.clear"
		lead=""
		trail=" clears the selection."
		otherwise="Escape clears the selection."
	/>
</p>
<p>
	While anything is selected, the task keys act on all of it: complete, My Day, the priorities and
	delete.
</p>

<h2 id="bar" tabindex="-1">The bar</h2>
<p>
	A bar comes up at the bottom of the page with the count and what you can do: Complete, Schedule,
	Priority, Status, Labels, Assign, Move to, and More, which holds My Day, Copy links and Delete.
	Right-click a selected task for the same, as a menu. Each change is one step, and the message
	after it has Undo for all of it.
</p>
<p>
	When the tasks differ, the menus say so. A check beside a priority or status means they all have
	it. In Labels and Assign, a dash means some have it: choose it to put it on all of them, and
	choose a check to take it off all of them. Only people who can reach every selected task are
	offered.
</p>
<p>
	Schedule's dates set the same date on all of them. A day later and A week later move each task's
	own dates instead, keeping the gaps between them. Status, for tasks in different projects, offers
	To do, In progress and Done, each project's first status of that kind. Copy links copies a list of
	links you can paste into a page.
</p>

<h2 id="stays" tabindex="-1">What stays selected</h2>
<p>
	What you select stays selected as you scroll, open a task or make a change, so you can make
	another. Going to another page clears it. A task that leaves the list, moved somewhere else,
	filtered out or deleted, leaves the selection too, and comes back with Undo.
</p>
<p>On a board, drag one of the selected cards to another column and the others go with it.</p>

<h2 id="others" tabindex="-1">Rules and other people</h2>
<p>
	Your automations run for each task you change, as if you'd changed them one by one, but a change
	to many counts once toward a rule's limit of 200 runs an hour. Someone you assign several tasks at
	once gets one notification on their phone or computer, and each task in their list.
</p>

<h2 id="phones" tabindex="-1">On a phone</h2>
<p>
	Hold your finger on a task for its menu, and choose Select. Until you tap Done, a tap selects a
	task instead of opening it, circles stand in for the checkboxes, and swipes are off. The bar takes
	the place of the tabs, with Complete, Schedule, Move to and More, and All at the top selects the
	whole list.
</p>
