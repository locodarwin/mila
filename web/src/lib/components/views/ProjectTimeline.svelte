<script lang="ts">
	// A project's tasks as a timeline, on the project's own page. The page
	// has no saved view to keep a layout in, so its sort, grouping and zoom
	// are kept on this device; Save as a view shares them.
	import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
	import { announce } from '../../announce.svelte';
	import { applyPaths, type DisplayPath } from '../../displayPaths';
	import { preferences } from '../../preferences.svelte';
	import type { ViewEdit } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { ViewDisplay } from '../../sync/entities';
	import DisplayPanel from './DisplayPanel.svelte';
	import TimelineView from './TimelineView.svelte';

	let { ws, projectId }: { ws: string; projectId: string } = $props();
	const key = $derived(`mila.projectTimeline.${projectId}`);

	function read(k: string): ViewDisplay {
		try {
			const v = JSON.parse(localStorage.getItem(k) ?? '{}');
			return typeof v === 'object' && v !== null && !Array.isArray(v) ? v : {};
		} catch {
			return {};
		}
	}
	let display = $derived<ViewDisplay>(read(key));

	function onchange(
		edit: ViewEdit<ViewDisplay, Partial<Record<DisplayPath, unknown>>>,
		say?: (d: ViewDisplay) => string
	) {
		display = applyPaths(display, typeof edit === 'function' ? edit(display) : edit);
		try {
			localStorage.setItem(key, JSON.stringify(display));
		} catch {
			// Kept for this visit only.
		}
		const said = say?.(display);
		if (said) announce(said);
	}

	const look = $derived({
		projectId,
		fields: engine.replica(ws)?.fields(projectId) ?? []
	});
	const filter = $derived({ showDone: preferences.current.showCompleted });
	let displaying = $state(false);
	let button = $state<HTMLButtonElement>();
</script>

<div class="frame">
	<div class="bar">
		<button
			title="Choose what shows, and how it is sorted and grouped"
			class="tool"
			type="button"
			aria-expanded={displaying}
			bind:this={button}
			onclick={() => (displaying = !displaying)}
		>
			<SlidersHorizontal size={15} />Display
		</button>
		<span class="note">Kept on this device. Save as a view to share it.</span>
	</div>
	{#if displaying}
		<DisplayPanel
			type="timeline"
			{display}
			{look}
			{onchange}
			onclose={() => {
				displaying = false;
				button?.focus();
			}}
		/>
	{/if}
	<div class="body">
		<TimelineView
			{ws}
			{projectId}
			{filter}
			{display}
			{onchange}
			label="Tasks"
			emptyTitle="No tasks yet"
			emptyHint="Add the first one with quick add."
		/>
	</div>
</div>

<style>
	.frame {
		display: flex;
		flex-direction: column;
		height: 100%;
	}
	.bar {
		flex: none;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-4);
	}
	.tool {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 28px);
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.tool:hover,
	.tool[aria-expanded='true'] {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.note {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.body {
		flex: 1;
		min-height: 0;
	}
</style>
