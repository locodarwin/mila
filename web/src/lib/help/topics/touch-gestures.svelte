<script lang="ts">
	import HelpLink from '../../components/help/HelpLink.svelte';
	import Tip from '../../components/help/Tip.svelte';
</script>

<h2 id="complete" tabindex="-1">Swipe right to complete</h2>
<p>
	Swipe a task to the right to complete it. The row holds for a second before it goes, and a message
	offers Undo, since a swipe is easy to make by accident. On a task that is already done, the same
	swipe reopens it.
</p>

<h2 id="schedule" tabindex="-1">Swipe left to schedule</h2>
<p>
	Swipe a task to the left to schedule it: today, tomorrow, next week, a date you pick, no date, or
	into My Day.
</p>
<Tip>The date button on every row opens the same choices, so nothing needs a swipe.</Tip>
<p>
	Swipes start from a finger or a pen, away from the sides of the screen, which belong to your
	phone's back gesture. Board cards do not swipe, because the board scrolls sideways.
</p>

<p>
	Hold a finger still on a task, view or project instead, and its menu comes up from the bottom of
	the screen: see <HelpLink topic="right-click-menus" section="phones">Right-click menus</HelpLink>.
</p>

<h2 id="sheet" tabindex="-1">Closing a task</h2>
<p>
	On a phone, a task opens as a sheet over the page. Pull it down by its top bar to close it, or tap
	Close.
</p>

<h2 id="search" tabindex="-1">Search and quick add</h2>
<p>
	The Search tab finds tasks, projects, saved views and help, and runs any command, such as
	switching to dark mode. The round add button opens quick add. Help is the question mark in the top
	bar, beside Settings.
</p>
