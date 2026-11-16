<script lang="ts" module>
	/** A line of a chart's tooltip: a swatch of the series' token, its name and value. */
	export interface TipRow {
		/** A color token, such as --chart-series-1, or none for a line of words. */
		color?: string;
		label: string;
		value: string;
	}
</script>

<script lang="ts">
	// A chart's tooltip, over the chart at the mark it describes, kept
	// inside the chart's box. Screen readers hear the same through the
	// chart's announcement, so it is hidden from them.
	interface Props {
		/** Where the mark is, in pixels from the chart's top left. */
		x: number;
		y: number;
		/** The chart's width, to keep the tooltip inside it. */
		width: number;
		title: string;
		rows: TipRow[];
	}

	let { x, y, width, title, rows }: Props = $props();
	let tipWidth = $state(140);
	let tipHeight = $state(60);
	const left = $derived(Math.max(0, Math.min(x + 12, width - tipWidth)));
	const top = $derived(Math.max(0, y - tipHeight - 8));
</script>

<div
	class="tip"
	aria-hidden="true"
	style:left={`${left}px`}
	style:top={`${top}px`}
	bind:clientWidth={tipWidth}
	bind:clientHeight={tipHeight}
>
	<b>{title}</b>
	{#each rows as r, i (i)}
		<span class="row">
			{#if r.color}<i style:background={`var(${r.color})`}></i>{/if}
			<span>{r.label}</span>
			<span class="v tabular">{r.value}</span>
		</span>
	{/each}
</div>

<style>
	.tip {
		position: absolute;
		z-index: 2;
		display: grid;
		gap: 1px;
		min-width: 130px;
		max-width: 260px;
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--menu-bg);
		box-shadow: var(--shadow-md);
		color: var(--text-primary);
		font-size: var(--text-xs);
		line-height: 1.45;
		pointer-events: none;
	}
	b {
		margin-bottom: 2px;
		font-weight: 600;
	}
	.row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--text-secondary);
	}
	i {
		flex: none;
		width: 8px;
		height: 8px;
		border-radius: 2px;
	}
	.v {
		margin-left: auto;
		padding-left: var(--space-3);
		color: var(--text-primary);
		font-weight: 600;
	}
</style>
