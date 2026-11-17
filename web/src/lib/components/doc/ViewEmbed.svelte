<script lang="ts">
	// A saved view in a wiki page or a task's notes: its
	// tasks as the view lays them out (a list, a board or a table), or as the page
	// asked for, worked out on this device from its copy, as the view's own page
	// does, so it works offline and stays current. Anyone sees only the tasks they
	// can see already. Drawn by the editor's node for it (nodeviews.svelte.ts).
	import { menu, taskActions } from '../../contextMenu.svelte';
	import { resolve } from '$app/paths';
	import { formatDate } from '../../dates';
	import { session } from '../../session.svelte';
	import { localToday, setTaskCompleted, writeColumn } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { FieldValue, Task } from '../../sync/entities';
	import {
		readTable,
		summarize,
		summaryOf,
		tableColumns,
		type SummaryContext,
		type TableColumn
	} from '../../table';
	import { openTask } from '../../taskLink';
	import { boardColumns } from '../../boardColumns';
	import { resolveDisplay, type DisplayLookups } from '../../viewDisplay';
	import { statusesById, viewList } from '../../viewList';
	import Checkbox from '../Checkbox.svelte';
	import PriorityIcon from '../PriorityIcon.svelte';
	import TableCell from '../views/TableCell.svelte';

	interface Props {
		ws: string;
		/** The view's ID. */
		id: string;
		/** The view's name when it was put here, for when it isn't. */
		name: string;
		/** The layout the page asked for, or null for the view's own. */
		layout?: string | null;
	}

	let { ws, id, name, layout = null }: Props = $props();

	/** How many rows show before Show all, and how many cards a board column. */
	const FIRST = 10;
	const CARDS = 5;
	let all = $state(false);
	let opened = $state<Record<string, boolean>>({});
	let width = $state(0);
	const replica = $derived(engine.replica(ws));
	const me = $derived(session.user?.id ?? '');
	const view = $derived(replica?.view(id));
	const shared = $derived(!!view && view.ownerId === null);
	const tasks = $derived(view && shared && replica ? viewList(ws, replica, view, me) : []);
	const shown = $derived(all ? tasks : tasks.slice(0, FIRST));
	const title = $derived(view?.name || name || 'A view');

	// What it's drawn as: the page's choice, or the view's own. A calendar or
	// timeline is a list for now; and where a table has no room, as on a phone,
	// it's a list too.
	const kind = $derived.by(() => {
		if (!view) return 'list';
		const want = layout ?? (view.type === 'board' || view.type === 'table' ? view.type : 'list');
		if (want === 'table' && width > 0 && width < 480) return 'list';
		return want;
	});
	const look = $derived<DisplayLookups>({
		projectId: view?.projectId ?? null,
		fields: replica ? (view?.projectId ? replica.fields(view.projectId) : replica.fields()) : []
	});
	const values = $derived(
		replica?.fieldValuesByField() ?? new Map<string, Map<string, FieldValue>>()
	);
	const statusById = $derived(replica ? statusesById(replica) : new Map());

	// A board: the view's columns, five cards each, a card dragged to another
	// moving its task as the board does.
	const columns = $derived.by(() => {
		if (kind !== 'board' || !view || !replica) return [];
		const resolved = resolveDisplay(view.display, 'board', look);
		return boardColumns(resolved.columns, {
			me,
			statuses: view.projectId ? replica.statuses(view.projectId) : [],
			fields: look.fields,
			values,
			members: session.members(ws).map((m) => ({ userId: m.userId, name: m.name })),
			nameOf: (uid) => session.nameOf(ws, uid),
			today: localToday(),
			dayName: (day) => formatDate(day, { weekday: 'long', year: false }),
			dated: view.filter.due === 'dated'
		}).map((c) => ({ ...c, tasks: tasks.filter(c.has) }));
	});
	const touch = typeof matchMedia !== 'undefined' && matchMedia('(pointer: coarse)').matches;
	let dragging = $state<string | null>(null);
	let over = $state<string | null>(null);
	function drop(columnId: string) {
		const col = columns.find((c) => c.id === columnId);
		const task = tasks.find((t) => t.id === dragging);
		dragging = over = null;
		if (col?.write && task && !col.has(task)) void writeColumn(ws, task, col.write);
	}

	// A table: the view's columns and summaries, read-only but for checking off.
	const table = $derived(readTable(view?.display));
	const cols = $derived(
		kind === 'table' && view
			? tableColumns(resolveDisplay(view.display, 'table', look).shown, table, look.fields)
			: ([] as TableColumn[])
	);
	const valueOf = (t: Task, c: TableColumn) =>
		c.field ? values.get(c.field.id)?.get(t.id) : undefined;
	const number = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 });
	const summaryCtx = $derived<SummaryContext>({
		today: localToday(),
		category: (t) =>
			statusById.get(t.statusId ?? '')?.category ?? (t.completedAt ? 'done' : 'todo'),
		projects: replica?.projects() ?? [],
		assignees: (tid) => replica?.assignees(tid) ?? [],
		labels: (tid) => (replica?.taskLabels(tid) ?? []).map((l) => l.id),
		values,
		nameOf: (uid) => session.nameOf(ws, uid),
		count: (t, what) =>
			what === 'subtasks'
				? (replica?.subtasks(t.id).length ?? 0)
				: what === 'checklist'
					? (replica?.checklist(t.id).length ?? 0)
					: what === 'comments'
						? (replica?.comments(t.id).length ?? 0)
						: 0,
		formatDate: (d) => formatDate(d),
		formatNumber: (n) => number.format(n),
		logged: (tid) => replica?.logged(tid) ?? 0
	});
	const untitled = (t: Task) => t.title || 'Untitled task';
</script>

<section class="embed" aria-label={`${title}, a saved view`} bind:clientWidth={width}>
	<header>
		<strong>{title}</strong>
		{#if view && shared}
			<span class="count"
				>{tasks.length === 1 ? '1 task' : `${tasks.length} tasks`}, as they are now</span
			>
			<a class="open" href={resolve('/(app)/views/[id]', { id })}>Open the view</a>
		{/if}
	</header>
	{#if !view}
		<p class="note">This view isn't here: it was deleted, or it's one you can't open.</p>
	{:else if !shared}
		<p class="note">This view is someone's own, so it doesn't show in a page.</p>
	{:else if !tasks.length}
		<p class="note">No tasks match this view right now.</p>
	{:else if kind === 'board'}
		<div class="board" role="group" aria-label={`${title} as a board`}>
			{#each columns as c (c.id)}
				{@const open = opened[c.id]}
				<div
					class="col"
					class:over={over === c.id}
					role="group"
					aria-label={c.title}
					ondragover={(e) => {
						if (!dragging) return;
						e.preventDefault();
						over = c.id;
					}}
					ondragleave={(e) => {
						if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node)) over = null;
					}}
					ondrop={(e) => {
						e.preventDefault();
						drop(c.id);
					}}
				>
					<p class="colhead"><b>{c.title}</b> <span class="n">{c.tasks.length}</span></p>
					<ul class="cards">
						{#each open ? c.tasks : c.tasks.slice(0, CARDS) as t (t.id)}
							<li
								class="card"
								class:done={!!t.completedAt}
								class:lifted={dragging === t.id}
								draggable={!touch}
								use:menu={taskActions(ws, t.id)}
								ondragstart={(e) => {
									dragging = t.id;
									// Not as text, which the page would take if dropped in it.
									e.dataTransfer?.setData('application/x-mila-card', t.id);
								}}
								ondragend={() => (dragging = over = null)}
							>
								<span class="line">
									<Checkbox
										label={`Complete ${untitled(t)}`}
										hideLabel
										checked={!!t.completedAt}
										onchange={(done) => void setTaskCompleted(ws, t, done)}
									/>
									<button type="button" class="task" onclick={() => openTask(t.id)}
										>{untitled(t)}</button
									>
								</span>
								{#if t.priority !== 'none' || t.dueDate}
									<span class="meta">
										{#if t.priority !== 'none'}<PriorityIcon priority={t.priority} />{/if}
										{#if t.dueDate}<span class="due">{formatDate(t.dueDate)}</span>{/if}
									</span>
								{/if}
							</li>
						{/each}
					</ul>
					{#if c.tasks.length > CARDS}
						<button type="button" class="colmore" onclick={() => (opened[c.id] = !open)}
							>{open ? 'Fewer' : `${c.tasks.length - CARDS} more`}</button
						>
					{/if}
				</div>
			{/each}
		</div>
	{:else if kind === 'table'}
		<div class="scroller">
			<table aria-label={`${title} as a table`}>
				<thead>
					<tr>
						{#each cols as c (c.id)}<th scope="col" class:end={c.end}>{c.label}</th>{/each}
					</tr>
				</thead>
				<tbody>
					{#each shown as t (t.id)}
						<tr class:done={!!t.completedAt} use:menu={taskActions(ws, t.id)}>
							{#each cols as c (c.id)}
								{#if c.kind === 'title'}
									<td class="sticky">
										<span class="titlecell">
											<Checkbox
												label={`Complete ${untitled(t)}`}
												hideLabel
												checked={!!t.completedAt}
												onchange={(done) => void setTaskCompleted(ws, t, done)}
											/>
											<button type="button" class="task" onclick={() => openTask(t.id)}
												>{untitled(t)}</button
											>
										</span>
									</td>
								{:else}
									<td class:end={c.end}
										><TableCell
											{ws}
											task={t}
											col={c}
											status={statusById.get(t.statusId ?? '')}
											value={valueOf(t, c)}
										/></td
									>
								{/if}
							{/each}
						</tr>
					{/each}
					<tr class="sum">
						{#each cols as c, i (c.id)}
							{@const s = summarize(summaryOf(table, c), c, tasks, summaryCtx)}
							<td class:sticky={i === 0} class:end={c.end}
								>{#if s?.kind === 'text'}<span title={s.said}>{s.text}</span>{:else if s}<span
										class="visually-hidden">{s.said}</span
									>{/if}</td
							>
						{/each}
					</tr>
				</tbody>
			</table>
		</div>
		{#if tasks.length > FIRST}
			<button type="button" class="more" onclick={() => (all = !all)}
				>{all ? 'Show fewer' : `Show all ${tasks.length}`}</button
			>
		{/if}
	{:else}
		<ul class="rows">
			{#each shown as t (t.id)}
				<li class:done={!!t.completedAt} use:menu={taskActions(ws, t.id)}>
					<Checkbox
						label={`Complete ${untitled(t)}`}
						hideLabel
						checked={!!t.completedAt}
						onchange={(done) => void setTaskCompleted(ws, t, done)}
					/>
					<button type="button" class="task" onclick={() => openTask(t.id)}>{untitled(t)}</button>
					{#if t.dueDate}<span class="due">{formatDate(t.dueDate)}</span>{/if}
				</li>
			{/each}
		</ul>
		{#if tasks.length > FIRST}
			<button type="button" class="more" onclick={() => (all = !all)}
				>{all ? 'Show fewer' : `Show all ${tasks.length}`}</button
			>
		{/if}
	{/if}
</section>

<style>
	.embed {
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		overflow: hidden;
		font-family: var(--font-ui);
		font-size: var(--text-md);
		line-height: var(--leading);
	}
	header {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-1) var(--space-3);
		padding: var(--space-2) var(--space-3);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-1);
	}
	strong {
		margin-right: auto;
		font-size: var(--text-sm);
	}
	.count {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.open,
	.more {
		font-size: var(--text-sm);
		font-weight: 600;
		color: var(--accent-text);
	}
	.open {
		display: inline-flex;
		align-items: center;
		min-height: max(var(--tap-min), 24px);
	}
	.more {
		display: block;
		min-height: max(var(--tap-min), 32px);
		margin: 0;
		padding: var(--space-2) var(--space-3);
		border: none;
		border-top: var(--border-width) solid var(--border);
		width: 100%;
		background: none;
		text-align: left;
		cursor: pointer;
	}
	/* As specific as the page's own list styles, which would push the rows in. */
	.embed ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.rows > li {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 34px);
		padding: 0 var(--space-3);
		border-top: var(--border-width) solid var(--border);
	}
	.rows > li:first-child {
		border-top: none;
	}
	.task {
		min-width: 0;
		min-height: max(var(--tap-min), 30px);
		padding: 0;
		border: none;
		background: none;
		color: var(--text-primary);
		font: inherit;
		font-size: var(--text-sm);
		text-align: left;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		cursor: pointer;
	}
	.task:hover {
		text-decoration: underline;
	}
	.rows > li.done .task {
		color: var(--text-muted);
		text-decoration: line-through;
	}
	.due {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		font-variant-numeric: tabular-nums;
	}
	.note {
		margin: 0;
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	/* A board: the view's columns side by side, scrolling sideways within the block. */
	.board {
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: minmax(190px, 1fr);
		gap: var(--space-2);
		padding: var(--space-2);
		overflow-x: auto;
	}
	.col {
		display: grid;
		align-content: start;
		gap: var(--space-1);
		min-height: 96px;
		padding: var(--space-2);
		border-radius: var(--radius-md);
		background: var(--surface-1);
	}
	.col.over {
		outline: 2px dashed var(--accent);
		outline-offset: -2px;
	}
	.colhead {
		margin: 0;
		padding: 0 var(--space-1);
		font-size: var(--text-xs);
	}
	.colhead .n {
		color: var(--text-muted);
	}
	.cards {
		display: grid;
		gap: var(--space-1);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.card {
		display: grid;
		gap: var(--space-1);
		padding: var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	.card.lifted {
		opacity: 0.5;
	}
	.line {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.card .task {
		flex: 1;
		min-height: max(var(--tap-min), 24px);
		white-space: normal;
	}
	.card.done .task {
		color: var(--text-muted);
		text-decoration: line-through;
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.colmore {
		justify-self: start;
		min-height: max(var(--tap-min), 24px);
		padding: 0 var(--space-1);
		border: none;
		background: none;
		color: var(--accent-text);
		font-size: var(--text-xs);
		font-weight: 600;
		cursor: pointer;
	}
	/* A table: the view's columns, its title held at the left while it scrolls. */
	.scroller {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
	}
	th {
		padding: var(--space-1) var(--space-3);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
		text-align: left;
		white-space: nowrap;
	}
	td {
		padding: var(--space-1) var(--space-3);
		border-top: var(--border-width) solid var(--border);
		white-space: nowrap;
	}
	th.end,
	td.end {
		text-align: right;
	}
	td.sticky {
		position: sticky;
		left: 0;
		min-width: 200px;
		background: var(--surface-0);
	}
	.titlecell {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	tr.done .task {
		color: var(--text-muted);
		text-decoration: line-through;
	}
	tr.sum td {
		background: var(--surface-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
	}
</style>
