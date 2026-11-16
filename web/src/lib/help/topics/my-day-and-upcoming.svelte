<script lang="ts">
	import Shot from '../../components/help/Shot.svelte';
	import Tip from '../../components/help/Tip.svelte';
	import Keys from '../../components/help/Keys.svelte';
</script>

<h2 id="adding" tabindex="-1">Adding to My Day</h2>
<p>My Day is your list for today.</p>
<Tip
	>It is yours alone: nobody else in the workspace sees what you picked, and picking a task changes
	nothing about the task itself.</Tip
>
<ul>
	<li>
		<Keys
			id="task.my-day"
			lead="On a task in a list, press "
			trail=". The same key takes it out again."
			otherwise="On a task in a list, the key for Add to My Day does it, once you give it one in Settings, Keyboard."
		/>
	</li>
	<li>
		Choose Add to My Day in a task's schedule chooser: its date button, or a swipe to the left on a
		phone.
	</li>
	<li>Open the command palette with a task focused or open, and choose Add the task to My Day.</li>
	<li>Use quick add while you are on My Day.</li>
</ul>

<h2 id="earlier" tabindex="-1">From earlier</h2>
<p>
	Tasks you picked on an earlier day and have not finished are not moved to today on their own. They
	wait under From earlier, so each morning starts with a choice. Add all to today brings them all
	back, or add them one at a time.
</p>

<h2 id="due" tabindex="-1">Due today or overdue</h2>
<p>
	Below your picks, My Day suggests the open tasks in the workspace that are due today or overdue
	and not picked yet. Add all puts them all in today's list.
</p>

<h2 id="upcoming" tabindex="-1">Upcoming</h2>
<p>
	Upcoming shows every open task with a due date, across all projects: what is overdue, then today
	and each of the next six days, then everything later. Tasks on the same day are in order of their
	due time.
</p>
<Shot
	name="upcoming"
	alt="Upcoming: tasks listed under Overdue, Today, Tomorrow, the next few days by name, and Later."
/>
<h2 id="views" tabindex="-1">Other ways to see them</h2>
<p>
	Inbox, My Day and Upcoming can each show as a list, a board, a calendar, a table or a timeline,
	from the buttons at the top, and Filter and Display work as they do in a saved view. A filter
	narrows what the list holds and never adds to it: Upcoming still shows only tasks with a due date.
</p>
<p>
	In a list, each keeps its own sections, such as Upcoming's days, until you group it another way in
	Display. A board, calendar, table or timeline of My Day shows what you picked, not the
	suggestions.
</p>
<p>
	A board here has a column for each priority, since each project has statuses of its own, and in My
	Day and Upcoming each card shows its project. Display can make the columns the due date, or a
	select or person field every project shares. Moving a card to another column gives its task that
	priority, day or choice.
</p>
<p>
	Upcoming's board starts with its own days: Overdue, Today, Tomorrow, the next five days by name,
	and Later. Moving a card to Later makes it due the day after the week, and nothing moves into
	Overdue.
</p>
<p>
	In My Day and Upcoming, cards stay in the list's own order, since they come from many projects.
	Add a task under a column adds one there, and My Day's is picked for today. Upcoming's board adds
	only to a day, since a task needs a due date to be there.
</p>
<p>
	What you choose is kept on this device, for each list in each workspace. Reset, which shows once
	you change something, puts the list back as it comes. Save as view makes a saved view of what you
	set up, only for you until you share it.
</p>
