<script lang="ts" module>
	import type { TipRow } from './ChartTip.svelte';

	export interface Dot {
		key: string;
		/** Where it falls along the bottom, from 0 to 1. */
		x: number;
		y: number;
		title: string;
		rows: TipRow[];
	}

	export interface Mark {
		value: number;
		/** Its words at the line's end: "median 2.1 days". */
		label: string;
		/** A color token. */
		color: string;
	}
</script>

<script lang="ts">
	// One dot a thing, by when (along the bottom) and how much (up the side),
	// drawn by Mila (docs/PLAN.md section 3): 4.5px dots ringed in the page's
	// color, dashed lines for marks such as the median, a recessive grid.
	// Hovering or a tap shows the nearest dot's tooltip; the arrow keys on the
	// focused chart step through the dots in order, and the same is announced.
	import { announce } from '../../announce.svelte';
	import { niceScale, stepMark, largest } from '../../charts';
	import ChartTip from './ChartTip.svelte';

	interface Props {
		label: string;
		dots: Dot[];
		marks: Mark[];
		/** The words at the bottom's two ends. */
		start: string;
		end: string;
		/** A tick's words: "3d". */
		tick: (v: number) => string;
		/** The dots' color token. */
		color: string;
		/** What the summary says of them all. */
		summary: string;
	}

	let { label, dots, marks, start, end, tick, color, summary }: Props = $props();

	const H = 220;
	const T = 12;
	const B = 26;
	const L = 40;
	const R = 12;
	let width = $state(480);
	let at = $state<number | null>(null);

	const scale = $derived(
		niceScale(
			largest(
				1,
				dots.map((d) => d.y),
				marks.map((m) => m.value)
			)
		)
	);
	const plotW = $derived(Math.max(40, width - L - R));
	const x = (f: number) => L + f * plotW;
	const y = (v: number) => T + (H - T - B) * (1 - v / scale.max);
	const id = $props.id();
	// A mark's label above its line, or above the label it would sit on.
	const placed = $derived.by(() => {
		const out: { m: Mark; ly: number }[] = [];
		for (const m of [...marks].sort((a, b) => a.value - b.value)) {
			let ly = y(m.value) - 5;
			for (const o of out) if (Math.abs(o.ly - ly) < 13) ly = o.ly - 13;
			out.push({ m, ly });
		}
		return out;
	});

	const say = (d: Dot) => `${d.title}: ${d.rows.map((r) => `${r.label} ${r.value}`).join(', ')}`;

	function point(e: PointerEvent) {
		const box = (e.currentTarget as SVGElement).closest('svg')!.getBoundingClientRect();
		const px = e.clientX - box.left;
		const py = e.clientY - box.top;
		let best: number | null = null;
		let far = 24 * 24;
		dots.forEach((d, i) => {
			const dist = (x(d.x) - px) ** 2 + (y(d.y) - py) ** 2;
			if (dist < far) {
				far = dist;
				best = i;
			}
		});
		at = best;
	}
	function keys(e: KeyboardEvent) {
		if (e.key === 'Escape' && at !== null) {
			e.preventDefault();
			at = null;
			return;
		}
		const next = stepMark(e.key, at, dots.length);
		if (next === null) return;
		e.preventDefault();
		at = next;
		announce(say(dots[next]));
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
				<text class="tick" x={L - 6} y={y(v) + 4} text-anchor="end">{tick(v)}</text>
			{/each}
			<text class="tick" x={L} y={H - 8}>{start}</text>
			<text class="tick" x={L + plotW} y={H - 8} text-anchor="end">{end}</text>
			{#each dots as d, i (d.key)}
				<circle
					class="dot"
					class:dim={at !== null && at !== i}
					cx={x(d.x)}
					cy={y(d.y)}
					r={at === i ? 6 : 4.5}
					fill={`var(${color})`}
				/>
			{/each}
			{#each placed as { m, ly } (m.label)}
				<line
					class="mark"
					x1={L}
					x2={L + plotW}
					y1={y(m.value)}
					y2={y(m.value)}
					stroke={`var(${m.color})`}
				/>
				<text class="mark-label" x={L + plotW} y={ly} text-anchor="end">{m.label}</text>
			{/each}
			<rect
				class="hit"
				x={L - 8}
				y={0}
				width={plotW + 16}
				height={H - B}
				role="presentation"
				onpointermove={point}
				onpointerdown={point}
				onpointerleave={(e) => {
					if (e.pointerType === 'mouse') at = null;
				}}
			/>
		</svg>
		{#if at !== null && dots[at]}
			<ChartTip
				x={x(dots[at].x)}
				y={y(dots[at].y)}
				{width}
				title={dots[at].title}
				rows={dots[at].rows}
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
	.dot {
		stroke: var(--surface-0);
		stroke-width: 2;
		transition: opacity var(--dur-micro) var(--ease);
	}
	.dot.dim {
		opacity: 0.45;
	}
	.mark {
		stroke-width: 2;
		stroke-dasharray: 6 4;
	}
	.mark-label {
		fill: var(--text-primary);
		font-size: var(--text-xs);
		font-weight: 550;
		font-variant-numeric: tabular-nums;
		paint-order: stroke;
		stroke: var(--surface-0);
		stroke-width: 3px;
	}
	.hit {
		fill: transparent;
	}
</style>
