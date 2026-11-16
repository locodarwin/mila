<script lang="ts">
	// The style guide's sections for saved views: the filter, the Display
	// panel, field conditions, field chips, a board with priority columns
	// and a table, on the sample workspace (sample.ts).
	import Board from '#lib/components/views/Board.svelte';
	import DisplayPanel from '#lib/components/views/DisplayPanel.svelte';
	import FieldChip from '#lib/components/views/FieldChip.svelte';
	import FieldConditionRow from '#lib/components/views/FieldCondition.svelte';
	import FilterPanel from '#lib/components/views/FilterPanel.svelte';
	import TableView from '#lib/components/views/TableView.svelte';
	import TimelineView from '#lib/components/views/TimelineView.svelte';
	import ViewNav from '#lib/components/ViewNav.svelte';
	import TaskItem from '#lib/components/TaskItem.svelte';
	import { engine } from '#lib/sync/engine.svelte.js';
	import type { FieldCondition, ViewDisplay, ViewFilter } from '#lib/sync/entities.js';
	import { boardColumns } from '#lib/boardColumns.js';
	import { applyPaths } from '#lib/viewDisplay.js';
	import { SAMPLE_ME, SAMPLE_PROJECT, SAMPLE_WS } from './sample';

	const sample = $derived(engine.replica(SAMPLE_WS));
	const sampleTasks2 = $derived(sample?.allTasks() ?? []);
	const sampleFields = $derived(sample?.fields() ?? []);
	let demoFilter = $state<ViewFilter>({
		priorities: ['high'],
		fields: [{ field: 'f-budget', op: 'gt', value: 10 }]
	});
	let demoDisplay = $state<ViewDisplay>({
		sort: [{ by: 'due', dir: 'asc' }],
		group: { by: 'priority' },
		fields: { list: { show: ['f-size'], hide: ['comments'] } }
	});
	let demoConditions = $state<FieldCondition[]>([
		{ field: 'f-budget', op: 'lt', value: 100 },
		{ field: 'f-delivery', op: 'between', value: [0, 7] },
		{ field: 'f-size', op: 'any', value: ['o-large'] },
		{ field: 'f-gone', op: 'not_empty' }
	]);
	const priorityColumns = $derived(
		boardColumns(
			{ by: 'priority' },
			{
				me: SAMPLE_ME,
				statuses: sample?.statuses(SAMPLE_PROJECT) ?? [],
				fields: sampleFields,
				values: sample?.fieldValuesByField() ?? new Map(),
				members: [],
				nameOf: () => ''
			}
		)
	);
	let tableDisplay = $state<ViewDisplay>({
		fields: { table: { show: ['labels'], hide: ['f-link', 'f-delivery'] } },
		table: { summaries: [{ col: 'priority', fn: 'bar' }] }
	});
	let timelineDisplay = $state<ViewDisplay>({ group: { by: 'none' }, timeline: { zoom: 'day' } });
	const chipShown = new Set(['priority', 'labels', 'due', 'f-budget', 'f-size', 'f-ordered']);
</script>

<section>
	<h2>Filters and views</h2>
	<div class="filters-demo">
		<div class="views-demo"><ViewNav ws={SAMPLE_WS} me={SAMPLE_ME} /></div>
		<FilterPanel
			ws={SAMPLE_WS}
			filter={demoFilter}
			acrossProjects
			onchange={(edit) => {
				demoFilter = { ...demoFilter, ...(typeof edit === 'function' ? edit(demoFilter) : edit) };
			}}
		/>
	</div>
</section>

<section>
	<h2>Display</h2>
	<p class="note">
		A saved view's sort, grouping (lists) or columns (boards), and the fields its rows show.
	</p>
	<DisplayPanel
		type="list"
		display={demoDisplay}
		look={{ projectId: SAMPLE_PROJECT, fields: sampleFields }}
		onchange={(edit) => {
			demoDisplay = applyPaths(demoDisplay, typeof edit === 'function' ? edit(demoDisplay) : edit);
		}}
	/>
</section>

<section>
	<h2>Field conditions</h2>
	<p class="note">
		Conditions on custom fields in a view's filter; one on a deleted field can only be removed.
	</p>
	<div class="conditions-demo">
		{#each demoConditions as c, i (i)}
			<FieldConditionRow
				ws={SAMPLE_WS}
				condition={c}
				fields={sampleFields}
				n={i + 1}
				onchange={(edit) => {
					demoConditions[i] = edit(demoConditions[i]);
				}}
				onremove={() => (demoConditions = demoConditions.filter((_, j) => j !== i))}
			/>
		{/each}
	</div>
</section>

<section>
	<h2>Field chips</h2>
	<p class="note">Custom fields on rows and cards, when a view shows them.</p>
	<div class="chips-demo">
		{#each sampleFields as f (f.id)}
			{#each sampleTasks2 as t (t.id)}
				{#if sample?.fieldValues(t.id)[f.id] !== undefined}
					<FieldChip ws={SAMPLE_WS} field={f} value={sample?.fieldValues(t.id)[f.id]} />
				{/if}
			{/each}
		{/each}
	</div>
	<div class="row-demo">
		<TaskItem
			ws={SAMPLE_WS}
			task={sampleTasks2.find((t) => t.id === 't-seeds')!}
			shown={chipShown}
		/>
	</div>
</section>

<section>
	<h2>Board by priority</h2>
	<p class="note">A board view whose columns are priorities; moving a card sets it.</p>
	<div class="board-demo">
		<Board
			ws={SAMPLE_WS}
			projectId={SAMPLE_PROJECT}
			tasks={sample?.tasks(SAMPLE_PROJECT) ?? []}
			columns={priorityColumns}
			shown={chipShown}
		/>
	</div>
</section>

<section>
	<h2>Table</h2>
	<p class="note">
		A table view: subtasks under their task, sortable headings, cells that edit in place, and a
		summary row under each group.
	</p>
	<div class="table-demo">
		<TableView
			ws={SAMPLE_WS}
			projectId={SAMPLE_PROJECT}
			filter={{ showDone: true }}
			display={tableDisplay}
			onchange={(edit) => {
				tableDisplay = applyPaths(
					tableDisplay,
					typeof edit === 'function' ? edit(tableDisplay) : edit
				);
			}}
			label="Sample tasks"
			emptyTitle="No tasks"
			emptyHint="The sample workspace has none."
		/>
	</div>
</section>

<section>
	<h2>Timeline</h2>
	<p class="note">
		A timeline view: a bar from each task's start to its due date, a diamond for one date, arrows
		from what a task waits for (red when the dates clash), faint diamonds for a repeat, and a tray
		of tasks with no dates. Drag a bar, its ends, or the dots beside it.
	</p>
	<div class="table-demo">
		<TimelineView
			ws={SAMPLE_WS}
			projectId={SAMPLE_PROJECT}
			filter={{ showDone: true }}
			display={timelineDisplay}
			onchange={(edit) => {
				timelineDisplay = applyPaths(
					timelineDisplay,
					typeof edit === 'function' ? edit(timelineDisplay) : edit
				);
			}}
			label="Sample timeline"
			emptyTitle="No tasks"
			emptyHint="The sample workspace has none."
		/>
	</div>
</section>

<style>
	.table-demo {
		height: 460px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	.filters-demo {
		display: grid;
		grid-template-columns: minmax(180px, 240px) minmax(0, 1fr);
		gap: var(--space-4);
		align-items: start;
	}
	.views-demo {
		padding: var(--space-2);
		border-radius: var(--radius-md);
		background: var(--surface-1);
	}
	@media (max-width: 720px) {
		.filters-demo {
			grid-template-columns: 1fr;
		}
	}
	.conditions-demo {
		display: grid;
		gap: var(--space-2);
	}
	.chips-demo {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3);
		align-items: center;
		margin-bottom: var(--space-3);
	}
	.row-demo {
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	.board-demo {
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
