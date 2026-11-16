<script lang="ts">
	// A report view's page (docs/PLAN.md section 3, Release 3.0): the Reports
	// page set up as the view is. What a member changes (tab, period, project,
	// filter) is saved to the view, as its filter is; a guest's stays on their
	// screen, as the view's page decides. Loaded only when a report view opens.
	import { reportDisplay, viewReport, type ReportSetup } from '../../reportSetup.svelte';
	import type { ViewEdit } from '../../sync/data';
	import type { SavedView, ViewDisplay, ViewFilter } from '../../sync/entities';
	import type { DisplayPath } from '../../displayPaths';
	import Button from '../Button.svelte';
	import Reports from './Reports.svelte';

	interface Props {
		ws: string;
		view: SavedView;
		setFilter: (edit: ViewEdit<ViewFilter, Partial<ViewFilter>>) => Promise<void>;
		setDisplay: (
			edit: ViewEdit<ViewDisplay, Partial<Record<DisplayPath, unknown>>>
		) => Promise<void>;
	}

	let { ws, view, setFilter, setDisplay }: Props = $props();

	const source = $derived({
		setup: viewReport(view),
		change: (patch: Partial<ReportSetup>) => {
			const { filter, ...rest } = patch;
			// A filter given whole replaces the view's: what it leaves out goes.
			if (filter)
				void setFilter((now) => {
					const out: Partial<ViewFilter> = { ...filter };
					for (const k of Object.keys(now))
						if (!(k in filter)) out[k as keyof ViewFilter] = undefined;
					return out;
				});
			if (Object.keys(rest).length)
				void setDisplay((d) => ({
					report: reportDisplay({ ...viewReport({ ...view, display: d }), ...rest })
				}));
		},
		editFilter: (edit: ViewEdit<ViewFilter, Partial<ViewFilter>>) => void setFilter(edit)
	});
</script>

<!-- A setup that can't be drawn leaves the page, with a way back. -->
<svelte:boundary>
	<Reports {ws} {source} fixedProject={view.projectId} />
	{#snippet failed(error, reset)}
		<div class="failed" role="alert">
			<p>This report can't be drawn with its setup.</p>
			{#if import.meta.env.DEV}<p class="sub">{String(error)}</p>{/if}
			<Button
				onclick={() => {
					void setDisplay(() => ({
						report: reportDisplay({ ...viewReport(view), period: 'weeks6', from: null, to: null })
					})).then(reset);
				}}>Show the last 6 weeks instead</Button
			>
		</div>
	{/snippet}
</svelte:boundary>

<style>
	.failed {
		display: grid;
		gap: var(--space-2);
		justify-items: start;
		padding: var(--space-4);
		color: var(--text-secondary);
	}
</style>
