<script lang="ts" module>
	export interface Stack {
		name: string;
		/** A color token, such as --chart-series-1. */
		color: string;
		values: number[];
	}
</script>

<script lang="ts">
	// Columns of several series stacked, drawn by Mila (docs/PLAN.md section 3):
	// the first series at the bottom, a 1px line in the page's color between
	// parts, the top part rounded, each column's total on its cap, a recessive
	// grid. A column still under way is drawn lighter. Hover, a tap or the arrow
	// keys on the focused chart show every part of a column, and the same is
	// announced.
	import { announce } from '../../announce.svelte';
	import { columnPath, largest, niceScale, stepMark } from '../../charts';
	import ChartTip from './ChartTip.svelte';

	interface Props {
		/** The chart's name for a screen reader. */
		label: string;
		/** The short labels under the columns. */
		labels: string[];
		/** Each column's words for the tooltip: "Week of Nov 2". */
		titles: string[];
		/** Bottom first. */
		stacks: Stack[];
		/** Each column's total, which may be less than its parts add up to. */
		totals: number[];
		/** A column still under way, drawn lighter and said to be so far. */
		partial?: number | null;
		/** A value with its unit: "3.5h". */
		unit: (n: number) => string;
	}

	let { label, labels, titles, stacks, totals, partial = null, unit }: Props = $props();

	const H = 240;
	const T = 22;
	const B = 26;
	const L = 36;
	const R = 6;
	let width = $state(480);
	let at = $state<number | null>(null);

	const count = $derived(labels.length);
	/** Each part's top in each column: the parts below it and itself. */
	const tops = $derived.by(() => {
		const out: number[][] = [];
		stacks.forEach((s, i) => out.push(s.values.map((v, j) => v + (i ? out[i - 1][j] : 0))));
		return out;
	});
	const scale = $derived(niceScale(largest(0, totals, tops.at(-1) ?? [])));
	const band = $derived(Math.max(1, (width - L - R) / Math.max(1, count)));
	const bw = $derived(Math.min(36, band * 0.62));
	const y = (v: number) => T + (H - T - B) * (1 - v / scale.max);
	const cx = (i: number) => L + band * i + band / 2;
	const capOf = (j: number) => Math.max(totals[j] ?? 0, tops.at(-1)?.[j] ?? 0);
	/** The topmost part with something in each column, which is rounded. */
	const topmost = (j: number) => {
		for (let i = stacks.length - 1; i >= 0; i--) if (stacks[i].values[j] > 0) return i;
		return -1;
	};
	// Each total on its cap while they fit, else every other one.
	const every = $derived(band >= 40 ? 1 : 2);
	const shownCap = (j: number) => j % every === 0 || j === count - 1;
	const id = $props.id();
	const said = (j: number) =>
		`${titles[j]}${partial === j ? ', so far' : ''}: ${unit(totals[j] ?? 0)} in all. ` +
		stacks
			.filter((s) => s.values[j] > 0)
			.map((s) => `${s.name} ${unit(s.values[j])}`)
			.join(', ');
	const summary = $derived(labels.map((_, j) => said(j)).join('. ') || 'Nothing to show.');

	function keys(e: KeyboardEvent) {
		if (e.key === 'Escape' && at !== null) {
			e.preventDefault();
			at = null;
			return;
		}
		const next = stepMark(e.key, at, count);
		if (next === null) return;
		e.preventDefault();
		at = next;
		announce(said(next));
	}
</script>

<div class="chart" bind:clientWidth={width}>
	<!-- Focusable, so the arrow keys can step through its columns; its name and summary say what it shows. -->
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div
		class="plot"
		role="group"
		tabindex="0"
		aria-label={label}
		aria-describedby={`${id}-sum`}
		onkeydown={keys}
		onblur={() => (at = null)}
	>
		<svg {width} height={H} viewBox={`0 0 ${width} ${H}`} aria-hidden="true">
			{#each scale.ticks as v (v)}
				<line class={v ? 'grid' : 'axis'} x1={L} x2={width - R} y1={y(v)} y2={y(v)} />
				<text class="tick" x={L - 6} y={y(v) + 4} text-anchor="end">{unit(v)}</text>
			{/each}
			{#each labels as name, j (j)}
				<g class="col" class:dim={at !== null && at !== j} class:partial={partial === j}>
					{#each stacks as s, i (i)}
						{#if s.values[j] > 0}
							{@const bottom = y(i ? tops[i - 1][j] : 0)}
							{@const top = y(tops[i][j])}
							{#if i === topmost(j)}
								<path d={columnPath(cx(j) - bw / 2, top, bw, bottom)} fill={`var(${s.color})`} />
							{:else}
								<rect
									x={cx(j) - bw / 2}
									y={top}
									width={bw}
									height={bottom - top}
									fill={`var(${s.color})`}
								/>
							{/if}
							{#if i}<line
									class="seam"
									x1={cx(j) - bw / 2}
									x2={cx(j) + bw / 2}
									y1={bottom}
									y2={bottom}
								/>{/if}
						{/if}
					{/each}
				</g>
				{#if shownCap(j) && (totals[j] ?? 0) > 0}
					<text class="value" x={cx(j)} y={y(capOf(j)) - 6} text-anchor="middle"
						>{unit(totals[j])}{partial === j ? ' so far' : ''}</text
					>
				{/if}
				<text class="tick" x={cx(j)} y={H - 8} text-anchor="middle">{name}</text>
				<rect
					class="hit"
					x={L + band * j}
					y={T}
					width={band}
					height={H - T - B}
					role="presentation"
					onpointerenter={() => (at = j)}
					onpointerdown={() => (at = j)}
					onpointerleave={(e) => {
						if (e.pointerType === 'mouse' && at === j) at = null;
					}}
				/>
			{/each}
		</svg>
		{#if at !== null}
			<ChartTip
				x={cx(at)}
				y={y(capOf(at))}
				{width}
				title={`${titles[at]}${partial === at ? ', so far' : ''}`}
				rows={[
					...stacks
						.map((s) => ({ color: s.color, label: s.name, value: unit(s.values[at!]) }))
						.filter((_, i) => stacks[i].values[at!] > 0)
						.reverse(),
					{ label: 'In all', value: unit(totals[at] ?? 0) }
				]}
			/>
		{/if}
	</div>
	<p class="visually-hidden" id={`${id}-sum`}>{summary}</p>
</div>

<style>
	.chart {
		position: relative;
		min-width: 0;
	}
	.plot {
		position: relative;
		border-radius: var(--radius-md);
	}
	.plot:focus-visible {
		outline: none;
		box-shadow: var(--focus-ring);
	}
	svg {
		display: block;
		overflow: visible;
	}
	.grid {
		stroke: var(--border);
		stroke-width: 1;
	}
	.axis {
		stroke: var(--border-strong);
		stroke-width: 1;
	}
	.seam {
		stroke: var(--surface-0);
		stroke-width: 1;
	}
	.tick {
		fill: var(--text-secondary);
		font-size: var(--text-xs);
		font-variant-numeric: tabular-nums;
	}
	.value {
		fill: var(--text-primary);
		font-size: var(--text-xs);
		font-weight: 550;
		font-variant-numeric: tabular-nums;
	}
	.col {
		transition: opacity var(--dur-micro) var(--ease);
	}
	.col.partial {
		opacity: 0.5;
	}
	.col.dim {
		opacity: 0.55;
	}
	.col.partial.dim {
		opacity: 0.3;
	}
	.hit {
		fill: transparent;
	}
</style>
