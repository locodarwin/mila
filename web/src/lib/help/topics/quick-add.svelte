<script lang="ts">
	import Shot from '../../components/help/Shot.svelte';
	import Example from '../../components/help/Example.svelte';
	import Keys from '../../components/help/Keys.svelte';
	import Tip from '../../components/help/Tip.svelte';
	import Token from '../../components/help/Token.svelte';
	import { ALL_DAY_AT, formatTime } from '../../dates';
	import { ui } from '../../ui.svelte';

	const nine = formatTime(ALL_DAY_AT);
</script>

<Example
	typed="Call Dana tomorrow 3pm #work !high"
	tokens={[
		{ kind: 'project', text: '#work' },
		{ kind: 'date', text: 'tomorrow' },
		{ kind: 'time', text: '3pm' },
		{ kind: 'priority', text: '!high' }
	]}
	result="Becomes &quot;Call Dana&quot;, due tomorrow afternoon, in the project Work, with high priority."
>
	<button type="button" class="try" onclick={() => ui.open('quickadd')}>Try it in quick add</button>
</Example>
<p>
	Quick add makes a task from one line of text. Open it with New task in the sidebar or the round
	add button on a phone<Keys id="quick-add" lead=", or press " />. As you type, it shows a chip for
	each part it understood, so you can see what you will get before you add it.
</p>
<Shot
	name="quick-add"
	alt="The quick add box holding the line Call the broker tomorrow 3pm #orders !high, with chips under it for the date, the time, the project and the priority."
/>
<Tip>Anything it does not understand stays in the title, so nothing you type is lost.</Tip>

<h2 id="project" tabindex="-1">Projects</h2>
<p>
	<Token kind="project" text="#work" /> puts the task in a project. The start of the name is enough, and
	a project whose whole name matches wins. Write spaces as hyphens:
	<code>#home-repairs</code> finds Home repairs. Only the first project counts.
</p>

<h2 id="priority" tabindex="-1">Priority</h2>
<p>
	<Token kind="priority" text="!high" /> sets the priority. Use <code>!urgent</code>,
	<code>!high</code>, <code>!medium</code> (or <code>!med</code>) and <code>!low</code>, or the
	numbers <code>!1</code> (urgent) to <code>!4</code> (low).
</p>

<h2 id="people" tabindex="-1">People</h2>
<p>
	<Token kind="person" text="@dana" /> assigns the task to a member of the workspace, found by the start
	of their name. You can name more than one person.
</p>

<h2 id="labels" tabindex="-1">Labels</h2>
<p>
	<Token kind="label" text="+phone" /> adds a label that already exists, found by the start of its name.
	You can add several. A label that does not exist yet stays in the title.
</p>

<h2 id="dates" tabindex="-1">Dates and times</h2>
<p>These set the due date. You can put "on", "due" or "by" in front of any of them.</p>
<ul>
	<li>
		<code>today</code>, <code>tomorrow</code> (or <code>tmr</code>), and a weekday such as
		<code>fri</code> or <code>friday</code>, which means the next one, never today.
	</li>
	<li>
		<code>next week</code> is the first day of next week, as your week starts in Preferences.
		<code>next fri</code> is the Friday after the coming one.
	</li>
	<li><code>in 3 days</code> and <code>in 2 weeks</code>.</li>
	<li>
		<code>nov 14</code>, <code>14 nov</code>, <code>11/14</code> (month first) and
		<code>2026-11-14</code>. A day that has passed this year means next year.
	</li>
</ul>
<p>
	A time sets the due time: <Token kind="time" text="3pm" />, <code>3:30pm</code>,
	<code>15:00</code>, <code>noon</code> or <code>midnight</code>, with or without "at" in front. A
	time with no date means today.
</p>

<h2 id="repeats" tabindex="-1">Repeats</h2>
<p>
	<Token kind="repeat" text="every week" /> makes the task repeat on a schedule. You can write
	<code>every day</code>, <code>every weekday</code>, <code>every 2 weeks</code>,
	<code>every other month</code>, <code>every monday</code>, <code>every mon and thu</code>, or
	<code>daily</code>, <code>weekly</code>, <code>monthly</code> and <code>yearly</code>. With no
	date, the task is due on the first day the repeat falls on.
</p>

<h2 id="reminders" tabindex="-1">Reminders</h2>
<p>
	A line can carry one reminder for you. <Token kind="reminder" text="remind 1h before" /> fires before
	the due time; it needs a due date, and a task with no time is due at {nine}. Use minutes, hours or
	days: <code>remind 30 minutes before</code>, <code>remind 2 days before</code>.
</p>
<p>
	Or give a moment: <code>remind 3pm</code>, <code>remind tomorrow 9am</code>,
	<code>remind me fri at 8:30am</code>. A time alone is on the due date, or today (tomorrow once the
	time has passed) for a task with no date. A date alone is at {nine}. A reminder that would already
	be past stays in the title.
</p>

<h2 id="where" tabindex="-1">Where the task goes</h2>
<p>
	<Keys id="quickadd.add" trail=" adds" otherwise="Adding" /> the task and closes quick add. <Keys
		id="panel.close"
		trail=" closes it without adding, as does Cancel on a touch screen."
		otherwise="Cancel on a touch screen closes it without adding."
	/> With no project named, the task goes to the project on screen (a project, or a saved view of one
	project), or else to the Inbox; the line under the field says which. Added from My Day, it also goes
	into My Day. A guest has no Inbox, so a guest names a project or opens one first.
</p>

<style>
	.try {
		padding: var(--space-1) var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		font-weight: 550;
		cursor: pointer;
	}
	.try:hover {
		background: var(--surface-2);
	}
</style>
