<script lang="ts">
	// One chart of a shared report view in a wiki page or a task's notes:
	// worked out on this device from its copy, as the Reports page does, so it
	// works offline, stays current, and counts only the tasks this reader can
	// see. It follows the view: a change to the view's period or filter
	// changes every page showing it. Drawn by the editor's node for it
	// (nodeviews.svelte.ts); the Reports component loads only when one shows.
	import { resolve } from '$app/paths';
	import { viewReport, type ReportSource } from '../../reportSetup.svelte';
	import { chartOf, type ReportChart } from '../../reportKinds';
	import { engine } from '../../sync/engine.svelte';

	interface Props {
		ws: string;
		/** The report view's ID. */
		id: string;
		/** The chart's key, which a newer app may know and this one not. */
		chart: string;
		/** The view's name when it was put here, for when it isn't. */
		name: string;
	}

	let { ws, id, chart, name }: Props = $props();

	const PERIODS: Record<string, string> = {
		weeks6: 'last 6 weeks',
		months3: 'last 3 months',
		year: 'this year',
		range: 'the dates picked'
	};

	const replica = $derived(engine.replica(ws));
	const view = $derived(replica?.view(id));
	const shared = $derived(!!view && view.ownerId === null);
	const known = $derived(chartOf(chart));
	const title = $derived(view?.name || name || 'A report');
	const setup = $derived(view && view.type === 'report' ? viewReport(view) : null);
	// Read only: the view is changed on its own page, not from a page showing it.
	const source = $derived<ReportSource | null>(
		setup ? { setup, change: () => {}, editFilter: () => {} } : null
	);
	const when = $derived(
		!known || !setup
			? ''
			: known.tab === 'workload'
				? 'as it stands today'
				: (PERIODS[setup.period] ?? '')
	);
</script>

<section class="embed" aria-label={`${known?.label ?? 'A chart'} from ${title}, a report`}>
	<header>
		<!-- A chart's card names it; the numbers have no card, so they're named here. -->
		{#if !known || known.key.endsWith('-numbers')}<strong>{known?.label ?? 'A chart'}</strong>{/if}
		{#if view && shared}
			<a class="open" href={resolve('/(app)/views/[id]', { id })}>{title}</a>
			{#if when}<span class="when">{when}</span>{/if}
		{/if}
	</header>
	{#if !view}
		<p class="note">This report isn't here: it was deleted, or it's one you can't open.</p>
	{:else if !shared}
		<p class="note">This report is someone's own, so it doesn't show in a page.</p>
	{:else if view.type !== 'report' || !source}
		<p class="note">{title} isn't a report any more, so it has no charts to show.</p>
	{:else if !known}
		<p class="note">This chart is from a newer Mila. Reload to see it.</p>
	{:else}
		<div class="chart">
			<!-- One report that can't be drawn never takes the page with it. -->
			<svelte:boundary>
				{#await import('../reports/Reports.svelte') then { default: Reports }}
					<Reports {ws} {source} fixedProject={view.projectId} only={known.key as ReportChart} />
				{/await}
				{#snippet failed()}
					<p class="note">This chart can't be drawn. Open the report to change its setup.</p>
				{/snippet}
			</svelte:boundary>
		</div>
		<p class="foot">Counts only the tasks you can see.</p>
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
		font-size: var(--text-sm);
	}
	.open {
		display: inline-flex;
		align-items: center;
		min-height: max(var(--tap-min), 24px);
		font-size: var(--text-sm);
		font-weight: 600;
		color: var(--accent-text);
	}
	.when {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.chart {
		padding: var(--space-3);
	}
	.note,
	.foot {
		margin: 0;
		padding: var(--space-3);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.foot {
		padding-top: 0;
		font-size: var(--text-xs);
	}
</style>
