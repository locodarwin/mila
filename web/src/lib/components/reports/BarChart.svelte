<script lang="ts" module>
	import type { TipRow } from './ChartTip.svelte';

	export interface Bar {
		key: string;
		name: string;
		value: number;
		/** The words at the bar's end: "9 h, 4 tasks". */
		label: string;
		/** Drawn quietly: the unassigned work. */
		muted?: boolean;
		/** The tooltip's lines. */
		tip: TipRow[];
	}
</script>

<script lang="ts">
	// Horizontal bars of one series, drawn by Mila:
	// a name before each, its value after it, rounded at the end and square
	// at the axis. Hover, a tap or the arrow keys on the focused chart show a
	// bar's tooltip, and the same is announced.
	import { announce } from '../../announce.svelte';
	import { barPath, stepMark, largest } from '../../charts';
	import ChartTip from './ChartTip.svelte';

	interface Props {
		label: string;
		bars: Bar[];
		/** The bars' color token. */
		color: string;
	}

	let { label, bars, color }: Props = $props();

	const ROW = 32;
	const BAR = 18;
	const T = 4;
	let width = $state(480);
	let at = $state<number | null>(null);

	const H = $derived(T * 2 + bars.length * ROW);
	// Names get up to a third of the width; longer ones are cut short.
	const L = $derived(Math.min(Math.max(72, width / 3), 160));
	const fits = $derived(Math.floor((L - 12) / 6.8));
	const short = (s: string) => (s.length > fits ? `${s.slice(0, Math.max(1, fits - 1))}…` : s);
	const R = $derived(
		largest(
			60,
			bars.map((b) => b.label.length * 6.6 + 12)
		)
	);
	const room = $derived(Math.max(20, width - L - R));
	const max = $derived(
		largest(
			0,
			bars.map((b) => b.value)
		) || 1
	);
	const len = (v: number) => (room * v) / max;
	const top = (i: number) => T + i * ROW + (ROW - BAR) / 2;
	const id = $props.id();
	const say = (b: Bar) => `${b.name}: ${b.tip.map((r) => `${r.label} ${r.value}`).join(', ')}`;

	function keys(e: KeyboardEvent) {
		if (e.key === 'Escape' && at !== null) {
			e.preventDefault();
			at = null;
			return;
		}
		const next = stepMark(e.key, at, bars.length);
		if (next === null) return;
		e.preventDefault();
		at = next;
		announce(say(bars[next]));
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
			<line class="axis" x1={L} x2={L} y1={T} y2={H - T} />
			{#each bars as b, i (b.key)}
				<text
					class="name"
					class:muted={b.muted}
					x={L - 10}
					y={top(i) + BAR / 2 + 4}
					text-anchor="end">{short(b.name)}</text
				>
				<path
					class:dim={at !== null && at !== i}
					d={barPath(L, top(i), len(b.value), BAR)}
					fill={b.muted ? 'var(--border-strong)' : `var(${color})`}
				/>
				<text class="value" x={L + len(b.value) + 8} y={top(i) + BAR / 2 + 4}>{b.label}</text>
				<rect
					class="hit"
					x="0"
					y={T + i * ROW}
					{width}
					height={ROW}
					role="presentation"
					onpointerenter={() => (at = i)}
					onpointerdown={() => (at = i)}
					onpointerleave={(e) => {
						if (e.pointerType === 'mouse' && at === i) at = null;
					}}
				/>
			{/each}
		</svg>
		{#if at !== null && bars[at]}
			<ChartTip
				x={L + len(bars[at].value)}
				y={top(at)}
				{width}
				title={bars[at].name}
				rows={bars[at].tip}
			/>
		{/if}
	</div>
	<p class="visually-hidden" id={`${id}-sum`}>{bars.map(say).join('. ') || 'Nothing to show.'}</p>
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
	.axis {
		stroke: var(--border-strong);
		stroke-width: 1;
	}
	.name {
		fill: var(--text-primary);
		font-size: var(--text-xs);
	}
	.name.muted {
		fill: var(--text-secondary);
	}
	.value {
		fill: var(--text-secondary);
		font-size: var(--text-xs);
		font-variant-numeric: tabular-nums;
	}
	path {
		transition: opacity var(--dur-micro) var(--ease);
	}
	path.dim {
		opacity: 0.55;
	}
	.hit {
		fill: transparent;
	}
</style>
