<script lang="ts">
	import Tip from '../../components/help/Tip.svelte';
	import HelpLink from '../../components/help/HelpLink.svelte';
</script>

<h2 id="setting" tabindex="-1">Setting a repeat</h2>
<p>
	Open the task and choose from its Repeat menu: every day, every weekday, or weekly, monthly or
	yearly on the task's due date (today, if it has none), or Custom. Custom repeats every so many
	days, weeks, months or years, on the weekdays you pick, or monthly on a day such as the second
	Tuesday or the last day.
</p>
<p>
	You can also type a repeat in quick add, such as <code>every mon and thu</code>.
	<HelpLink topic="quick-add" section="repeats">Repeats in quick add</HelpLink>
</p>

<h2 id="modes" tabindex="-1">On a schedule or after completion</h2>
<p>
	A repeat keeps to its schedule unless you turn on Repeat after completion. On a schedule, the next
	task is due on the next date the rule falls on.
</p>
<Tip
	>If you finish late, dates already gone are skipped, so you never get a pile of overdue copies.</Tip
>
<p>
	After completion, the next task is due a set time after the day you finish this one: water the
	plants 3 days after you last did.
</p>

<h2 id="completing" tabindex="-1">Completing a repeating task</h2>
<p>
	Completing a repeating task, or moving it to a Done or Canceled status, keeps it as done and makes
	the next one just below it, with the new due date. The next one keeps the title, notes, project,
	priority, due time, labels, people, custom fields and checklist, with the checklist unchecked.
	Reminders tied to the due date come along too, moved to the new one.
</p>
<p>
	Undo takes back the completion and the next task with it. If two devices complete the same task
	while offline, only one next task is made.
</p>

<h2 id="ending" tabindex="-1">Ending a repeat</h2>
<p>
	In Custom, Until sets the last date the repeat may fall on; completing the task after that makes
	no next one. To stop a repeat at once, choose Does not repeat.
</p>
