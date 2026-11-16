<script lang="ts">
	// Columns of one series, drawn by Mila (docs/PLAN.md section 3): thin
	// columns rounded at the top and square on the baseline, each value on
	// its cap, a recessive grid. A column can take a color of its own (the
	// overdue one, in the danger color). Hover, a tap or the arrow keys on
	// the focused chart show a column's tooltip, and the same is announced.
	import { announce } from '../../announce.svelte';
	import { columnPath, largest as most, niceScale, stepMark } from '../../charts';
	import ChartTip from './ChartTip.svelte';

	interface Props {
		label: string;
		/** The short labels under the columns. */
		labels: string[];
		/** Each column's words for the tooltip: "Due next week". */
		titles: string[];
		values: number[];
		/** The series' name in the tooltip: "Open tasks". */
		name: string;
		/** The columns' color token, and one for a column of its own. */
		color: string;
		colorOf?: (i: number) => string;
		unit: (n: number) => string;
	}

	let { label, labels, titles, values, name, color, colorOf, unit }: Props = $props();

	const H = 220;
	const T = 18;
	const B = 26;
	const L = 30;
	const R = 6;
	let width = $state(480);
	let at = $state<number | null>(null);

	const count = $derived(values.length);
	const scale = $derived(niceScale(most(0, values)));
	const band = $derived(Math.max(1, (width - L - R) / Math.max(1, count)));
	const bw = $derived(Math.min(24, band * 0.62));
	const y = (v: number) => T + (H - T - B) * (1 - v / scale.max);
	const cx = (i: number) => L + band * i + band / 2;
	const fill = (i: number) => `var(${colorOf?.(i) ?? color})`;
	// A value on every cap while they fit, else only on the largest.
	const largest = $derived(values.indexOf(most(-Infinity, values)));
	const labelAll = $derived(band >= 26);
	const id = $props.id();
	const summary = $derived(
		labels.map((_, i) => `${titles[i]}: ${unit(values[i])}`).join('. ') || 'Nothing to show.'
	);

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
		announce(`${titles[next]}: ${unit(values[next])}`);
	}
</script>

<div class="chart" bind:clientWidth={width}>
	<!-- Focusable, so the arrow keys can step through its marks; its name and summary say what it shows. -->
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
				<text class="tick" x={L - 6} y={y(v) + 4} text-anchor="end">{v}</text>
			{/each}
			{#each values as v, i (i)}
				<path
					class="column"
					class:dim={at !== null && at !== i}
					d={columnPath(cx(i) - bw / 2, y(v), bw, H - B)}
					fill={fill(i)}
				/>
				{#if labelAll || i === largest}
					<text class="value" x={cx(i)} y={y(v) - 5} text-anchor="middle">{v}</text>
				{/if}
				<text class="tick" x={cx(i)} y={H - 8} text-anchor="middle">{labels[i]}</text>
				<rect
					class="hit"
					x={L + band * i}
					y={T}
					width={band}
					height={H - T - B}
					role="presentation"
					onpointerenter={() => (at = i)}
					onpointerdown={() => (at = i)}
					onpointerleave={(e) => {
						if (e.pointerType === 'mouse' && at === i) at = null;
					}}
				/>
			{/each}
		</svg>
		{#if at !== null}
			<ChartTip
				x={cx(at)}
				y={y(values[at])}
				{width}
				title={titles[at]}
				rows={[{ color: colorOf?.(at) ?? color, label: name, value: unit(values[at]) }]}
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
	.column {
		transition: opacity var(--dur-micro) var(--ease);
	}
	.column.dim {
		opacity: 0.55;
	}
	.hit {
		fill: transparent;
	}
</style>
