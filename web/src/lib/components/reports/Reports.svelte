<script lang="ts">
	// The Reports page (docs/PLAN.md section 3, Release 3.0): three tabs,
	// Project status, Workload, and Throughput and cycle time, each worked
	// out on this device from the tasks it holds (lib/reports.ts), narrowed
	// by a saved view's filter and, where it counts weeks, a period. The tab,
	// the period and the filter are kept on this device for the workspace
	// (lib/reportSetup.svelte.ts). Every chart has its numbers as a table,
	// and each tab downloads its tables as one CSV file.
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import Clock from '@lucide/svelte/icons/clock';
	import Download from '@lucide/svelte/icons/download';
	import Hourglass from '@lucide/svelte/icons/hourglass';
	import ListFilter from '@lucide/svelte/icons/list-filter';
	import Timer from '@lucide/svelte/icons/timer';
	import { tick } from 'svelte';
	import { announce } from '../../announce.svelte';
	import { formatDate, weekStart } from '../../dates';
	import { formatDuration } from '../../duration';
	import { filterCount, matches } from '../../filter';
	import { Growing } from '../../growing.svelte';
	import {
		EARLIEST_DAY,
		deviceReport,
		reportDisplay,
		type ReportSource
	} from '../../reportSetup.svelte';
	import {
		bucketNames,
		chartOf,
		dailyFlow,
		daysBetween,
		flowReport,
		foldShares,
		inScope,
		LATENESS,
		openBy,
		overdueAges,
		periodOf,
		priorityShares,
		ranked,
		REPORT_TABS,
		sayDays,
		statusReport,
		statusShares,
		tablesCsv,
		workloadReport,
		type CsvTable,
		type DueBucket,
		type PeriodKind,
		type ReportChart,
		type ReportTab,
		type Share,
		type Week
	} from '../../reports';
	import { session } from '../../session.svelte';
	import { createView, localToday, type ViewEdit } from '../../sync/data';
	import { toasts } from '../../toasts.svelte';
	import BookmarkPlus from '@lucide/svelte/icons/bookmark-plus';
	import { engine } from '../../sync/engine.svelte';
	import type { Task, ViewFilter } from '../../sync/entities';
	import { openTask } from '../../taskLink';
	import { filterContextOf, statusesById } from '../../viewList';
	import Button from '../Button.svelte';
	import HelpLink from '../help/HelpLink.svelte';
	import BarChart from './BarChart.svelte';
	import ChartCard from './ChartCard.svelte';
	import ColumnChart from './ColumnChart.svelte';
	import LineChart from './LineChart.svelte';
	import Scatter from './Scatter.svelte';
	import StackedArea from './StackedArea.svelte';
	import StackedBar from './StackedBar.svelte';

	interface Props {
		ws: string;
		/** A person's name; the style guide's sample people have theirs here. */
		nameOf?: (userId: string) => string;
		/** Keeps the tab out of the page's address: for the style guide. */
		embedded?: boolean;
		/** Where the setup comes from and goes to: a report view's, or this device's. */
		source?: ReportSource;
		/** A report view's own project, which it reports on alone. */
		fixedProject?: string | null;
		/** One chart alone, for a report in a page: no tabs or controls. */
		only?: ReportChart;
		/** Offers Save as view: on the Reports page, for those who may save views. */
		saveable?: boolean;
	}

	let {
		ws,
		nameOf,
		embedded = false,
		source,
		fixedProject = null,
		only,
		saveable = false
	}: Props = $props();

	const SERIES = [
		'--chart-series-1',
		'--chart-series-2',
		'--chart-series-3',
		'--chart-series-4',
		'--chart-series-5',
		'--chart-series-6'
	];
	/** The parts folded into Other. */
	const OTHER = '--border-strong';
	const TABS: { id: ReportTab; label: string }[] = [
		{ id: 'status', label: 'Project status' },
		{ id: 'workload', label: 'Workload' },
		{ id: 'flow', label: 'Throughput and cycle time' }
	];
	const PERIODS: { kind: PeriodKind; label: string }[] = [
		{ kind: 'weeks6', label: 'Last 6 weeks' },
		{ kind: 'months3', label: 'Last 3 months' },
		{ kind: 'year', label: 'This year' },
		{ kind: 'range', label: 'Pick dates' }
	];

	const replica = $derived(engine.replica(ws));
	const me = $derived(session.user?.id ?? '');
	const src = $derived(source ?? deviceReport(ws));
	const setup = $derived(src.setup);
	// The Reports page itself keeps its tab and project in the address too.
	const linked = $derived(!embedded && !source && !only);
	const asked = $derived(linked ? page.url.searchParams.get('tab') : null);
	const tab = $derived<ReportTab>(
		only
			? (chartOf(only)?.tab ?? 'status')
			: REPORT_TABS.includes(asked as ReportTab)
				? (asked as ReportTab)
				: setup.tab
	);
	/** Whether a chart shows: every one, or the one a page asks for. */
	const shows = (key: ReportChart) => !only || only === key;
	const today = localToday();
	const first = $derived(weekStart());
	const period = $derived(periodOf(setup.period, today, first, setup));
	// The one project reported on, from the address or as last chosen here;
	// none (all of them) when it is archived or gone.
	const projects = $derived(replica?.projects() ?? []);
	const askedProject = $derived(linked ? page.url.searchParams.get('project') : null);
	const projectId = $derived.by(() => {
		const id = fixedProject ?? askedProject ?? setup.project;
		return id && projects.some((p) => p.id === id) ? id : null;
	});
	// A project the address names, such as from its menu, is the one chosen here from then on.
	$effect(() => {
		if (askedProject && askedProject === projectId && setup.project !== askedProject)
			src.change({ project: askedProject });
	});
	const personName = (id: string) => nameOf?.(id) ?? session.nameOf(ws, id);

	// The tasks every tab counts: the filter keeps finished ones too.
	const statusById = $derived(replica ? statusesById(replica) : new Map());
	const ctx = $derived(replica ? filterContextOf(replica, me, statusById) : null);
	const all = $derived(replica?.allTasks() ?? []);
	const tasks = $derived.by((): Task[] => {
		if (!replica || !ctx) return [];
		const filter: ViewFilter = { ...setup.filter, showDone: true };
		const live = new Set(replica.projects().map((p) => p.id));
		const one = projectId;
		return inScope({
			tasks: all,
			task: (id) => replica.task(id),
			projectInUse: (id) => live.has(id),
			category: ctx.category,
			keep: (t) => (!one || t.projectId === one) && matches(t, filter, ctx)
		});
	});
	const filtered = $derived(filterCount(setup.filter) > 0);

	const status = $derived(
		ctx && replica
			? statusReport(tasks, {
					period,
					today,
					weekStart: first,
					category: ctx.category,
					openBlockers: (id) => replica.openBlockers(id).length,
					logged: (id) => replica.logged(id)
				})
			: null
	);
	const workload = $derived(
		ctx && replica
			? workloadReport(tasks, {
					today,
					weekStart: first,
					category: ctx.category,
					assignees: (id) => replica.assignees(id),
					nameOf: personName
				})
			: null
	);
	const flow = $derived(
		ctx ? flowReport(tasks, { period, weekStart: first, category: ctx.category }) : null
	);

	// The charts added with the project picker: shares, ranks and flow by day.
	const statusAll = $derived(
		ctx && replica
			? statusShares(tasks, {
					category: ctx.category,
					statuses: projectId
						? replica.statuses(projectId).map((s) => ({ id: s.id, name: s.name }))
						: undefined
				})
			: []
	);
	const statusParts = $derived(foldShares(statusAll));
	const priorityAll = $derived(ctx ? priorityShares(tasks, ctx.category) : []);
	const priorityParts = $derived(foldShares(priorityAll));
	// Colors follow what a part is, never where it falls: kinds of status as
	// "Where the open work is" has them, priorities and a project's statuses
	// by their own order, whichever of them have tasks.
	const KIND_COLORS: Record<string, string> = {
		backlog: SERIES[0],
		todo: SERIES[1],
		'in-progress': SERIES[2],
		done: SERIES[3]
	};
	const colorsOf = (all: Share[], kinds = false) =>
		new Map(
			all.map((p, i) => [
				p.key,
				kinds ? (KIND_COLORS[p.key] ?? OTHER) : i < SERIES.length ? SERIES[i] : OTHER
			])
		);
	const statusColors = $derived(colorsOf(statusAll, !projectId));
	const priorityColors = $derived(colorsOf(priorityAll));
	const lateness = $derived(ctx ? overdueAges(tasks, { today, category: ctx.category }) : []);
	const labelNames = $derived(new Map((replica?.labels() ?? []).map((l) => [l.id, l.name])));
	const labelBars = $derived(
		ctx && replica
			? ranked(
					openBy(tasks, {
						category: ctx.category,
						keys: (t) => replica.taskLabels(t.id).map((l) => l.id),
						nameOf: (id) => labelNames.get(id) ?? 'A label'
					})
				)
			: []
	);
	const projectBars = $derived(
		ctx && !projectId
			? ranked(
					openBy(tasks, {
						category: ctx.category,
						keys: (t) => [t.projectId ?? ''],
						nameOf: (id) => projectName(id || null)
					})
				)
			: []
	);
	const daily = $derived(ctx ? dailyFlow(tasks, { period, category: ctx.category }) : null);
	/** A share's parts in their colors, Other in a quiet one. */
	const colored = (parts: Share[], colors: Map<string, string>) =>
		parts.map((p) => ({
			key: p.key,
			name: p.name,
			value: p.value,
			color: (!p.other && colors.get(p.key)) || OTHER
		}));
	const shareTable = (name: string, parts: Share[]) => {
		const sum = parts.reduce((n, p) => n + p.value, 0);
		return {
			head: [name, 'Tasks', 'Share'],
			rows: parts.map((p) => [p.name, p.value, `${sum ? Math.round((p.value / sum) * 100) : 0}%`])
		};
	};

	// Words.
	const tasksOf = (n: number) => (n === 1 ? '1 task' : `${n} tasks`);
	const day = (d: string) => formatDate(d, { year: false });
	const weekLabels = (weeks: Week[]) => weeks.map((w) => day(w.from));
	const weekTitles = (weeks: Week[]) =>
		weeks.map((w) => (w.from === w.to ? formatDate(w.from) : `Week of ${formatDate(w.from)}`));
	const range = $derived(
		period.from === period.to
			? formatDate(period.from)
			: `${formatDate(period.from)} to ${formatDate(period.to)}`
	);
	const projectName = (id: string | null) =>
		id ? (replica?.project(id)?.name ?? 'A project') : 'Inbox';
	const hours = (minutes: number) => {
		const h = minutes / 60;
		return `${Number.isInteger(h) ? h : h.toFixed(1)} h`;
	};
	const dueName = (b: DueBucket) =>
		b.kind === 'overdue'
			? { short: 'Overdue', long: 'Overdue' }
			: b.kind === 'this'
				? { short: 'This week', long: 'Due this week' }
				: b.kind === 'next'
					? { short: 'Next week', long: 'Due next week' }
					: b.kind === 'later'
						? { short: 'Later', long: `Due from ${formatDate(b.from!)} on` }
						: { short: day(b.from!), long: `Due the week of ${formatDate(b.from!)}` };
	const lateWords = (n: number) => (n === 1 ? '1 day late' : `${n} days late`);

	// The tables: each chart's, as Show as table and the CSV have them.
	const burnupTable = $derived({
		head: ['Week of', 'All tasks', 'Done'],
		rows: (status?.weeks ?? []).map((w, i) => [day(w.from), status!.scope[i], status!.doneBy[i]])
	});
	const statusTable = $derived({
		head: ['Status', 'Open tasks'],
		rows: (status?.byStatus ?? []).map((s) => [s.label, s.count])
	});
	const loadTable = $derived({
		head: ['Person', 'Estimated hours', 'Open tasks', 'Overdue', 'No estimate'],
		rows: (workload?.people ?? []).map((p) => [
			p.userId ? personName(p.userId) : 'Unassigned',
			Math.round((p.minutes / 60) * 10) / 10,
			p.tasks,
			p.overdue,
			p.unestimated
		])
	});
	const dueTable = $derived({
		head: ['Due', 'Open tasks'],
		rows: (workload?.due ?? []).map((b) => [dueName(b).long, b.count])
	});
	const flowTable = $derived({
		head: ['Week of', 'Made', 'Done'],
		rows: (flow?.weeks ?? []).map((w, i) => [day(w.from), flow!.made[i], flow!.done[i]])
	});
	const cycleTable = $derived({
		head: ['Took', 'Tasks'],
		rows: (flow?.buckets ?? []).map((b, i) => [bucketNames(i).long, b.count])
	});
	const statusShareTable = $derived(shareTable('Status', statusParts));
	const priorityTable = $derived(shareTable('Priority', priorityParts));
	const latenessTable = $derived({
		head: ['How late', 'Open tasks'],
		rows: LATENESS.map((b, i) => [b.long, lateness[i] ?? 0])
	});
	const labelTable = $derived({
		head: ['Label', 'Open tasks'],
		rows: labelBars.map((b) => [b.name, b.value])
	});
	const projectTable = $derived({
		head: ['Project', 'Open tasks'],
		rows: projectBars.map((b) => [b.name, b.value])
	});
	const dailyTable = $derived({
		head: ['Day', 'To do', 'In progress', 'Done'],
		rows: (daily?.days ?? []).map((d, i) => [
			day(d),
			daily!.todo[i],
			daily!.inProgress[i],
			daily!.done[i]
		])
	});
	const dotsTable = $derived({
		head: ['Task', 'Done on', 'Days from start to done'],
		rows: (flow?.dots ?? []).map((d) => [d.task.title, day(d.day), Math.round(d.days * 10) / 10])
	});

	/** The tab's tables for the CSV: dates as YYYY-MM-DD, times in plain numbers. */
	function csvTables(): CsvTable[] {
		if (tab === 'status' && status) {
			return [
				{
					title: 'Now',
					head: ['Open', 'Done', 'All tasks', 'Overdue', 'Due this week or next'],
					rows: [[status.open, status.done, status.total, status.overdue, status.dueSoon]]
				},
				{
					title: 'Burnup',
					head: ['Week of', 'All tasks', 'Done'],
					rows: status.weeks.map((w, i) => [w.from, status.scope[i], status.doneBy[i]])
				},
				{ title: 'Open work by status', ...statusTable },
				{ title: 'Tasks by status', ...statusShareTable },
				{ title: 'Open work by priority', ...priorityTable },
				{ title: 'How overdue', ...latenessTable },
				{ title: 'Open work by label', ...labelTable },
				...(projectId ? [] : [{ title: 'Open work by project', ...projectTable }]),
				{
					title: 'Needs attention',
					head: [
						'Task',
						'Project',
						'Assigned to',
						'Due',
						'Days late',
						'Waiting on',
						'Minutes over the estimate'
					],
					rows: status.attention.map((a) => [
						a.task.title,
						projectName(a.task.projectId),
						assignedTo(a.task),
						a.task.dueDate ?? '',
						a.late ?? '',
						a.waiting || '',
						a.over ?? ''
					])
				}
			];
		}
		if (tab === 'workload' && workload) {
			return [
				{ title: 'Open work by person', ...loadTable },
				{
					title: 'Open tasks by the week they are due',
					head: ['Due', 'Week of', 'Open tasks'],
					rows: [
						...workload.due.map((b) => [dueName(b).long, b.from ?? '', b.count]),
						['No due date', '', workload.undated]
					]
				}
			];
		}
		if (flow) {
			return [
				{
					title: 'Summary',
					head: [
						'Done in the period',
						'Done a week, on average',
						'Median days from start to done',
						'8 in 10 done within, days',
						'Done without starting'
					],
					rows: [
						[
							flow.doneTotal,
							Math.round(flow.perWeek * 10) / 10,
							flow.median === null ? '' : Math.round(flow.median * 10) / 10,
							flow.p80 === null ? '' : Math.round(flow.p80 * 10) / 10,
							flow.unstarted
						]
					]
				},
				{
					title: 'Made and done each week',
					head: ['Week of', 'Made', 'Done'],
					rows: flow.weeks.map((w, i) => [w.from, flow.made[i], flow.done[i]])
				},
				{ title: 'How long tasks took', ...cycleTable },
				{
					title: 'Cumulative flow',
					head: ['Day', 'To do', 'In progress', 'Done'],
					rows: (daily?.days ?? []).map((d, i) => [
						d,
						daily!.todo[i],
						daily!.inProgress[i],
						daily!.done[i]
					])
				},
				{
					title: 'Cycle time, task by task',
					head: ['Task', 'Done on', 'Days from start to done'],
					rows: flow.dots.map((d) => [d.task.title, d.day, Math.round(d.days * 10) / 10])
				}
			];
		}
		return [];
	}

	function download() {
		const label = TABS.find((t) => t.id === tab)!.label;
		const of = projectId ? `${projectName(projectId)}, ` : '';
		const name =
			tab === 'workload'
				? `Reports, ${of}${label}, ${today}.csv`
				: `Reports, ${of}${label}, ${period.from} to ${period.to}.csv`;
		const blob = new Blob([tablesCsv(csvTables())], { type: 'text/csv;charset=utf-8' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = name;
		a.click();
		setTimeout(() => URL.revokeObjectURL(url), 1000);
	}

	/** Who a task is for, where it is and when it is due, for the list. */
	const metaOf = (t: Task) =>
		[assignedTo(t), projectName(t.projectId), t.dueDate ? `due ${day(t.dueDate)}` : null]
			.filter(Boolean)
			.join(' · ');

	function assignedTo(t: Task): string {
		const who = replica?.assignees(t.id) ?? [];
		return who.length ? who.map(personName).join(', ') : 'Unassigned';
	}

	// The controls.
	function showTab(id: ReportTab) {
		src.change({ tab: id });
		if (!linked) return;
		const u = new URL(page.url.href);
		u.searchParams.set('tab', id);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- same page, new query
		void goto(u, { reset: false, replace: true });
	}
	function tabKeys(e: KeyboardEvent) {
		const i = REPORT_TABS.indexOf(tab);
		const to =
			e.key === 'ArrowRight'
				? REPORT_TABS[(i + 1) % REPORT_TABS.length]
				: e.key === 'ArrowLeft'
					? REPORT_TABS[(i + REPORT_TABS.length - 1) % REPORT_TABS.length]
					: e.key === 'Home'
						? REPORT_TABS[0]
						: e.key === 'End'
							? REPORT_TABS.at(-1)!
							: null;
		if (!to) return;
		e.preventDefault();
		showTab(to);
		requestAnimationFrame(() => document.getElementById(`${uid}-tab-${to}`)?.focus());
	}
	async function setProject(id: string) {
		const project = id || null;
		src.change({ project });
		if (linked) {
			const u = new URL(page.url.href);
			if (project) u.searchParams.set('project', project);
			else u.searchParams.delete('project');
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- same page, new query
			await goto(u, { reset: false, replace: true });
		}
		await tick();
		announce(
			`${project ? projectName(project) : 'All projects'}: ${tasksOf(tasks.length)} in the report`
		);
	}
	function setPeriod(kind: PeriodKind) {
		if (kind === 'range' && !setup.from) {
			const now = periodOf(setup.period, today, first, setup);
			src.change({ period: kind, from: now.from, to: now.to });
		} else src.change({ period: kind });
	}
	let filtering = $state(false);
	let filterButton = $state<HTMLButtonElement>();
	async function setFilter(edit: ViewEdit<ViewFilter, Partial<ViewFilter>>) {
		src.editFilter(edit);
		await tick();
		announce(`${tasksOf(tasks.length)} in the report`);
	}
	function clearFilter() {
		src.change({ filter: {} });
		announce(`Filter cleared, ${tasksOf(tasks.length)} in the report`);
	}

	// Save as view: this page's setup as a report view, personal until shared.
	const canSave = $derived(linked && saveable);
	async function saveAsView() {
		const name = projectId ? `${projectName(projectId)} report` : 'All projects report';
		// A plain copy: what is kept on the device can't hold the page's live state.
		const now = $state.snapshot(setup);
		const id = await createView(ws, {
			name,
			type: 'report',
			projectId,
			ownerId: me,
			filter: now.filter,
			display: { report: reportDisplay({ ...now, tab, project: null }) }
		});
		toasts.show(`Saved "${name}" as a view. Only you see it until you share it.`);
		await goto(resolve('/(app)/views/[id]', { id }));
	}

	const attention = new Growing(() => status?.attention.length ?? 0);
	const uid = $props.id();
</script>

<div class="reports" class:only>
	{#if !only}
		<div class="tabs" role="tablist" aria-label="Reports">
			{#each TABS as t (t.id)}
				<button
					id={`${uid}-tab-${t.id}`}
					type="button"
					role="tab"
					aria-selected={tab === t.id}
					aria-controls={`${uid}-panel`}
					tabindex={tab === t.id ? 0 : -1}
					onclick={() => showTab(t.id)}
					onkeydown={tabKeys}>{t.label}</button
				>
			{/each}
		</div>
		<div class="controls">
			{#if !fixedProject}
				<label class="pick"
					>Project <select
						value={projectId ?? ''}
						onchange={(e) => setProject(e.currentTarget.value)}
					>
						<option value="">All projects</option>
						{#each projects as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
					</select></label
				>
			{/if}
			{#if tab !== 'workload'}
				<label class="pick"
					>Period <select
						value={setup.period}
						onchange={(e) => setPeriod(e.currentTarget.value as PeriodKind)}
					>
						{#each PERIODS as p (p.kind)}<option value={p.kind}>{p.label}</option>{/each}
					</select></label
				>
				{#if setup.period === 'range'}
					<label class="pick"
						>From <input
							type="date"
							value={period.from}
							min={EARLIEST_DAY}
							max={today}
							onchange={(e) =>
								e.currentTarget.value >= EARLIEST_DAY &&
								src.change({ from: e.currentTarget.value })}
						/></label
					>
					<label class="pick"
						>To <input
							type="date"
							value={period.to}
							min={EARLIEST_DAY}
							max={today}
							onchange={(e) =>
								e.currentTarget.value >= EARLIEST_DAY && src.change({ to: e.currentTarget.value })}
						/></label
					>
				{/if}
			{:else}
				<span class="now">Open work as it stands today</span>
			{/if}
			<button
				class="tool"
				type="button"
				title="Choose which tasks the reports count"
				aria-expanded={filtering}
				bind:this={filterButton}
				onclick={() => (filtering = !filtering)}
			>
				<ListFilter size={15} />Filter
				{#if filterCount(setup.filter)}<span class="badge tabular">{filterCount(setup.filter)}</span
					>{/if}
			</button>
			{#if canSave}
				<button
					class="tool"
					type="button"
					title="Keep this report, as it is set up, as a view"
					onclick={() => void saveAsView()}><BookmarkPlus size={15} />Save as view</button
				>
			{/if}
			<span class="spacer"></span>
			<a class="tool" href={resolve('/(app)/time')} title="Time logged by person, project or label"
				><Clock size={15} />Time</a
			>
			<button
				class="tool"
				type="button"
				title="The tables behind every chart on this tab, as CSV"
				disabled={!tasks.length}
				onclick={download}><Download size={15} />Download CSV</button
			>
			<HelpLink topic="reports" label="How reports count" />
		</div>
		{#if filtering}
			<!-- Loaded when first opened, as on a saved view's page. -->
			{#await import('../views/FilterPanel.svelte') then { default: FilterPanel }}
				<FilterPanel
					{ws}
					filter={setup.filter}
					acrossProjects
					projectId={null}
					hide={['showDone']}
					onchange={setFilter}
					onclose={() => {
						filtering = false;
						filterButton?.focus();
					}}
				/>
			{/await}
		{/if}
	{/if}

	<div
		class="panel"
		id={`${uid}-panel`}
		role={only ? undefined : 'tabpanel'}
		aria-labelledby={only ? undefined : `${uid}-tab-${tab}`}
	>
		{#if !all.length}
			<div class="empty">
				<h2>Nothing to report yet</h2>
				<p>Reports count the tasks in this workspace. Once it has some, they show here.</p>
			</div>
		{:else if !tasks.length}
			<div class="empty">
				<h2>{filtered ? 'No tasks match this filter' : 'No tasks to count'}</h2>
				<p>
					{filtered
						? 'Change the filter, or clear it to count every task you can see.'
						: 'Tasks in archived projects and canceled tasks are left out of reports.'}
				</p>
				{#if filtered}<Button size="sm" onclick={clearFilter}>Clear the filter</Button>{/if}
			</div>
		{:else if tab === 'status' && status}
			{#if shows('status-numbers')}
				<div class="tiles">
					<div class="tile">
						<span class="n tabular">{status.open}</span><span class="l">Open</span>
					</div>
					<div class="tile">
						<span class="n tabular">{status.done}</span><span class="l">Done of {status.total}</span
						>
					</div>
					<div class="tile" class:alert={status.overdue > 0}>
						<span class="n tabular"
							>{#if status.overdue}<CircleAlert
									size={18}
									aria-hidden="true"
								/>{/if}{status.overdue}</span
						><span class="l">Overdue</span>
					</div>
					<div class="tile">
						<span class="n tabular">{status.dueSoon}</span><span class="l"
							>Due this week or next</span
						>
					</div>
				</div>
			{/if}
			{#if shows('burnup') || shows('where') || shows('attention')}
				<div class="grid">
					{#if shows('burnup')}
						<ChartCard
							id={`${uid}-burnup`}
							title="Burnup"
							sub={`all tasks and those done, by week, ${range}`}
							legend={[
								{ name: 'All tasks', color: SERIES[0] },
								{ name: 'Done', color: SERIES[1] }
							]}
							table={burnupTable}
						>
							<LineChart
								label="Burnup: all tasks and those done, by week"
								labels={weekLabels(status.weeks)}
								titles={weekTitles(status.weeks)}
								series={[
									{ name: 'All tasks', short: 'in all', color: SERIES[0], values: status.scope },
									{ name: 'Done', short: 'done', color: SERIES[1], values: status.doneBy }
								]}
								unit={tasksOf}
							/>
						</ChartCard>
					{/if}
					{#if shows('where') || shows('attention')}
						<div class="stack">
							{#if shows('where')}
								<ChartCard
									id={`${uid}-status`}
									title="Where the open work is"
									sub="by status"
									table={statusTable}
								>
									{#if status.open}
										<StackedBar
											label="Open tasks by status"
											parts={status.byStatus.map((s, i) => ({
												key: s.category,
												name: s.label,
												value: s.count,
												color: SERIES[i]
											}))}
											unit={(n) => (n === 1 ? '1 open task' : `${n} open tasks`)}
										/>
									{:else}
										<p class="quiet">Nothing is open: every task here is done.</p>
									{/if}
								</ChartCard>
							{/if}
							{#if shows('attention')}
								<section class="attention" aria-labelledby={`${uid}-att`}>
									<h2 id={`${uid}-att`}>Needs attention</h2>
									{#if status.attention.length}
										<ul>
											{#each status.attention.slice(0, attention.limit) as a (a.task.id)}
												<li>
													<button
														type="button"
														class="item"
														title={`Open ${a.task.title}`}
														onclick={() => openTask(a.task.id)}
													>
														<span class="t">{a.task.title}</span>
														<span class="pills">
															{#if a.late !== null}<span class="pill late"
																	><CircleAlert size={12} aria-hidden="true" />{lateWords(
																		a.late
																	)}</span
																>{/if}
															{#if a.waiting}<span class="pill"
																	><Hourglass size={12} aria-hidden="true" />Waiting on {a.waiting}</span
																>{/if}
															{#if a.over !== null}<span class="pill"
																	><Timer size={12} aria-hidden="true" />{formatDuration(a.over)} over</span
																>{/if}
														</span>
														<span class="m">{metaOf(a.task)}</span>
													</button>
												</li>
											{/each}
										</ul>
										<div use:attention.more></div>
									{:else}
										<p class="quiet">
											Nothing is overdue, waiting on other tasks or past its estimate.
										</p>
									{/if}
								</section>
							{/if}
						</div>
					{/if}
				</div>
			{/if}
			{#if shows('by-status') || shows('by-priority') || shows('overdue') || shows('by-label') || shows('by-project')}
				<div class="grid">
					{#if shows('by-status')}
						<ChartCard
							id={`${uid}-shares`}
							title="Tasks by status"
							sub={projectId
								? `${projectName(projectId)}'s statuses, done ones too`
								: 'by kind of status, done ones too'}
							table={statusShareTable}
						>
							<StackedBar
								label="All tasks by status"
								parts={colored(statusParts, statusColors)}
								unit={tasksOf}
								what="Tasks"
								of="Of all"
							/>
						</ChartCard>
					{/if}
					{#if shows('by-priority')}
						<ChartCard
							id={`${uid}-priority`}
							title="Open work by priority"
							sub="open tasks"
							table={priorityTable}
						>
							{#if status.open}
								<StackedBar
									label="Open tasks by priority"
									parts={colored(priorityParts, priorityColors)}
									unit={(n) => (n === 1 ? '1 open task' : `${n} open tasks`)}
								/>
							{:else}
								<p class="quiet">Nothing is open: every task here is done.</p>
							{/if}
						</ChartCard>
					{/if}
					{#if shows('overdue')}
						<ChartCard
							id={`${uid}-late`}
							title="How overdue"
							sub="open tasks past their due date, by how late"
							table={latenessTable}
						>
							{#if status.overdue}
								<ColumnChart
									label="Overdue tasks by how late they are"
									labels={LATENESS.map((b) => b.short)}
									titles={LATENESS.map((b) => b.long)}
									values={lateness}
									name="Overdue tasks"
									color="--danger"
									unit={tasksOf}
								/>
							{:else}
								<p class="quiet">Nothing is overdue.</p>
							{/if}
						</ChartCard>
					{/if}
					{#if shows('by-label')}
						<ChartCard
							id={`${uid}-labels`}
							title="Open work by label"
							sub="open tasks, the 8 most; a task can have several labels"
							table={labelTable}
						>
							{#if labelBars.length}
								<BarChart
									label="Open tasks by label"
									color={SERIES[0]}
									bars={labelBars.map((b) => ({
										key: b.key,
										name: b.name,
										value: b.value,
										label: String(b.value),
										tip: [{ color: SERIES[0], label: 'Open tasks', value: String(b.value) }]
									}))}
								/>
							{:else}
								<p class="quiet">No open task here has a label.</p>
							{/if}
						</ChartCard>
					{/if}
					{#if shows('by-project')}
						{#if !projectId}
							<ChartCard
								id={`${uid}-projects`}
								title="Open work by project"
								sub="open tasks, the 8 most"
								table={projectTable}
							>
								{#if projectBars.length}
									<BarChart
										label="Open tasks by project"
										color={SERIES[0]}
										bars={projectBars.map((b) => ({
											key: b.key || 'inbox',
											name: b.name,
											value: b.value,
											label: String(b.value),
											tip: [{ color: SERIES[0], label: 'Open tasks', value: String(b.value) }]
										}))}
									/>
								{:else}
									<p class="quiet">Nothing is open: every task here is done.</p>
								{/if}
							</ChartCard>
						{/if}
					{/if}
				</div>
			{/if}
		{:else if tab === 'workload' && workload}
			{#if !workload.open}
				<div class="empty">
					<h2>No open work</h2>
					<p>Every task this report counts is done.</p>
				</div>
			{:else}
				{#if shows('by-person') || shows('due-weeks')}
					<div class="grid">
						{#if shows('by-person')}
							<ChartCard
								id={`${uid}-load`}
								title="Open work by person"
								sub="estimated hours, with the number of tasks"
								table={loadTable}
							>
								<BarChart
									label="Estimated hours of open work by person"
									color={SERIES[0]}
									bars={workload.people.map((p) => {
										const name = p.userId ? personName(p.userId) : 'Unassigned';
										return {
											key: p.userId ?? '',
											name,
											value: p.minutes,
											label: `${hours(p.minutes)}, ${tasksOf(p.tasks)}`,
											muted: !p.userId,
											tip: [
												{
													color: p.userId ? SERIES[0] : '--border-strong',
													label: 'Estimated',
													value: hours(p.minutes)
												},
												{ label: 'Open tasks', value: String(p.tasks) },
												{ label: 'Overdue', value: String(p.overdue) },
												{ label: 'No estimate', value: String(p.unestimated) }
											]
										};
									})}
								/>
								{#snippet note()}
									{workload.unestimated
										? `Tasks with no estimate count as tasks but add no hours: ${workload.unestimated} of them here.`
										: 'Every open task here has an estimate.'}
									A task with several assignees counts for each.
								{/snippet}
							</ChartCard>
						{/if}
						{#if shows('due-weeks')}
							<ChartCard
								id={`${uid}-due`}
								title="Due by week"
								sub="open tasks, everyone"
								table={dueTable}
							>
								<ColumnChart
									label="Open tasks by the week they are due"
									labels={workload.due.map((b) => dueName(b).short)}
									titles={workload.due.map((b) => dueName(b).long)}
									values={workload.due.map((b) => b.count)}
									name="Open tasks"
									color={SERIES[0]}
									colorOf={(i) => (i === 0 ? '--danger' : SERIES[0])}
									unit={tasksOf}
								/>
								{#snippet note()}
									Overdue is the first column.
									{workload.undated === 1
										? 'The one open task with no due date is not counted here.'
										: workload.undated
											? `The ${workload.undated} open tasks with no due date are not counted here.`
											: ''}
								{/snippet}
							</ChartCard>
						{/if}
					</div>
				{/if}
			{/if}
		{:else if tab === 'flow' && flow}
			{#if shows('flow-numbers')}
				<div class="tiles">
					<div class="tile">
						<span class="n tabular">{Math.round(flow.perWeek * 10) / 10}</span><span class="l"
							>Done a week, on average</span
						>
					</div>
					<div class="tile">
						<span class="n">{flow.median === null ? 'None yet' : sayDays(flow.median)}</span><span
							class="l">Typical time from start to done (median)</span
						>
					</div>
					<div class="tile">
						<span class="n">{flow.p80 === null ? 'None yet' : sayDays(flow.p80)}</span><span
							class="l">8 in 10 finish within</span
						>
					</div>
				</div>
			{/if}
			{#if shows('made-done') || shows('cycle') || shows('cfd') || shows('dots')}
				<div class="grid">
					{#if shows('made-done')}
						<ChartCard
							id={`${uid}-flow`}
							title="Made and done each week"
							sub={range}
							legend={[
								{ name: 'Made', color: SERIES[0] },
								{ name: 'Done', color: SERIES[1] }
							]}
							table={flowTable}
						>
							<LineChart
								label="Tasks made and done each week"
								labels={weekLabels(flow.weeks)}
								titles={weekTitles(flow.weeks)}
								series={[
									{ name: 'Made', short: 'made', color: SERIES[0], values: flow.made },
									{ name: 'Done', short: 'done', color: SERIES[1], values: flow.done }
								]}
								unit={tasksOf}
							/>
						</ChartCard>
					{/if}
					{#if shows('cycle')}
						<ChartCard
							id={`${uid}-cycle`}
							title="How long tasks took"
							sub={`from first in progress to done, ${range}`}
							table={cycleTable}
						>
							{#if flow.cycle.length}
								<ColumnChart
									label="Tasks by how long they took from start to done"
									labels={flow.buckets.map((_, i) => bucketNames(i).short)}
									titles={flow.buckets.map((_, i) => bucketNames(i).long)}
									values={flow.buckets.map((b) => b.count)}
									name="Tasks"
									color={SERIES[0]}
									unit={tasksOf}
								/>
							{:else}
								<p class="quiet">
									{flow.doneTotal
										? 'None of the tasks done in this period went in progress first.'
										: 'Nothing was done in this period.'}
								</p>
							{/if}
							{#snippet note()}
								{flow.unstarted === 1
									? 'One task done without ever going in progress is left out.'
									: flow.unstarted
										? `${flow.unstarted} tasks done without ever going in progress are left out.`
										: ''}
							{/snippet}
						</ChartCard>
					{/if}
					{#if shows('cfd')}
						{#if daily}
							<ChartCard
								id={`${uid}-cfd`}
								title="Cumulative flow"
								sub={`tasks to do, in progress and done each day, ${range}`}
								legend={[
									{ name: 'To do', color: SERIES[0] },
									{ name: 'In progress', color: SERIES[1] },
									{ name: 'Done', color: SERIES[2] }
								]}
								table={dailyTable}
							>
								<StackedArea
									label="Tasks to do, in progress and done each day"
									labels={daily.days.map(day)}
									titles={daily.days.map((d) => formatDate(d, { weekday: 'short' }))}
									bands={[
										{ name: 'Done', color: SERIES[2], values: daily.done },
										{ name: 'In progress', color: SERIES[1], values: daily.inProgress },
										{ name: 'To do', color: SERIES[0], values: daily.todo }
									]}
									unit={tasksOf}
								/>
								{#snippet note()}
									A middle band that widens is work piling up in progress. A task counts as in
									progress from the first time it went there.
								{/snippet}
							</ChartCard>
						{/if}
					{/if}
					{#if shows('dots')}
						<ChartCard
							id={`${uid}-dots`}
							title="Cycle time, task by task"
							sub={`each task done, by when and how long it took, ${range}`}
							table={dotsTable}
						>
							{#if flow.dots.length}
								<Scatter
									label="Each task done in the period, by when it was done and how many days it took"
									color={SERIES[0]}
									start={day(period.from)}
									end={day(period.to)}
									tick={(v) => `${v}d`}
									dots={flow.dots.map((d) => ({
										key: d.task.id,
										x:
											daysBetween(period.from, d.day) /
											Math.max(1, daysBetween(period.from, period.to)),
										y: d.days,
										title: d.task.title,
										rows: [
											{ label: 'Done', value: formatDate(d.day) },
											{ color: SERIES[0], label: 'Took', value: sayDays(d.days) }
										]
									}))}
									marks={[
										...(flow.median === null
											? []
											: [
													{
														value: flow.median,
														label: `median ${sayDays(flow.median)}`,
														color: SERIES[2]
													}
												]),
										...(flow.p80 === null
											? []
											: [
													{
														value: flow.p80,
														label: `8 in 10 within ${sayDays(flow.p80)}`,
														color: SERIES[1]
													}
												])
									]}
									summary={`${tasksOf(flow.dots.length)} done in the period after starting${flow.median === null ? '' : `; the median took ${sayDays(flow.median)}`}${flow.p80 === null ? '' : `, and 8 in 10 were done within ${sayDays(flow.p80)}`}.`}
								/>
							{:else}
								<p class="quiet">
									{flow.doneTotal
										? 'None of the tasks done in this period went in progress first.'
										: 'Nothing was done in this period.'}
								</p>
							{/if}
						</ChartCard>
					{/if}
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	.reports {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		align-content: start;
	}
	.tabs {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
		padding: 0 var(--space-4);
		border-bottom: var(--border-width) solid var(--border);
	}
	.tabs button {
		min-height: max(var(--tap-min), 36px);
		margin-bottom: calc(-1 * var(--border-width));
		padding: 0 var(--space-2);
		border: none;
		border-bottom: 2px solid transparent;
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: 550;
		cursor: pointer;
	}
	.tabs button:hover {
		color: var(--text-primary);
	}
	.tabs button[aria-selected='true'] {
		border-bottom-color: var(--accent);
		color: var(--text-primary);
	}
	.tabs button:focus-visible {
		outline: none;
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.controls {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2) var(--space-3);
		padding: var(--space-2) var(--space-5);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-1);
		font-size: var(--text-sm);
	}
	.spacer {
		flex: 1;
	}
	.pick {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--text-secondary);
	}
	.pick select,
	.pick input {
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-sm);
		background: var(--input-bg);
		color: var(--input-text);
		font-size: var(--text-sm);
	}
	.now {
		color: var(--text-secondary);
	}
	.tool {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		text-decoration: none;
		cursor: pointer;
	}
	.tool:hover:not(:disabled),
	.tool[aria-expanded='true'] {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	.tool:disabled {
		opacity: 0.5;
		cursor: default;
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
	.panel {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-5);
		align-content: start;
		padding: var(--space-4) var(--space-5) var(--space-6);
	}
	.tiles {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		gap: var(--space-3);
	}
	.tile {
		display: grid;
		gap: 2px;
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.tile .n {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-xl);
		font-weight: 650;
		line-height: 1.2;
	}
	.tile .l {
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.tile.alert .n {
		color: var(--danger);
	}
	/* One chart in a page: as wide as the page, with no frame of its own. */
	.only .panel {
		padding: 0;
	}
	.only .grid {
		grid-template-columns: minmax(0, 1fr);
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr));
		gap: var(--space-5) var(--space-6);
		align-items: start;
	}
	.stack {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-5);
		min-width: 0;
	}
	.attention {
		display: grid;
		gap: var(--space-1);
	}
	.attention h2 {
		font-size: var(--text-md);
	}
	.attention ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.attention li + li {
		border-top: var(--border-width) solid var(--border);
	}
	.item {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: 2px var(--space-3);
		width: 100%;
		min-height: max(var(--tap-min), 40px);
		padding: var(--space-2) var(--space-1);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-primary);
		font-size: var(--text-sm);
		text-align: left;
		cursor: pointer;
	}
	.item:hover {
		background: var(--row-hover-bg);
	}
	.t {
		font-weight: 500;
		overflow-wrap: anywhere;
	}
	.m {
		grid-column: 1 / -1;
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.pills {
		display: inline-flex;
		flex-wrap: wrap;
		justify-content: end;
		gap: var(--space-1);
		align-self: center;
	}
	.pill {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-full);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 500;
		white-space: nowrap;
	}
	.pill.late {
		border-color: var(--danger);
		color: var(--danger);
	}
	.quiet {
		margin: 0;
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.empty {
		display: grid;
		justify-items: center;
		gap: var(--space-2);
		padding: var(--space-7) var(--space-4);
		text-align: center;
	}
	.empty h2 {
		font-size: var(--text-lg);
	}
	.empty p {
		max-width: 48ch;
		margin: 0;
		color: var(--text-secondary);
	}
	@media (max-width: 720px) {
		.controls,
		.panel {
			padding-inline: var(--space-3);
		}
		.tabs {
			padding-inline: var(--space-2);
		}
	}
</style>
