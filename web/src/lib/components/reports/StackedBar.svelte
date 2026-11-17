<script lang="ts" module>
	export interface Part {
		key: string;
		name: string;
		value: number;
		/** A color token, such as --chart-series-1. */
		color: string;
	}
</script>

<script lang="ts">
	// One whole split into its parts, drawn by Mila:
	// one bar, a 2px gap in the page's color between the parts, rounded at
	// the whole bar's two ends, each part's count under its start where it
	// fits, and a legend of their names. Hover, a tap or the arrow keys on
	// the focused chart show a part's tooltip, and the same is announced.
	import { announce } from '../../announce.svelte';
	import { stepMark } from '../../charts';
	import ChartTip from './ChartTip.svelte';

	interface Props {
		label: string;
		parts: Part[];
		/** A count with its unit: "7 open tasks". */
		unit: (n: number) => string;
		/** The tooltip's words for a part's count and its share. */
		what?: string;
		of?: string;
	}

	let { label, parts, unit, what = 'Open tasks', of = 'Of all open' }: Props = $props();

	const H = 64;
	const T = 6;
	const BAR = 26;
	const GAP = 2;
	let width = $state(480);
	let at = $state<number | null>(null);

	const shown = $derived(parts.filter((p) => p.value > 0));
	const total = $derived(shown.reduce((s, p) => s + p.value, 0));
	const blocks = $derived.by(() => {
		const room = width - GAP * Math.max(0, shown.length - 1);
		let x = 0;
		return shown.map((p) => {
			const w = total ? (room * p.value) / total : 0;
			const b = { p, x, w };
			x += w + GAP;
			return b;
		});
	});
	const share = (v: number) => (total ? Math.round((v / total) * 100) : 0);

	/** A block's shape: square, but rounded where it ends the whole bar. */
	function shape(x: number, w: number, first: boolean, last: boolean): string {
		const r = Math.min(4, w / 2);
		const rl = first ? r : 0;
		const rr = last ? r : 0;
		const y = T;
		const b = T + BAR;
		return (
			`M${x + rl},${y}H${x + w - rr}` +
			(rr
				? `Q${x + w},${y} ${x + w},${y + rr}V${b - rr}Q${x + w},${b} ${x + w - rr},${b}`
				: `V${b}`) +
			`H${x + rl}` +
			(rl ? `Q${x},${b} ${x},${b - rl}V${y + rl}Q${x},${y} ${x + rl},${y}` : `V${y}`) +
			'Z'
		);
	}
	const id = $props.id();
	const say = (p: Part) => `${p.name}: ${unit(p.value)}, ${share(p.value)}%`;

	function keys(e: KeyboardEvent) {
		if (e.key === 'Escape' && at !== null) {
			e.preventDefault();
			at = null;
			return;
		}
		const next = stepMark(e.key, at, blocks.length);
		if (next === null) return;
		e.preventDefault();
		at = next;
		announce(say(blocks[next].p));
	}
</script>

<div class="chart">
	<ul class="legend" aria-hidden="true">
		{#each shown as p (p.key)}<li><i style:background={`var(${p.color})`}></i>{p.name}</li>{/each}
	</ul>
	<!-- Focusable, so the arrow keys can step through its marks; its name and summary say what it shows. -->
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div
		class="plot"
		role="group"
		tabindex="0"
		aria-label={label}
		aria-describedby={`${id}-sum`}
		bind:clientWidth={width}
		onkeydown={keys}
		onblur={() => (at = null)}
	>
		<svg {width} height={H} viewBox={`0 0 ${width} ${H}`} aria-hidden="true">
			{#each blocks as b, i (b.p.key)}
				<path
					class:dim={at !== null && at !== i}
					d={shape(b.x, b.w, i === 0, i === blocks.length - 1)}
					fill={`var(${b.p.color})`}
				/>
				{#if b.w >= String(b.p.value).length * 7 + 4}
					<text class="value" x={b.x} y={T + BAR + 18}>{b.p.value}</text>
				{/if}
				<rect
					class="hit"
					x={b.x}
					y="0"
					width={b.w + GAP}
					height={H}
					role="presentation"
					onpointerenter={() => (at = i)}
					onpointerdown={() => (at = i)}
					onpointerleave={(e) => {
						if (e.pointerType === 'mouse' && at === i) at = null;
					}}
				/>
			{/each}
		</svg>
		{#if at !== null && blocks[at]}
			<ChartTip
				x={blocks[at].x + blocks[at].w / 2}
				y={T}
				{width}
				title={blocks[at].p.name}
				rows={[
					{ color: blocks[at].p.color, label: what, value: String(blocks[at].p.value) },
					{ label: of, value: `${share(blocks[at].p.value)}%` }
				]}
			/>
		{/if}
	</div>
	<p class="visually-hidden" id={`${id}-sum`}>{shown.map(say).join('. ') || 'Nothing to show.'}</p>
</div>

<style>
	.chart {
		position: relative;
		display: grid;
		/* No wider than its card, whatever the bar was last drawn at. */
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-2);
		min-width: 0;
	}
	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1) var(--space-4);
		margin: 0;
		padding: 0;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		list-style: none;
	}
	.legend i {
		display: inline-block;
		width: 10px;
		height: 10px;
		margin-right: var(--space-1);
		border-radius: 2px;
		vertical-align: -1px;
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
	.value {
		fill: var(--text-primary);
		font-size: var(--text-xs);
		font-weight: 550;
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
