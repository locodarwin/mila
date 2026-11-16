<script lang="ts">
	import TaskSections from '#lib/components/TaskSections.svelte';
	import ListViews from '#lib/components/views/ListViews.svelte';
	import { session } from '#lib/session.svelte.js';
	import { localToday } from '#lib/sync/data.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import type { Task } from '#lib/sync/entities.js';
	import { formatDate } from '#lib/dates.js';
	import { addDays } from '#lib/filter.js';

	const ws = $derived(session.workspace?.id ?? '');
	const today = localToday();

	const byDue = (a: Task, b: Task) =>
		(a.dueDate ?? '').localeCompare(b.dueDate ?? '') ||
		(a.dueTime ?? '99').localeCompare(b.dueTime ?? '99') ||
		a.title.localeCompare(b.title);

	// Every open task with a date, across projects.
	const scope = $derived(
		(engine.replica(ws)?.activeTasks() ?? []).filter((t) => !t.completedAt && t.dueDate).sort(byDue)
	);

	// Overdue, then each of the next seven days, then later.
	function sectionsOf(keep: (t: Task) => boolean) {
		const due = scope.filter(keep);
		const week = Array.from({ length: 7 }, (_, i) => addDays(today, i));
		const out = [
			{
				id: 'overdue',
				title: 'Overdue',
				tasks: due.filter((t) => t.dueDate! < today),
				tone: 'danger' as const
			}
		];
		week.forEach((day, i) => {
			const title =
				i === 0
					? 'Today'
					: i === 1
						? 'Tomorrow'
						: formatDate(day, { weekday: 'long', year: false });
			out.push({
				id: day,
				title,
				tasks: due.filter((t) => t.dueDate === day),
				tone: undefined as never
			});
		});
		out.push({
			id: 'later',
			title: 'Later',
			tasks: due.filter((t) => t.dueDate! > week[6]),
			tone: undefined as never
		});
		return out;
	}
	const sections = $derived(sectionsOf(() => true));
</script>

<svelte:head><title>Upcoming · Mila</title></svelte:head>

<ListViews
	{ws}
	list="upcoming"
	title="Upcoming"
	{scope}
	{sectionsOf}
	label="Upcoming tasks"
	emptyTitle="Nothing due soon"
	emptyHint="Give a task a due date and it shows up here, from every project."
>
	{#snippet own()}
		<TaskSections
			{ws}
			{sections}
			label="Upcoming tasks"
			emptyTitle="Nothing due soon"
			emptyHint="Give a task a due date and it shows up here, from every project."
		/>
	{/snippet}
</ListViews>
