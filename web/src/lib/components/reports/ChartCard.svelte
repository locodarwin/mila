<script lang="ts" module>
	/** A chart's numbers as a table: its headings and rows, as the table view and the CSV have them. */
	export interface ChartTable {
		head: string[];
		rows: (string | number)[][];
	}
</script>

<script lang="ts">
	// A chart with its heading, what it shows, a legend for two series or
	// more, and Show as table, which puts the same numbers in a table
	// (scrolled in its own box on a narrow screen) in place of the picture.
	import type { Snippet } from 'svelte';

	interface Props {
		id: string;
		title: string;
		/** What it counts, beside the title. */
		sub?: string;
		/** The series, for the legend: their names and color tokens. Only shown for two or more. */
		legend?: { name: string; color: string }[];
		table: ChartTable;
		/** Words under the chart or table. */
		note?: Snippet;
		children: Snippet;
	}

	let { id, title, sub, legend = [], table, note, children }: Props = $props();
	let asTable = $state(false);
</script>

<section class="card" aria-labelledby={`${id}-h`}>
	<div class="head">
		<h2 id={`${id}-h`}>{title}</h2>
		{#if sub}<span class="sub">{sub}</span>{/if}
		<button
			type="button"
			class="toggle"
			aria-pressed={asTable}
			title={asTable
				? `Show ${title.toLowerCase()} as a chart`
				: `Show the numbers behind ${title.toLowerCase()} as a table`}
			onclick={() => (asTable = !asTable)}>Show as table</button
		>
	</div>
	{#if asTable}
		<!-- Focusable, so the keyboard can scroll it sideways on a narrow screen. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<div class="scroll" tabindex="0" role="region" aria-label={`${title}, as a table`}>
			<table>
				<thead>
					<tr>
						{#each table.head as h, i (i)}<th scope="col" class:n={i > 0}>{h}</th>{/each}
					</tr>
				</thead>
				<tbody>
					{#each table.rows as r, i (i)}
						<tr>
							{#each r as c, j (j)}
								{#if j === 0}<th scope="row">{c}</th>
								{:else}<td class="n tabular">{c}</td>{/if}
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		{#if legend.length > 1}
			<ul class="legend" aria-hidden="true">
				{#each legend as l (l.name)}<li>
						<i style:background={`var(${l.color})`}></i>{l.name}
					</li>{/each}
			</ul>
		{/if}
		{@render children()}
	{/if}
	{#if note}<p class="note">{@render note()}</p>{/if}
</section>

<style>
	.card {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-2);
		align-content: start;
		min-width: 0;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-1) var(--space-2);
	}
	h2 {
		font-size: var(--text-md);
	}
	.sub {
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.toggle {
		min-height: max(var(--tap-min), 24px);
		margin-left: auto;
		padding: 0 var(--space-1);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--accent-text);
		font-size: var(--text-xs);
		text-decoration: underline;
		text-underline-offset: 2px;
		cursor: pointer;
	}
	.toggle[aria-pressed='true'] {
		font-weight: 600;
	}
	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1) var(--space-4);
		margin: 0;
		padding: 0;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		list-style: none;
	}
	.legend i {
		display: inline-block;
		width: 10px;
		height: 10px;
		margin-right: var(--space-1);
		border-radius: 2px;
		vertical-align: -1px;
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
		padding: var(--space-1) var(--space-3);
		border-bottom: var(--border-width) solid var(--border);
		text-align: left;
		white-space: nowrap;
	}
	tbody tr:last-child > * {
		border-bottom: none;
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
	.note {
		margin: 0;
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
</style>
