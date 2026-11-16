<script lang="ts" module>
	export interface LineSeries {
		name: string;
		/** A word for the label at the line's end: "done", "made". */
		short: string;
		/** A color token, such as --chart-series-1. */
		color: string;
		values: number[];
	}
</script>

<script lang="ts">
	// Lines over weeks on one axis, drawn by Mila (docs/PLAN.md section 3):
	// 2px lines, a ringed dot and the value at each line's end, a recessive
	// grid. Hovering, a tap or the arrow keys on the focused chart move a
	// crosshair that shows every series at that week, and the same is
	// announced.
	import { announce } from '../../announce.svelte';
	import { labelEvery, nearest, niceScale, stepMark, largest } from '../../charts';
	import ChartTip from './ChartTip.svelte';

	interface Props {
		/** The chart's name for a screen reader. */
		label: string;
		/** The short labels along the bottom. */
		labels: string[];
		/** Each point's words for the tooltip: "Week of Nov 2". */
		titles: string[];
		series: LineSeries[];
		/** A value with its unit: "12 tasks". */
		unit: (n: number) => string;
	}

	let { label, labels, titles, series, unit }: Props = $props();

	const H = 220;
	const T = 12;
	const B = 26;
	const L = 36;
	let width = $state(480);
	let at = $state<number | null>(null);

	const count = $derived(labels.length);
	const scale = $derived(niceScale(largest(0, ...series.map((s) => s.values))));
	// Room on the right for the labels at the lines' ends.
	const R = $derived(
		Math.max(
			16,
			...series.map((s) => (String(s.values.at(-1) ?? 0).length + s.short.length + 1) * 6.6 + 14)
		)
	);
	const plotW = $derived(Math.max(40, width - L - R));
	const x = (i: number) => (count <= 1 ? L + plotW / 2 : L + (i * plotW) / (count - 1));
	const y = (v: number) => T + (H - T - B) * (1 - v / scale.max);
	const every = $derived(labelEvery(count, plotW, 54));
	const shownLabel = (i: number) =>
		i === count - 1 || (i % every === 0 && count - 1 - i >= every * 0.6);

	// The end labels, top to bottom; one that would sit on another is left to the legend.
	const ends = $derived.by(() => {
		const out: { s: LineSeries; v: number; y: number }[] = [];
		const sorted = series.map((s) => ({ s, v: s.values.at(-1) ?? 0 })).sort((a, b) => b.v - a.v);
		for (const e of sorted) {
			const ey = y(e.v);
			if (out.some((o) => Math.abs(o.y - ey) < 13)) continue;
			out.push({ ...e, y: ey });
		}
		return out;
	});

	const summary = $derived(
		count
			? `${titles[0]} to ${titles[count - 1]}. ` +
					series
						.map((s) => `${s.name}: ${unit(s.values[0] ?? 0)} to ${unit(s.values.at(-1) ?? 0)}`)
						.join('. ')
			: 'Nothing to show.'
	);
	const id = $props.id();

	function say(i: number) {
		announce(
			`${titles[i]}: ${series.map((s) => `${s.name} ${unit(s.values[i] ?? 0)}`).join(', ')}`
		);
	}
	function point(e: PointerEvent) {
		const box = (e.currentTarget as SVGElement).closest('svg')!.getBoundingClientRect();
		at = nearest(e.clientX - box.left, L, plotW, count);
	}
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
		say(next);
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
				<line class={v ? 'grid' : 'axis'} x1={L} x2={L + plotW} y1={y(v)} y2={y(v)} />
				<text class="tick" x={L - 6} y={y(v) + 4} text-anchor="end">{v}</text>
			{/each}
			{#each labels as l, i (i)}
				{#if shownLabel(i)}
					<text class="tick" x={x(i)} y={H - 8} text-anchor="middle">{l}</text>
				{/if}
			{/each}
			{#if at !== null}
				<line class="cross" x1={x(at)} x2={x(at)} y1={T} y2={H - B} />
			{/if}
			{#each series as s (s.name)}
				<polyline
					points={s.values.map((v, i) => `${x(i)},${y(v)}`).join(' ')}
					fill="none"
					stroke={`var(${s.color})`}
					stroke-width="2"
					stroke-linejoin="round"
					stroke-linecap="round"
				/>
				{#if s.values.length}
					<circle
						class="dot"
						cx={x(s.values.length - 1)}
						cy={y(s.values.at(-1)!)}
						r="4"
						fill={`var(${s.color})`}
					/>
				{/if}
				{#if at !== null && at !== s.values.length - 1}
					<circle class="dot" cx={x(at)} cy={y(s.values[at] ?? 0)} r="4" fill={`var(${s.color})`} />
				{/if}
			{/each}
			{#each ends as e (e.s.name)}
				<text class="end" x={x(count - 1) + 8} y={e.y + 4}>{e.v} {e.s.short}</text>
			{/each}
			<rect
				class="hit"
				x={L}
				y={T}
				width={plotW}
				height={H - T - B}
				role="presentation"
				onpointermove={point}
				onpointerdown={point}
				onpointerleave={(e) => {
					if (e.pointerType === 'mouse') at = null;
				}}
			/>
		</svg>
		{#if at !== null}
			<ChartTip
				x={x(at)}
				y={Math.min(...series.map((s) => y(s.values[at!] ?? 0)))}
				{width}
				title={titles[at]}
				rows={series.map((s) => ({
					color: s.color,
					label: s.name,
					value: unit(s.values[at!] ?? 0)
				}))}
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
	.axis,
	.cross {
		stroke: var(--border-strong);
		stroke-width: 1;
	}
	.tick {
		fill: var(--text-secondary);
		font-size: var(--text-xs);
		font-variant-numeric: tabular-nums;
	}
	.end {
		fill: var(--text-primary);
		font-size: var(--text-xs);
		font-weight: 550;
		font-variant-numeric: tabular-nums;
	}
	.dot {
		stroke: var(--surface-0);
		stroke-width: 2;
	}
	.hit {
		fill: transparent;
		cursor: crosshair;
	}
</style>
