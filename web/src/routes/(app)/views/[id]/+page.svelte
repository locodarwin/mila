<script lang="ts">
	import { menu, viewActions } from '#lib/contextMenu.svelte.js';
	import { tick, untrack } from 'svelte';
	import { intents } from '#lib/intents.svelte.js';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import CalendarPlus from '@lucide/svelte/icons/calendar-plus';
	import Kanban from '@lucide/svelte/icons/kanban';
	import List from '@lucide/svelte/icons/list';
	import ListFilter from '@lucide/svelte/icons/list-filter';
	import Share2 from '@lucide/svelte/icons/share-2';
	import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
	import Table2 from '@lucide/svelte/icons/table-2';
	import ChartGantt from '@lucide/svelte/icons/chart-gantt';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import TaskSections from '#lib/components/TaskSections.svelte';
	import { filterCount, type FilterContext } from '#lib/filter.js';
	import { filterContextOf, tasksOfView } from '#lib/viewList.js';
	import { canEditViews } from '#lib/roles.js';
	import { session } from '#lib/session.svelte.js';
	import { formatDate } from '#lib/dates.js';
	import { deleteView, editView, localToday, shareView, type ViewEdit } from '#lib/sync/data.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import type { FieldType, Status, ViewDisplay, ViewFilter, ViewType } from '#lib/sync/entities.js';
	import { toasts } from '#lib/toasts.svelte.js';
	import { announce } from '#lib/announce.svelte.js';
	import {
		applyPaths,
		differs,
		groupTasks,
		resolveDisplay,
		sortSaid,
		sortTasks,
		type DisplayLookups,
		type DisplayPath
	} from '#lib/viewDisplay.js';

	const ws = $derived(session.workspace?.id ?? '');
	const replica = $derived(engine.replica(ws));
	const saved = $derived(replica?.view(page.params.id ?? ''));
	const me = $derived(session.user?.id ?? '');
	const visible = $derived(!!saved && (saved.ownerId === null || saved.ownerId === me));
	const project = $derived(saved?.projectId ? replica?.project(saved.projectId) : undefined);
	// A guest cannot write views: their changes stay on this screen only.
	const canSave = $derived(!!session.workspace && canEditViews(session.workspace.role));
	let filtering = $state(false);
	let displaying = $state(false);
	let filterButton = $state<HTMLButtonElement>();
	let displayButton = $state<HTMLButtonElement>();

	// A guest's changes, over the saved view, for this view only.
	let local = $state<{
		id: string;
		type?: ViewType;
		filter?: ViewFilter;
		display?: ViewDisplay;
	} | null>(null);
	const mine = $derived(local && local.id === saved?.id ? local : null);
	const view = $derived(
		saved && {
			...saved,
			type: mine?.type ?? saved.type,
			filter: mine?.filter ?? saved.filter,
			display: mine?.display ?? saved.display
		}
	);

	const types = [
		{ id: 'list' as const, label: 'List', icon: List },
		{ id: 'board' as const, label: 'Board', icon: Kanban },
		{ id: 'calendar' as const, label: 'Calendar', icon: CalendarDays },
		{ id: 'table' as const, label: 'Table', icon: Table2 },
		{ id: 'timeline' as const, label: 'Timeline', icon: ChartGantt }
	];

	// Each change to the workspace works these out again, in order: what the
	// filter needs, the tasks it leaves, their order, then their sections or
	// columns. Field values are read in one pass.
	const values = $derived(replica?.fieldValuesByField() ?? new Map());
	const allFields = $derived(replica?.fields() ?? []);
	const look = $derived<DisplayLookups>({
		projectId: view?.projectId ?? null,
		fields: view?.projectId ? (replica?.fields(view.projectId) ?? []) : allFields,
		// A view of the Inbox alone, as Save as view makes one from it.
		inbox: !view?.projectId && view?.filter?.projects?.join() === 'inbox'
	});
	const statusById = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- made afresh by each derivation
		const out = new Map<string, Status>();
		for (const p of replica?.projects() ?? [])
			for (const s of replica?.statuses(p.id) ?? []) out.set(s.id, s);
		return out;
	});
	const ctx = $derived.by((): FilterContext => {
		if (replica) return filterContextOf(replica, me, statusById, values);
		const types = new Map<string, FieldType>(allFields.map((f) => [f.id, f.type]));
		return {
			me,
			today: localToday(),
			category: (t) => (t.completedAt ? 'done' : 'todo'),
			labels: () => [],
			assignees: () => [],
			blocked: () => false,
			blocking: () => false,
			entries: () => [],
			fieldType: (id) => types.get(id),
			value: () => undefined
		};
	});
	const resolved = $derived(resolveDisplay(view?.display, view?.type ?? 'list', look));
	const tasks = $derived.by(() => {
		if (!view || !replica) return [];
		// Across projects, tasks in archived or deleted projects stay out.
		return tasksOfView(replica, view, ctx);
	});
	const sorted = $derived(
		sortTasks(tasks, resolved.sort, {
			category: ctx.category,
			status: (t) => (t.statusId ? statusById.get(t.statusId) : undefined),
			projects: replica?.projects() ?? [],
			times: (id) => replica?.taskTimes(id) ?? { created: '', updated: '' },
			fields: look.fields,
			values,
			memberName: (id) => session.memberName(ws, id),
			logged: (id) => replica?.logged(id) ?? 0
		})
	);
	const members = $derived(session.members(ws).map((m) => ({ userId: m.userId, name: m.name })));
	const sections = $derived(
		view?.type === 'list'
			? groupTasks(sorted, resolved.group, {
					me,
					today: localToday(),
					projectId: view.projectId,
					projects: replica?.projects() ?? [],
					statuses: view.projectId ? (replica?.statuses(view.projectId) ?? []) : [],
					category: ctx.category,
					labels: replica?.labels() ?? [],
					taskLabels: ctx.labels,
					assignees: ctx.assignees,
					fields: look.fields,
					values,
					members,
					nameOf: (id) => session.nameOf(ws, id)
				})
			: []
	);
	// A board's columns come with the board, which loads only when shown.
	let boards = $state<typeof import('#lib/boardColumns.js') | null>(null);
	$effect(() => {
		if (view?.type === 'board' && !boards)
			void import('#lib/boardColumns.js').then((m) => (boards = m));
	});
	const columns = $derived(
		view?.type === 'board' && boards
			? boards.boardColumns(resolved.columns, {
					me,
					statuses: view.projectId ? (replica?.statuses(view.projectId) ?? []) : [],
					fields: look.fields,
					values,
					members,
					nameOf: (id) => session.nameOf(ws, id),
					today: localToday(),
					dayName: (day) => formatDate(day, { weekday: 'long', year: false }),
					// A view of dated tasks, as Save as view makes from Upcoming.
					dated: view.filter.due === 'dated'
				})
			: []
	);
	const shown = $derived(new Set(resolved.shown));
	const manual = $derived(resolved.sort.length === 0);
	const sortedBy = $derived(sortSaid(resolved.sort, look.fields));
	// Sort keys order tasks within their own project (or the inbox), so a
	// section holding tasks of several projects has no one order to move
	// a task in: across projects, only grouping by project keeps it.
	const fixed = $derived(
		!manual
			? `${sortedBy}. Choose manual order to move tasks in the list.`
			: !view?.projectId && resolved.group.by !== 'project'
				? 'This list mixes projects, each in its own order. Group by project to move tasks.'
				: undefined
	);
	const customized = $derived(!!view && differs(resolved, view.type, look));

	let name = $derived(saved?.name ?? '');
	function save() {
		const n = name.trim();
		if (saved && canSave && n && n !== saved.name) void editView(ws, saved.id, { name: n });
		else name = saved?.name ?? '';
	}

	/** Keeps a guest's change on this screen, over what they had already changed. */
	function keep(change: { type?: ViewType; filter?: ViewFilter; display?: ViewDisplay }) {
		if (!view) return;
		local = { id: view.id, type: view.type, filter: view.filter, display: view.display, ...change };
	}

	const setType = (type: ViewType) => {
		if (!view) return;
		if (canSave) void editView(ws, view.id, { type });
		else keep({ type });
	};

	// How many tasks a changed filter leaves is said aloud, since the list
	// changes away from where focus is.
	const setFilter = async (edit: ViewEdit<ViewFilter, Partial<ViewFilter>>) => {
		if (!view) return;
		if (canSave) await editView(ws, view.id, { filter: edit });
		else {
			const filter: Record<string, unknown> = { ...view.filter };
			const patch = typeof edit === 'function' ? edit(view.filter) : edit;
			for (const [k, v] of Object.entries(patch)) {
				if (v === undefined) delete filter[k];
				else filter[k] = v;
			}
			keep({ filter: filter as ViewFilter });
		}
		announce(`${tasks.length} ${tasks.length === 1 ? 'task matches' : 'tasks match'}`, 700);
	};

	const setDisplay = async (
		edit: ViewEdit<ViewDisplay, Partial<Record<DisplayPath, unknown>>>,
		say?: (display: ViewDisplay) => string
	) => {
		if (!view) return;
		if (canSave) await editView(ws, view.id, { display: edit });
		else
			keep({
				display: applyPaths(view.display, typeof edit === 'function' ? edit(view.display) : edit)
			});
		// What is said reads the display as it now is.
		const said = view && say?.(view.display);
		if (said) announce(said);
	};

	function closePanel(which: 'filter' | 'display') {
		if (which === 'filter') {
			filtering = false;
			filterButton?.focus();
		} else {
			displaying = false;
			displayButton?.focus();
		}
	}

	async function share() {
		if (!saved) return;
		// The personal view is gone once this resolves, so read it first.
		const shared = saved.name;
		const id = await shareView(ws, saved);
		toasts.show(`Shared ${shared} with the workspace`);
		await goto(resolve('/(app)/views/[id]', { id }), { replace: true });
	}

	// What a right-click menu elsewhere asked of this view (menus.ts).
	let nameField = $state<HTMLInputElement>();
	$effect(() => {
		void intents.pending;
		const id = saved?.id;
		const what = untrack(() => intents.take('view', id));
		if (!what) return;
		void tick().then(() => {
			if (what === 'rename') {
				nameField?.focus();
				nameField?.select();
			} else if (what === 'filters') filtering = true;
		});
	});

	async function remove() {
		if (!saved) return;
		const { id, name: gone } = saved;
		// Undo takes back this delete, not whatever was edited after it.
		const { done, undo } = engine.track(() => deleteView(ws, id));
		await done;
		toasts.show(`Deleted the view ${gone}`, { onundo: () => void undo() });
		await goto(resolve('/(app)'));
	}
</script>

<svelte:head><title>{view?.name ?? 'View'} · Mila</title></svelte:head>

{#if view && visible}
	<div class="frame" data-screen>
		<header class="page" use:menu={viewActions(ws, view.id)}>
			<!-- The name is a field; the page still needs a heading named after it. -->
			<h1 class="visually-hidden">{view.name}</h1>
			<input
				class="name"
				aria-label="View name"
				bind:this={nameField}
				readonly={!canSave}
				bind:value={name}
				onblur={save}
				onkeydown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
			/>
			<span class="scope">
				{project ? project.name : 'All projects'} · {view.ownerId ? 'Only you' : 'Shared'}
			</span>
			{#if view.type !== 'report'}
				<div class="types" role="group" aria-label="Show as">
					{#each types as t (t.id)}
						<button
							type="button"
							aria-label={t.label}
							aria-pressed={view.type === t.id}
							onclick={() => setType(t.id)}
						>
							<t.icon size={15} /><span>{t.label}</span>
						</button>
					{/each}
				</div>
				<button
					title="Choose which tasks this view shows"
					class="tool"
					type="button"
					aria-expanded={filtering}
					bind:this={filterButton}
					onclick={() => (filtering = !filtering)}
				>
					<ListFilter size={15} /><span class="word">Filter</span>
					{#if filterCount(view.filter)}<span class="badge tabular">{filterCount(view.filter)}</span
						>{/if}
				</button>
				{#if view.type !== 'calendar'}
					<button
						title="Choose what shows, and how it is sorted and grouped"
						class="tool"
						type="button"
						aria-expanded={displaying}
						aria-describedby={customized ? 'display-changed' : undefined}
						bind:this={displayButton}
						onclick={() => (displaying = !displaying)}
					>
						<SlidersHorizontal size={15} /><span class="word">Display</span>
						{#if customized}<span class="dot" aria-hidden="true"></span>{/if}
					</button>
					<span id="display-changed" hidden>Changed from the defaults</span>
				{/if}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- resolved, with a query -->
				<a
					class="icon"
					href={`${resolve('/(app)/settings/w/[ws]/calendar', { ws })}?view=${view.id}`}
					aria-label="Subscribe in a calendar app"
					title="Subscribe in a calendar app"
				>
					<CalendarPlus size={16} />
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{/if}
			{#if canSave && view.ownerId}
				<button class="icon" type="button" aria-label="Share with the workspace" onclick={share}>
					<Share2 size={16} />
				</button>
			{/if}
			{#if canSave}
				<button class="icon" type="button" aria-label={`Delete ${view.name}`} onclick={remove}>
					<Trash2 size={16} />
				</button>
			{/if}
		</header>
		{#if mine}
			<div class="unsaved" role="region" aria-label="Unsaved changes">
				<span>Changes here are only for you and are not saved.</span>
				<button type="button" title="Go back to the view as saved" onclick={() => (local = null)}
					>Reset</button
				>
			</div>
		{/if}
		{#if filtering}
			<!-- Loaded when first opened, as Display is. -->
			{#await import('#lib/components/views/FilterPanel.svelte') then { default: FilterPanel }}
				<FilterPanel
					{ws}
					filter={view.filter}
					acrossProjects={!view.projectId}
					projectId={view.projectId}
					onchange={setFilter}
					onclose={() => closePanel('filter')}
				/>
			{/await}
		{/if}
		{#if displaying && view.type !== 'calendar' && view.type !== 'report'}
			<!-- Loaded when first opened, which keeps the page's first load small. -->
			{#await import('#lib/components/views/DisplayPanel.svelte') then { default: DisplayPanel }}
				<DisplayPanel
					type={view.type}
					display={view.display}
					{look}
					onchange={setDisplay}
					onclose={() => closePanel('display')}
				/>
			{/await}
		{/if}
		<div class="body">
			<!-- The board and the calendar, like the table and timeline, load only when shown. -->
			{#if view.type === 'report'}
				<!-- The Reports page under the view's name, set up as the view is. -->
				{#await import('#lib/components/reports/ReportView.svelte') then { default: ReportView }}
					<ReportView {ws} {view} {setFilter} {setDisplay} />
				{/await}
			{:else if view.type === 'board' && boards}
				<!-- Across projects, cards keep their projects' orders, and a new
				     one would have no project to go in. -->
				{#await import('#lib/components/views/Board.svelte') then { default: Board }}
					<Board
						{ws}
						projectId={view.projectId}
						tasks={sorted}
						{columns}
						manual={manual && !!view.projectId}
						{sortedBy}
						fixed={manual && !view.projectId
							? 'This board mixes projects, each in its own order. Open a project to move cards within a column.'
							: undefined}
						{shown}
					/>
				{/await}
			{:else if view.type === 'calendar'}
				{#await import('#lib/components/views/Calendar.svelte') then { default: Calendar }}
					<Calendar {ws} {tasks} />
				{/await}
			{:else if view.type === 'timeline'}
				<!-- The timeline loads only when one is shown. -->
				{#await import('#lib/components/views/TimelineView.svelte') then { default: TimelineView }}
					<TimelineView
						{ws}
						projectId={view.projectId}
						filter={view.filter}
						display={view.display}
						onchange={setDisplay}
						label="Tasks"
						emptyTitle="Nothing matches"
						emptyHint="Change the filter to see more."
					/>
				{/await}
			{:else if view.type === 'table'}
				<!-- The table, its sorting and summaries load only when one is shown. -->
				{#await import('#lib/components/views/TableView.svelte') then { default: TableView }}
					<TableView
						{ws}
						projectId={view.projectId}
						filter={view.filter}
						display={view.display}
						onchange={setDisplay}
						label="Tasks"
						emptyTitle="Nothing matches"
						emptyHint="Change the filter to see more."
					/>
				{/await}
			{:else}
				<div class="list">
					<TaskSections
						{ws}
						{sections}
						{shown}
						{fixed}
						label="Tasks"
						emptyTitle="Nothing matches"
						emptyHint="Change the filter to see more."
					/>
				</div>
			{/if}
		</div>
	</div>
{:else}
	<header class="page"><h1>View</h1></header>
	<EmptyState title="This view isn't here" hint="It may have been deleted or shared again." />
{/if}

<style>
	/* One screen tall, less what the layout puts over it. */
	.frame {
		display: flex;
		flex-direction: column;
		height: calc(100% - var(--above, 0px));
	}
	/* The header and the panels keep their height; the tasks below take
	   what is left and scroll. */
	.frame > :global(:not(.body)) {
		flex: none;
	}
	.page {
		flex-wrap: wrap;
		height: auto;
		min-height: 52px;
		padding-block: var(--space-2);
	}
	.name {
		flex: 1;
		min-width: 8rem;
		padding: 2px var(--space-1);
		border: var(--border-width) solid transparent;
		border-radius: var(--radius-sm);
		background: none;
		font-family: var(--font-heading);
		font-size: var(--text-lg);
		font-weight: var(--heading-weight);
	}
	.name:hover {
		border-color: var(--border);
	}
	.scope {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.types {
		display: inline-flex;
		padding: 2px;
		border-radius: var(--radius-md);
		background: var(--surface-2);
	}
	.types button,
	.tool {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: calc(var(--radius-md) - 2px);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.types button[aria-pressed='true'] {
		background: var(--surface-0);
		color: var(--text-primary);
		box-shadow: var(--shadow-sm);
	}
	.tool:hover,
	.tool[aria-expanded='true'] {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--accent);
	}
	.unsaved {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-4);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--accent-soft);
		font-size: var(--text-sm);
	}
	.unsaved button {
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.badge {
		min-width: 18px;
		padding: 0 4px;
		border-radius: var(--radius-full);
		background: var(--accent);
		color: var(--accent-contrast);
		font-size: var(--text-xs);
		text-align: center;
	}
	.icon {
		display: grid;
		place-items: center;
		width: 32px;
		height: 32px;
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.icon:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.body {
		min-height: 0;
		flex: 1;
		overflow: auto;
	}
	.list {
		/* The last row scrolls clear of the phone's FAB. */
		padding: var(--space-2) var(--space-4) calc(var(--space-2) + var(--fab-room, 0px));
	}
	/* A task panel open takes the room from here, whatever the window's width:
	   the rest gives up its words before the name gives up its room. */
	.frame {
		container-type: inline-size;
	}
	@container (max-width: 1100px) {
		.types span,
		.scope {
			display: none;
		}
		/* The words stay as the buttons' names. */
		.tool .word {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
			white-space: nowrap;
		}
	}
</style>
