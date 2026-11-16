<script lang="ts">
	// Upcoming as a list of the app's own takes a view, a filter and a display
	// (components/views/ListViews.svelte), on the sample workspace (sample.ts).
	import TaskSections from '#lib/components/TaskSections.svelte';
	import ListViews from '#lib/components/views/ListViews.svelte';
	import { engine } from '#lib/sync/engine.svelte.js';
	import type { Task } from '#lib/sync/entities.js';
	import { SAMPLE_WS } from './sample';

	const scope = $derived(
		(engine.replica(SAMPLE_WS)?.activeTasks() ?? []).filter((t) => !t.completedAt && t.dueDate)
	);
	const sectionsOf = (keep: (t: Task) => boolean) => [
		{ id: 'dated', title: 'Dated', tasks: scope.filter(keep) }
	];
</script>

<section>
	<h2>Lists of the app's own</h2>
	<p class="note">
		Inbox, My Day and Upcoming each take a view, a filter and a display, kept on the device; Reset
		shows once one is set. Here as Upcoming, on the sample workspace.
	</p>
	<div class="demo">
		<ListViews
			ws={SAMPLE_WS}
			list="upcoming"
			title="Upcoming"
			{scope}
			{sectionsOf}
			label="Sample upcoming tasks"
			emptyTitle="Nothing due soon"
			emptyHint="Give a task a due date and it shows up here."
		>
			{#snippet own()}
				<TaskSections
					ws={SAMPLE_WS}
					sections={sectionsOf(() => true)}
					label="Sample upcoming tasks"
					emptyTitle="Nothing due soon"
					emptyHint="Give a task a due date and it shows up here."
				/>
			{/snippet}
		</ListViews>
	</div>
</section>

<style>
	.demo {
		height: 360px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	/* As the style guide's own sections. */
	section {
		display: grid;
		gap: var(--space-4);
		padding: var(--space-6) 0;
		border-bottom: var(--border-width) solid var(--border);
	}
	h2 {
		font-size: var(--text-xl);
	}
	.note {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
