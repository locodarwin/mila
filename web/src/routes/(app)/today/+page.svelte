<script lang="ts">
	import Button from '#lib/components/Button.svelte';
	import TaskSections from '#lib/components/TaskSections.svelte';
	import ListViews from '#lib/components/views/ListViews.svelte';
	import type { Task } from '#lib/sync/entities.js';
	import { session } from '#lib/session.svelte.js';
	import { localToday, setInMyDay } from '#lib/sync/data.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { formatDate } from '#lib/dates.js';
	import { keymap } from '#lib/keys/keymap.svelte.js';

	const ws = $derived(session.workspace?.id ?? '');
	const me = $derived(session.user?.id ?? '');
	const today = localToday();
	const replica = $derived(engine.replica(ws));
	const entries = $derived(replica?.myDay(me, today) ?? []);
	const picked = $derived(new Set(entries.map((e) => e.task.id)));

	const todays = $derived(entries.filter((e) => e.day === today).map((e) => e.task));
	// Picked on an earlier day and not done: offered back, not carried over.
	const earlier = $derived(
		entries.filter((e) => e.day < today && !e.task.completedAt).map((e) => e.task)
	);
	const suggested = $derived(
		(replica?.activeTasks() ?? []).filter(
			(t) => !t.completedAt && t.dueDate !== null && t.dueDate <= today && !picked.has(t.id)
		)
	);

	// What a calendar, table or timeline shows: what is picked, not the suggestions.
	const scope = $derived([...todays, ...earlier]);

	function addAll(tasks: { id: string }[]) {
		for (const t of tasks) void setInMyDay(ws, me, t.id, true);
	}
	// The keys as the person has them, while they work.
	const emptyHint = $derived.by(() => {
		const myDay = keymap.say('task.my-day');
		const add = keymap.say('quick-add');
		if (!myDay) return "Use a task's schedule button to bring it here.";
		return add
			? `Press ${myDay} on any task to bring it here, or ${add} to add a new one.`
			: `Press ${myDay} on any task to bring it here.`;
	});
	const dateLabel = $derived(
		formatDate(new Date(), { weekday: 'long', month: 'long', year: false })
	);
</script>

<svelte:head><title>My Day · Mila</title></svelte:head>

{#snippet addEarlier()}<Button size="sm" variant="ghost" onclick={() => addAll(earlier)}
		>Add all to today</Button
	>{/snippet}
{#snippet addSuggested()}<Button size="sm" variant="ghost" onclick={() => addAll(suggested)}
		>Add all</Button
	>{/snippet}
{#snippet date()}<span class="date">{dateLabel}</span>{/snippet}

<ListViews
	{ws}
	list="myday"
	title="My Day"
	extra={date}
	{scope}
	sectionsOf={(keep: (t: Task) => boolean) => [
		{ id: 'today', title: 'Today', tasks: todays.filter(keep) },
		{ id: 'earlier', title: 'From earlier', tasks: earlier.filter(keep), action: addEarlier },
		{
			id: 'due',
			title: 'Due today or overdue',
			tasks: suggested.filter(keep),
			action: addSuggested
		}
	]}
	label="My Day"
	emptyTitle="Nothing planned for today"
	{emptyHint}
>
	{#snippet own()}
		<TaskSections
			{ws}
			label="My Day"
			emptyTitle="Nothing planned for today"
			{emptyHint}
			sections={[
				{ id: 'today', title: 'Today', tasks: todays },
				{ id: 'earlier', title: 'From earlier', tasks: earlier, action: addEarlier },
				{ id: 'due', title: 'Due today or overdue', tasks: suggested, action: addSuggested }
			]}
		/>
	{/snippet}
</ListViews>

<style>
	.date {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
