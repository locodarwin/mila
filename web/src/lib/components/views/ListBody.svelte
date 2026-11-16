<script lang="ts">
	// What Inbox, My Day or Upcoming shows once its view, filter or display is
	// set (ListViews.svelte): a board, a calendar, a table, a timeline, or the
	// list filtered, sorted or grouped. Loaded only then, since a list as it comes
	// needs none of it.
	import type { Snippet } from 'svelte';
	import TaskSections from '../TaskSections.svelte';
	import { announce } from '../../announce.svelte';
	import { formatDate } from '../../dates';
	import { matches } from '../../filter';
	import { filterContextOf } from '../../viewList';
	import type { FixedList, ListSetup } from '../../listViews.svelte';
	import { canOrganize } from '../../roles';
	import { session } from '../../session.svelte';
	import { createTaskIn, localToday, setInMyDay, type ViewEdit } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { Status, Task, ViewDisplay } from '../../sync/entities';
	import { boardColumns } from '../../boardColumns';
	import {
		groupTasks,
		resolveDisplay,
		sortSaid,
		sortTasks,
		type ColumnWrite,
		type DisplayLookups,
		type DisplayPath
	} from '../../viewDisplay';

	interface Section {
		id: string;
		title: string;
		tasks: Task[];
		tone?: 'danger';
		action?: Snippet;
	}

	interface Props {
		ws: string;
		list: FixedList;
		setup: ListSetup;
		scope: Task[];
		sectionsOf: (keep: (task: Task) => boolean) => Section[];
		/** Whether nothing is set but the view: the list's own empty words then. */
		unfiltered: boolean;
		onchange: (
			edit: ViewEdit<ViewDisplay, Partial<Record<DisplayPath, unknown>>>,
			say?: (display: ViewDisplay) => string
		) => void;
		label: string;
		emptyTitle: string;
		emptyHint: string;
	}

	let {
		ws,
		list,
		setup,
		scope,
		sectionsOf,
		unfiltered,
		onchange,
		label,
		emptyTitle,
		emptyHint
	}: Props = $props();

	const replica = $derived(engine.replica(ws));
	const me = $derived(session.user?.id ?? '');
	const statusById = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- made afresh by each derivation
		const out = new Map<string, Status>();
		for (const p of replica?.projects() ?? [])
			for (const s of replica?.statuses(p.id) ?? []) out.set(s.id, s);
		return out;
	});
	const values = $derived(replica?.fieldValuesByField() ?? new Map());
	const look = $derived<DisplayLookups>({
		projectId: null,
		fields: replica?.fields() ?? [],
		inbox: list === 'inbox',
		columns: list === 'upcoming' ? { by: 'due' } : undefined
	});
	const ctx = $derived(replica ? filterContextOf(replica, me, statusById, values) : undefined);
	const keep = $derived.by(() => {
		const f = setup.filter;
		const c = ctx;
		return (t: Task) => !!c && matches(t, f, c);
	});
	const kept = $derived(scope.filter(keep));

	// How many a filter change leaves, said as it happens, as on a saved view.
	let lastFilter = '';
	$effect(() => {
		const now = JSON.stringify(setup.filter);
		const n = kept.length;
		if (lastFilter && now !== lastFilter) announce(`${n} ${n === 1 ? 'task' : 'tasks'}`);
		lastFilter = now;
	});
	const resolved = $derived(resolveDisplay(setup.display, setup.type, look));
	// A list grouped or sorted another way than its own.
	const regrouped = $derived(
		setup.type === 'list' && (setup.display.group !== undefined || resolved.sort.length > 0)
	);
	const board = $derived(setup.type === 'board');
	// A board in the list's own order, unless the display sorts it.
	const sorted = $derived(
		(regrouped || (board && resolved.sort.length > 0)) && ctx
			? sortTasks(kept, resolved.sort, {
					category: ctx.category,
					status: (t) => (t.statusId ? statusById.get(t.statusId) : undefined),
					projects: replica?.projects() ?? [],
					times: (id) => replica?.taskTimes(id) ?? { created: '', updated: '' },
					fields: look.fields,
					values,
					memberName: (id) => session.memberName(ws, id),
					logged: (id) => replica?.logged(id) ?? 0
				})
			: []
	);
	const members = $derived(session.members(ws).map((m) => ({ userId: m.userId, name: m.name })));
	const sections = $derived.by((): Section[] => {
		if (setup.type !== 'list' || !ctx) return [];
		if (!regrouped) return sectionsOf(keep);
		const by = setup.display.group === undefined ? { by: 'none' as const } : resolved.group;
		return groupTasks(sorted, by, {
			me,
			today: localToday(),
			projectId: null,
			projects: replica?.projects() ?? [],
			statuses: [],
			category: ctx.category,
			labels: replica?.labels() ?? [],
			taskLabels: ctx.labels,
			assignees: ctx.assignees,
			fields: look.fields,
			values,
			members,
			nameOf: (id) => session.nameOf(ws, id)
		});
	});
	const shown = $derived(new Set(resolved.shown));
	const sortedBy = $derived(sortSaid(resolved.sort, look.fields));
	const fixed = $derived(
		resolved.sort.length > 0
			? `${sortedBy}. Choose manual order to move tasks in the list.`
			: regrouped
				? 'This list mixes projects, each in its own order. Open a project to move tasks.'
				: undefined
	);

	// A board's columns are what every task has (priority, due date, or a
	// field of the workspace's), since statuses are a project's own. Upcoming's
	// are its days by default, and hold only open, dated tasks.
	const columns = $derived(
		board
			? boardColumns(resolved.columns, {
					me,
					statuses: [],
					fields: look.fields,
					values,
					members,
					nameOf: (id) => session.nameOf(ws, id),
					today: localToday(),
					dayName: (day) => formatDate(day, { weekday: 'long', year: false }),
					dated: list === 'upcoming'
				})
			: []
	);
	const cards = $derived(board && resolved.sort.length > 0 ? sorted : kept);
	// The Inbox's tasks share one order; My Day and Upcoming mix projects'.
	const mixed = $derived(list !== 'inbox');
	const organizes = $derived(!!session.workspace && canOrganize(session.workspace.role));
	// A task added to a column: to the Inbox, and in My Day to today too.
	// Upcoming adds one only to a day, since a task without a date is not in it.
	const addTask = $derived.by(() => {
		if (!organizes || (list === 'upcoming' && resolved.columns.by !== 'due')) return null;
		if (list !== 'myday')
			return (title: string, write: ColumnWrite) => createTaskIn(ws, null, title, write);
		return (title: string, write: ColumnWrite) =>
			engine.group(async () => {
				const id = await createTaskIn(ws, null, title, write);
				await setInMyDay(ws, me, id, true);
			});
	});
</script>

{#if board}
	{#await import('./Board.svelte') then { default: Board }}
		<Board
			{ws}
			projectId={null}
			tasks={cards}
			{columns}
			manual={!mixed && resolved.sort.length === 0}
			{sortedBy}
			fixed={mixed && resolved.sort.length === 0
				? 'This board mixes projects, each in its own order. Open a project to move cards within a column.'
				: undefined}
			{shown}
			{addTask}
		/>
	{/await}
{:else if setup.type === 'calendar'}
	{#await import('./Calendar.svelte') then { default: Calendar }}
		<Calendar {ws} tasks={kept} />
	{/await}
{:else if setup.type === 'table'}
	{#await import('./TableView.svelte') then { default: TableView }}
		<TableView
			{ws}
			projectId={null}
			{scope}
			filter={setup.filter}
			display={setup.display}
			{onchange}
			{label}
			emptyTitle={unfiltered ? emptyTitle : 'Nothing matches'}
			emptyHint={unfiltered ? emptyHint : 'Change the filter to see more.'}
		/>
	{/await}
{:else if setup.type === 'timeline'}
	{#await import('./TimelineView.svelte') then { default: TimelineView }}
		<TimelineView
			{ws}
			projectId={null}
			{scope}
			filter={setup.filter}
			display={setup.display}
			{onchange}
			{label}
			emptyTitle={unfiltered ? emptyTitle : 'Nothing matches'}
			emptyHint={unfiltered ? emptyHint : 'Change the filter to see more.'}
		/>
	{/await}
{:else}
	<TaskSections
		{ws}
		{sections}
		{shown}
		{fixed}
		{label}
		emptyTitle="Nothing matches"
		emptyHint="Change the filter to see more, or Reset."
	/>
{/if}
