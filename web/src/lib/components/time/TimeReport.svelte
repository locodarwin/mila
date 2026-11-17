<script lang="ts">
	// The Time page: what was logged in a week, a
	// month, weeks, a quarter or a range, by person, project or label, a column
	// a day or a week, with a chart of it by week for a longer period, and
	// estimates against what finished tasks took. A guest sees only their own
	// time. Export CSV writes the table as shown, or a line per entry.
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import Download from '@lucide/svelte/icons/download';
	import Plus from '@lucide/svelte/icons/plus';
	import { tick } from 'svelte';
	import Button from '../Button.svelte';
	import StackedColumns from '../reports/StackedColumns.svelte';
	import { formatDate, weekStart } from '../../dates';
	import { formatDuration, sayDuration } from '../../duration';
	import { anchored } from '../../popover';
	import LogProjectTime from './LogProjectTime.svelte';
	import { session } from '../../session.svelte';
	import { localToday } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import {
		buildReport,
		estimatesAgainst,
		periodOf,
		reportCsv,
		tableCsv,
		underWay,
		weeklySeries,
		type PeriodKind,
		type ReportSource,
		type RowsBy
	} from '../../timeReport';

	let { ws }: { ws: string } = $props();
	const replica = $derived(engine.replica(ws));
	const guest = $derived(session.workspaces.find((w) => w.id === ws)?.role === 'guest');
	const me = $derived(session.user?.id ?? '');
	const today = localToday();
	const first = weekStart();

	const PERIODS: { kind: PeriodKind; label: string }[] = [
		{ kind: 'week', label: 'This week' },
		{ kind: 'last-week', label: 'Last week' },
		{ kind: 'month', label: 'This month' },
		{ kind: 'last-month', label: 'Last month' },
		{ kind: 'weeks8', label: 'Last 8 weeks' },
		{ kind: 'weeks12', label: 'Last 12 weeks' },
		{ kind: 'quarter', label: 'This quarter' },
		{ kind: 'last-quarter', label: 'Last quarter' },
		{ kind: 'range', label: 'Between two days' }
	];
	// The parts of the chart by week, as Reports colors its own: the top five
	// in the series colors, the rest together in a quiet one.
	const SERIES = [
		'--chart-series-1',
		'--chart-series-2',
		'--chart-series-3',
		'--chart-series-4',
		'--chart-series-5',
		'--chart-series-6'
	];
	const OTHERS = '--border-strong';
	const ROWS: { by: RowsBy; label: string; one: string }[] = [
		{ by: 'person', label: 'People', one: 'Person' },
		{ by: 'project', label: 'Projects', one: 'Project' },
		{ by: 'label', label: 'Labels', one: 'Label' }
	];

	let kind = $state<PeriodKind>('week');
	let rows = $state<RowsBy>('person');
	let project = $state('');
	const thisMonth = periodOf('month', today, first);
	let from = $state(thisMonth.from);
	let to = $state(today);
	const period = $derived(
		kind === 'range'
			? from <= to
				? { from, to }
				: { from: to, to: from }
			: periodOf(kind, today, first)
	);

	const projects = $derived(replica?.projects() ?? []);
	const source = $derived.by((): ReportSource => {
		const names = new Map(projects.map((p) => [p.id, p.name]));
		return {
			entries: replica?.allTimeEntries() ?? [],
			task: (id) => replica?.task(id),
			// Archived projects count, as their tasks do; those in the trash do not.
			projectLive: (id) => !!replica?.project(id),
			projectName: (id) =>
				id ? (names.get(id) ?? replica?.project(id)?.name ?? 'A project') : 'Inbox',
			labels: (id) => replica?.taskLabels(id) ?? [],
			personName: (id) => session.nameOf(ws, id)
		};
	});
	const options = $derived({
		period,
		rows,
		project: project || null,
		onlyUser: guest ? me : null,
		weekStart: first
	});
	const report = $derived(buildReport(source, options));
	const localDay = (iso: string) => {
		const d = new Date(iso);
		return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	};
	const against = $derived(estimatesAgainst(replica?.allTasks() ?? [], source, options, localDay));
	const estimated = $derived(against.reduce((s, a) => s + a.estimate, 0));
	const took = $derived(against.reduce((s, a) => s + a.logged, 0));
	const top = $derived(report.rows[0]);
	const widest = $derived(Math.max(1, ...report.rows.map((r) => r.total)));
	const scale = $derived(Math.max(1, ...against.flatMap((a) => [a.estimate, a.logged])));
	const rowsLabel = $derived(ROWS.find((r) => r.by === rows)!);
	const range = $derived(
		period.from === period.to
			? formatDate(period.from)
			: `${formatDate(period.from)} to ${formatDate(period.to)}`
	);
	const heading = (c: { from: string; to: string }) =>
		c.from === c.to
			? formatDate(c.from, { weekday: 'short', year: false })
			: formatDate(c.from, { year: false });
	const percent = (a: { estimate: number; logged: number }) =>
		a.estimate ? Math.round((Math.abs(a.logged - a.estimate) / a.estimate) * 100) : 0;

	// The chart by week, for a period long enough to have a column a week.
	const weekly = $derived(
		report.columns.length > 1 && report.columns[0].from !== report.columns[0].to
	);
	const series = $derived(weeklySeries(report));
	const stacks = $derived(
		series.map((s, i) => ({
			name: s.name,
			color: s.other ? OTHERS : SERIES[i],
			values: s.values.map((m) => m / 60)
		}))
	);
	const partial = $derived.by(() => {
		const i = report.columns.findIndex((c) => underWay(c, today));
		return i < 0 ? null : i;
	});
	const hours = (h: number) => `${Math.round(h * 10) / 10}h`;
	const weekTitle = (c: { from: string; to: string }) =>
		`${formatDate(c.from, { year: false })} to ${formatDate(c.to, { year: false })}`;

	// Log time to a project itself, from here as from the project's Time tab.
	let logging = $state(false);
	async function closeLog() {
		logging = false;
		await tick();
		document.querySelector<HTMLElement>('[data-log-time]')?.focus();
	}

	let exportMenu = $state<HTMLElement>();
	function download(text: string, name: string) {
		exportMenu?.hidePopover();
		const blob = new Blob([text], { type: 'text/csv;charset=utf-8' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = name;
		a.click();
		setTimeout(() => URL.revokeObjectURL(url), 1000);
	}
	const exportTable = () =>
		download(
			tableCsv(report, rowsLabel.one),
			`Time by ${rowsLabel.one.toLowerCase()} ${period.from} to ${period.to}.csv`
		);
	const exportEntries = () =>
		download(reportCsv(report, source), `Time ${period.from} to ${period.to}.csv`);
</script>

<div class="report">
	<div class="bar">
		<label class="pick"
			>Period <select bind:value={kind}>
				{#each PERIODS as p (p.kind)}<option value={p.kind}>{p.label}</option>{/each}
			</select></label
		>
		{#if kind === 'range'}
			<label class="pick">From <input type="date" bind:value={from} max={today} /></label>
			<label class="pick">To <input type="date" bind:value={to} max={today} /></label>
		{/if}
		<label class="pick"
			>Rows <select bind:value={rows}>
				{#each ROWS as r (r.by)}<option value={r.by}>{r.label}</option>{/each}
			</select></label
		>
		<label class="pick"
			>Projects <select bind:value={project}>
				<option value="">All projects</option>
				<option value="inbox">Inbox</option>
				{#each projects as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
			</select></label
		>
		<span class="spacer"></span>
		<Button
			size="sm"
			disabled={!projects.length}
			title={projects.length
				? 'Add time spent on a project itself, not on one of its tasks'
				: 'Make a project first'}
			aria-expanded={logging}
			data-log-time
			onclick={() => (logging = !logging)}><Plus size={14} />Log time</Button
		>
		<Button size="sm" disabled={!report.entries.length} popovertarget="time-export"
			><Download size={14} />Export CSV<ChevronDown size={14} /></Button
		>
		<div id="time-export" popover class="menu" use:anchored bind:this={exportMenu}>
			<button type="button" class="option" onclick={exportTable}
				><span class="name">This table</span><span class="meta"
					>A row a {rowsLabel.one.toLowerCase()}, a column a {weekly ? 'week' : 'day'}, in hours</span
				></button
			>
			<button type="button" class="option" onclick={exportEntries}
				><span class="name">Every entry</span><span class="meta">A line a time entry</span></button
			>
		</div>
	</div>

	{#if logging}
		<LogProjectTime {ws} {projects} start={project} onclose={() => void closeLog()} />
	{/if}

	{#if guest}<p class="note">You see only your own time here.</p>{/if}

	<div class="tiles">
		<div class="tile">
			<span class="k">Logged</span>
			<span class="v tabular">{formatDuration(report.total)}</span>
			<span class="s">{range}</span>
		</div>
		<div class="tile">
			<span class="k">Most</span>
			<span class="v name">{top?.name ?? 'Nobody yet'}</span>
			<span class="s tabular">{top ? formatDuration(top.total) : ''}</span>
		</div>
		<div class="tile">
			<span class="k">Estimated, finished tasks</span>
			<span class="v tabular">{formatDuration(estimated)}</span>
			<span class="s"
				>{against.reduce((s, a) => s + a.tasks, 0)} with an estimate, finished in the period</span
			>
		</div>
		<div class="tile">
			<span class="k">They took</span>
			<span class="v tabular" class:over={took > estimated}>{formatDuration(took)}</span>
			<span class="s"
				>{estimated
					? took > estimated
						? `${formatDuration(took - estimated)} over`
						: `${formatDuration(estimated - took)} under`
					: 'Nothing to compare yet'}</span
			>
		</div>
	</div>

	{#if report.rows.length && weekly}
		<section class="weeks" aria-labelledby="weeks-h">
			<h2 id="weeks-h">By week</h2>
			<ul class="key" aria-hidden="true">
				{#each stacks as s (s.name)}<li>
						<i style:background={`var(${s.color})`}></i>{s.name}
					</li>{/each}
			</ul>
			<StackedColumns
				label={`Hours logged each week by ${rowsLabel.one.toLowerCase()}, ${range}`}
				labels={report.columns.map((c) => formatDate(c.from, { year: false }))}
				titles={report.columns.map(weekTitle)}
				{stacks}
				totals={report.totals.map((m) => m / 60)}
				{partial}
				unit={hours}
			/>
			{#if partial !== null}<p class="note">The week under way is lighter, and says so far.</p>{/if}
		</section>
	{/if}

	{#if report.rows.length}
		<!-- Focusable, so the keyboard can scroll it sideways on a narrow screen. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<div class="scroll" tabindex="0" role="region" aria-label="The report">
			<table>
				<caption class="visually-hidden"
					>Time logged by {rowsLabel.one.toLowerCase()}, {range}</caption
				>
				<thead>
					<tr>
						<th scope="col">{rowsLabel.one}</th>
						{#each report.columns as c (c.from)}<th scope="col" class="n">{heading(c)}</th>{/each}
						<th scope="col" class="n">Total</th>
					</tr>
				</thead>
				<tbody>
					{#each report.rows as r (r.key)}
						<tr>
							<th scope="row"
								><span
									class="share"
									style:width={`${Math.round((r.total / widest) * 48)}px`}
									aria-hidden="true"
								></span>{r.name}</th
							>
							{#each r.cells as m, i (i)}<td class="n tabular" class:none={!m}
									>{m ? formatDuration(m) : '–'}</td
								>{/each}
							<td class="n tabular strong" aria-label={sayDuration(r.total)}
								>{formatDuration(r.total)}</td
							>
						</tr>
					{/each}
				</tbody>
				<tfoot>
					<tr>
						<th scope="row">Total</th>
						{#each report.totals as m, i (i)}<td class="n tabular">{m ? formatDuration(m) : '–'}</td
							>{/each}
						<td class="n tabular strong">{formatDuration(report.total)}</td>
					</tr>
				</tfoot>
			</table>
		</div>
		{#if rows === 'label'}
			<p class="note">A task with two labels counts under both; the total counts its time once.</p>
		{/if}
	{:else}
		<p class="empty">No time logged in this period.</p>
	{/if}

	<section class="against" aria-labelledby="against-h">
		<h2 id="against-h">Estimates against what they took</h2>
		<p class="note">
			Tasks finished in the period that had an estimate, with all the time logged on them.
		</p>
		{#if against.length}
			<p class="legend" aria-hidden="true">
				<span><i class="e"></i>Estimated</span><span><i class="a"></i>Logged</span><span
					><i class="a over"></i>Logged, past the estimate</span
				>
			</p>
			<ul>
				{#each against as a (a.key)}
					<li>
						<span class="who">{a.name}</span>
						<span class="bars" aria-hidden="true">
							<span class="e" style:width={`${(a.estimate / scale) * 100}%`}></span>
							<span
								class="a"
								class:over={a.logged > a.estimate}
								style:width={`${(a.logged / scale) * 100}%`}
							></span>
						</span>
						<span class="num tabular"
							>{formatDuration(a.logged)} of {formatDuration(a.estimate)}
							<span class:over={a.logged > a.estimate}
								>{a.logged > a.estimate ? '+' : '−'}{percent(a)}%</span
							><span class="visually-hidden">
								{a.logged > a.estimate ? 'over' : 'under'}, {a.tasks}
								{a.tasks === 1 ? 'task' : 'tasks'}</span
							></span
						>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="empty">No task with an estimate was finished in this period.</p>
		{/if}
	</section>
</div>

<style>
	.report {
		display: grid;
		/* One column no wider than the page: the table scrolls in its own box. */
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-4);
		align-content: start;
		padding: var(--space-4) var(--space-5) var(--space-6);
	}
	.bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2) var(--space-3);
		font-size: var(--text-sm);
	}
	.spacer {
		flex: 1;
	}
	/* base.css lays out .menu .option; these are buttons. */
	.option {
		width: 100%;
		border: none;
		background: none;
		color: var(--text-primary);
		text-align: left;
		cursor: pointer;
	}
	.option .name {
		flex: 1;
	}
	.option .meta {
		color: var(--text-muted);
		font-size: var(--text-xs);
	}
	.weeks {
		display: grid;
		gap: var(--space-2);
	}
	.weeks h2 {
		font-size: var(--text-md);
	}
	.key {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1) var(--space-4);
		margin: 0;
		padding: 0;
		list-style: none;
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.key i {
		display: inline-block;
		width: 10px;
		height: 10px;
		margin-right: var(--space-1);
		border-radius: 3px;
		vertical-align: -1px;
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
		background: var(--surface-0);
		color: var(--text-primary);
	}
	.tiles {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
		gap: var(--space-3);
	}
	.tile {
		display: grid;
		gap: 2px;
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.k {
		color: var(--text-muted);
		font-size: var(--text-xs);
		font-weight: 550;
	}
	.v {
		font-size: var(--text-xl);
		font-weight: 650;
	}
	.v.name {
		overflow: hidden;
		font-size: var(--text-md);
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.s,
	.note,
	.empty {
		margin: 0;
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.s {
		font-size: var(--text-xs);
	}
	.over {
		color: var(--danger);
	}
	.scroll {
		overflow-x: auto;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
	}
	th,
	td {
		padding: var(--space-2) var(--space-3);
		border-bottom: var(--border-width) solid var(--border);
		text-align: left;
		white-space: nowrap;
	}
	thead th {
		background: var(--surface-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	tbody th {
		font-weight: 500;
	}
	.n {
		text-align: right;
	}
	.none {
		color: var(--text-muted);
	}
	.strong {
		font-weight: 650;
	}
	tfoot th,
	tfoot td {
		border-bottom: none;
		background: var(--surface-1);
		font-weight: 600;
	}
	.share {
		display: inline-block;
		height: 6px;
		margin-right: var(--space-2);
		border-radius: var(--radius-full);
		background: var(--accent);
		vertical-align: middle;
	}
	.against {
		display: grid;
		gap: var(--space-2);
	}
	.against h2 {
		font-size: var(--text-md);
	}
	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
		margin: 0;
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.legend i {
		display: inline-block;
		width: 10px;
		height: 10px;
		margin-right: var(--space-1);
		border-radius: 3px;
		vertical-align: -1px;
	}
	.against ul {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.against li {
		display: grid;
		grid-template-columns: minmax(100px, 180px) minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--space-3);
		font-size: var(--text-sm);
	}
	.who {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.bars {
		display: grid;
		gap: 3px;
	}
	.bars span {
		height: 8px;
		border-radius: var(--radius-full);
	}
	.e {
		background: var(--border-strong);
	}
	.a {
		background: var(--accent);
	}
	.a.over {
		background: var(--danger);
	}
	.num {
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	@media (max-width: 720px) {
		.report {
			padding: var(--space-3);
		}
		.against li {
			grid-template-columns: minmax(0, 1fr) auto;
		}
		.against .bars {
			grid-column: 1 / -1;
			grid-row: 2;
		}
	}
</style>
