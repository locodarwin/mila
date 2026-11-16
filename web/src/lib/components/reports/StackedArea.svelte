<script lang="ts" module>
	export interface Band {
		name: string;
		/** A color token, such as --chart-series-1. */
		color: string;
		values: number[];
	}
</script>

<script lang="ts">
	// Bands stacked over days on one axis, drawn by Mila (docs/PLAN.md
	// section 3): the first band at the bottom, a 1px line in the page's
	// color between them, a recessive grid. Hovering, a tap or the arrow keys
	// on the focused chart move a crosshair that shows every band that day,
	// and the same is announced.
	import { announce } from '../../announce.svelte';
	import { labelEvery, nearest, niceScale, stepMark, largest } from '../../charts';
	import ChartTip from './ChartTip.svelte';

	interface Props {
		/** The chart's name for a screen reader. */
		label: string;
		/** The short labels along the bottom. */
		labels: string[];
		/** Each point's words for the tooltip: "Tue, Nov 3". */
		titles: string[];
		/** Bottom first. */
		bands: Band[];
		/** A value with its unit: "12 tasks". */
		unit: (n: number) => string;
	}

	let { label, labels, titles, bands, unit }: Props = $props();

	const H = 220;
	const T = 12;
	const B = 26;
	const L = 36;
	const R = 12;
	let width = $state(480);
	let at = $state<number | null>(null);

	const count = $derived(labels.length);
	/** Each band's top at each point: the bands below it and itself. */
	const tops = $derived.by(() => {
		const out: number[][] = [];
		bands.forEach((b, i) => out.push(b.values.map((v, j) => v + (i ? out[i - 1][j] : 0))));
		return out;
	});
	const scale = $derived(niceScale(largest(0, tops.at(-1) ?? [])));
	const plotW = $derived(Math.max(40, width - L - R));
	const x = (i: number) => (count <= 1 ? L + plotW / 2 : L + (i * plotW) / (count - 1));
	const y = (v: number) => T + (H - T - B) * (1 - v / scale.max);
	const every = $derived(labelEvery(count, plotW, 54));
	const shownLabel = (i: number) =>
		i === count - 1 || (i % every === 0 && count - 1 - i >= every * 0.6);

	/** A band's shape: along its top, and back along the top of the one below. */
	function area(i: number): string {
		const top = tops[i].map((v, j) => `${x(j)},${y(v)}`);
		const under = (i ? tops[i - 1] : tops[i].map(() => 0)).map((v, j) => `${x(j)},${y(v)}`);
		return `M${top.join('L')}L${under.reverse().join('L')}Z`;
	}

	const total = (i: number) => tops.at(-1)?.[i] ?? 0;
	const summary = $derived(
		count
			? `${titles[0]} to ${titles[count - 1]}. ` +
					bands
						.map((b) => `${b.name}: ${unit(b.values[0] ?? 0)} to ${unit(b.values.at(-1) ?? 0)}`)
						.join('. ')
			: 'Nothing to show.'
	);
	const id = $props.id();

	function say(i: number) {
		announce(
			`${titles[i]}: ${[...bands]
				.reverse()
				.map((b) => `${b.name} ${unit(b.values[i] ?? 0)}`)
				.join(', ')}`
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
					<text
						class="tick"
						x={x(i)}
						y={H - 8}
						text-anchor={i === 0 ? 'start' : i === count - 1 ? 'end' : 'middle'}>{l}</text
					>
				{/if}
			{/each}
			{#if count > 1}
				{#each bands as b, i (b.name)}
					<path class="band" d={area(i)} fill={`var(${b.color})`} />
				{/each}
			{/if}
			{#if at !== null}
				<line class="cross" x1={x(at)} x2={x(at)} y1={T} y2={H - B} />
			{/if}
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
				y={y(total(at))}
				{width}
				title={titles[at]}
				rows={[
					...[...bands].reverse().map((b) => ({
						color: b.color,
						label: b.name,
						value: unit(b.values[at!] ?? 0)
					})),
					{ label: 'In all', value: unit(total(at)) }
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
	.axis,
	.cross {
		stroke: var(--border-strong);
		stroke-width: 1;
	}
	.band {
		stroke: var(--surface-0);
		stroke-width: 1;
		stroke-linejoin: round;
	}
	.tick {
		fill: var(--text-secondary);
		font-size: var(--text-xs);
		font-variant-numeric: tabular-nums;
	}
	.hit {
		fill: transparent;
		cursor: crosshair;
	}
</style>
