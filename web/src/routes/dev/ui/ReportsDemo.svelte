<script lang="ts">
	// The style guide's charts and the Reports page on the sample workspace,
	// loaded apart, as the app loads them only with Reports. The charts take
	// fixed numbers; the page counts the sample's Kitchen and Garden.
	import BarChart from '#lib/components/reports/BarChart.svelte';
	import ChartCard from '#lib/components/reports/ChartCard.svelte';
	import ColumnChart from '#lib/components/reports/ColumnChart.svelte';
	import LineChart from '#lib/components/reports/LineChart.svelte';
	import Reports from '#lib/components/reports/Reports.svelte';
	import StackedBar from '#lib/components/reports/StackedBar.svelte';
	import StackedColumns from '#lib/components/reports/StackedColumns.svelte';
	import { formatDate } from '#lib/dates.js';
	import { SAMPLE_ME, SAMPLE_WS } from './sample';

	const NAMES: Record<string, string> = {
		[SAMPLE_ME]: 'Alex Kim',
		'u-sam': 'Sam Ortega',
		'u-lee': 'Lee Park',
		'u-dana': 'Dana Whitfield'
	};
	const nameOf = (id: string) => NAMES[id] ?? 'Someone';

	const weeks = [
		'2026-09-27',
		'2026-10-04',
		'2026-10-11',
		'2026-10-18',
		'2026-10-25',
		'2026-11-01'
	];
	const labels = weeks.map((w) => formatDate(w, { year: false }));
	const titles = weeks.map((w) => `Week of ${formatDate(w)}`);
	const scope = [25, 33, 43, 48, 57, 64];
	const done = [11, 18, 27, 35, 42, 51];
	const tasks = (n: number) => (n === 1 ? '1 task' : `${n} tasks`);
	const due = [3, 4, 3, 1, 1, 0];
	const dueLabels = ['Overdue', 'This week', 'Next week', 'Nov 15', 'Nov 22', 'Later'];
	// Hours a week by person, as the Time page draws them; the last week under way.
	const logged = [
		{ name: 'Dana Whitfield', color: '--chart-series-1', values: [12, 14.5, 9, 13, 15, 4] },
		{ name: 'Sam Ortega', color: '--chart-series-2', values: [8, 9.5, 11, 7, 12, 3.5] },
		{ name: 'Lee Park', color: '--chart-series-3', values: [6, 4.5, 7, 9, 8, 1.5] },
		{ name: 'Others (2)', color: '--border-strong', values: [3, 2, 4.5, 2.5, 3, 0.75] }
	];
	const loggedTotals = weeks.map((_, i) => logged.reduce((n, s) => n + s.values[i], 0));
	const hoursOf = (h: number) => `${Math.round(h * 10) / 10}h`;
	const people = [
		{ name: 'Dana Whitfield', hours: 9, tasks: 4 },
		{ name: 'Sam Ortega', hours: 7, tasks: 3 },
		{ name: 'Lee Park', hours: 4.5, tasks: 2 },
		{ name: 'Unassigned', hours: 1, tasks: 1, muted: true }
	];
	const parts = [
		{ key: 'backlog', name: 'Backlog', value: 2, color: '--chart-series-1' },
		{ key: 'todo', name: 'To do', value: 7, color: '--chart-series-2' },
		{ key: 'in-progress', name: 'In progress', value: 4, color: '--chart-series-3' }
	];
</script>

<section>
	<h2>Charts</h2>
	<p class="note">
		Reports' charts, drawn by Mila in the theme's chart colors: lines with a crosshair, columns (one
		may take the danger color), columns in parts (the Time page's by week, the week under way
		lighter), bars and one bar in parts. Hover or tap a chart, or focus it and step through it from
		the keyboard; each has Show as table.
	</p>
	<div class="charts">
		<ChartCard
			id="demo-line"
			title="Burnup"
			sub="all tasks and those done, by week"
			legend={[
				{ name: 'All tasks', color: '--chart-series-1' },
				{ name: 'Done', color: '--chart-series-2' }
			]}
			table={{
				head: ['Week of', 'All tasks', 'Done'],
				rows: labels.map((l, i) => [l, scope[i], done[i]])
			}}
		>
			<LineChart
				label="Burnup"
				{labels}
				{titles}
				series={[
					{ name: 'All tasks', short: 'in all', color: '--chart-series-1', values: scope },
					{ name: 'Done', short: 'done', color: '--chart-series-2', values: done }
				]}
				unit={tasks}
			/>
		</ChartCard>
		<ChartCard
			id="demo-columns"
			title="Due by week"
			sub="open tasks"
			table={{ head: ['Due', 'Open tasks'], rows: dueLabels.map((l, i) => [l, due[i]]) }}
		>
			<ColumnChart
				label="Open tasks by the week they are due"
				labels={dueLabels}
				titles={dueLabels}
				values={due}
				name="Open tasks"
				color="--chart-series-1"
				colorOf={(i) => (i === 0 ? '--danger' : '--chart-series-1')}
				unit={tasks}
			/>
		</ChartCard>
		<ChartCard
			id="demo-stacked-columns"
			title="By week"
			sub="hours logged, by person"
			table={{
				head: ['Week', ...logged.map((s) => s.name), 'In all'],
				rows: weeks.map((w, i) => [
					formatDate(w, { year: false }),
					...logged.map((s) => s.values[i]),
					loggedTotals[i]
				])
			}}
		>
			<StackedColumns
				label="Hours logged each week by person"
				{labels}
				{titles}
				stacks={logged}
				totals={loggedTotals}
				partial={weeks.length - 1}
				unit={hoursOf}
			/>
		</ChartCard>
		<ChartCard
			id="demo-bars"
			title="Open work by person"
			sub="estimated hours"
			table={{
				head: ['Person', 'Estimated hours', 'Open tasks'],
				rows: people.map((p) => [p.name, p.hours, p.tasks])
			}}
		>
			<BarChart
				label="Estimated hours of open work by person"
				color="--chart-series-1"
				bars={people.map((p) => ({
					key: p.name,
					name: p.name,
					value: p.hours,
					label: `${p.hours} h, ${tasks(p.tasks)}`,
					muted: p.muted,
					tip: [
						{
							color: p.muted ? '--border-strong' : '--chart-series-1',
							label: 'Estimated',
							value: `${p.hours} h`
						},
						{ label: 'Open tasks', value: String(p.tasks) }
					]
				}))}
			/>
		</ChartCard>
		<ChartCard
			id="demo-stacked"
			title="Where the open work is"
			sub="by status"
			table={{ head: ['Status', 'Open tasks'], rows: parts.map((p) => [p.name, p.value]) }}
		>
			<StackedBar
				label="Open tasks by status"
				{parts}
				unit={(n) => (n === 1 ? '1 open task' : `${n} open tasks`)}
			/>
		</ChartCard>
	</div>
</section>

<section>
	<h2>Reports</h2>
	<p class="note">
		The Reports page on the sample workspace: Project status, Workload, and Throughput and cycle
		time, for one project or all, each narrowed by a filter and a period, with Download CSV.
	</p>
	<div class="page-demo">
		<Reports ws={SAMPLE_WS} {nameOf} embedded />
	</div>
</section>

<style>
	/* As the style guide's own sections. */
	section {
		display: grid;
		gap: var(--space-4);
		padding: var(--space-6) 0;
		border-bottom: var(--border-width) solid var(--border);
	}
	section > h2 {
		font-size: var(--text-xl);
	}
	.note {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.charts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr));
		gap: var(--space-5) var(--space-6);
	}
	.page-demo {
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
</style>
