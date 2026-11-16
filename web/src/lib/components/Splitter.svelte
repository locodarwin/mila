<script lang="ts">
	interface Props {
		/** What it resizes, as its accessible name: "Resize sidebar". */
		label: string;
		/** The id of the panel it resizes. */
		controls?: string;
		value: number;
		min: number;
		max: number;
		/** Which way the panel grows: right for one on the left of it, left for one on the right. */
		grows: 'right' | 'left';
		onresize: (px: number) => void;
		/** A pointer drag started or ended, so the layout can stop animating meanwhile. */
		ondragging?: (on: boolean) => void;
		/**
		 * Marks an edge nothing else shows: a grip in its corner, kept on screen as
		 * what is beside it scrolls, and the width while it moves.
		 */
		grip?: boolean;
		/** Where double-clicking it, or Enter, puts it back to. */
		reset?: number;
		/** In place of reset: puts it back some other way (a board's own fit). */
		onreset?: () => void;
		/**
		 * How many equal widths it moves together: the edge after a board's third
		 * column moves three, so it follows the pointer as each grows by a third.
		 */
		share?: number;
		/** False for one of several edges that resize the same thing, reached by its first. */
		tabbable?: boolean;
	}

	let {
		label,
		controls,
		value,
		min,
		max,
		grows,
		onresize,
		ondragging,
		grip,
		reset,
		onreset,
		share = 1,
		tabbable = true
	}: Props = $props();

	let start: { x: number; value: number } | null = null;
	const sign = $derived(grows === 'right' ? 1 : -1);

	// The width shows while it moves, and a moment after.
	let moved = $state(false);
	let fading: ReturnType<typeof setTimeout> | undefined;
	function resize(px: number) {
		onresize(px);
		moved = true;
		clearTimeout(fading);
		if (!start) fading = setTimeout(() => (moved = false), 1200);
	}
	function backToStart() {
		if (onreset) onreset();
		else if (reset !== undefined) resize(reset);
	}

	function down(e: PointerEvent) {
		if (e.button !== 0) return;
		e.preventDefault();
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		start = { x: e.clientX, value };
		ondragging?.(true);
	}

	function move(e: PointerEvent) {
		if (start) resize(start.value + (sign * (e.clientX - start.x)) / share);
	}

	function up() {
		if (!start) return;
		start = null;
		ondragging?.(false);
		clearTimeout(fading);
		fading = setTimeout(() => (moved = false), 1200);
	}

	// The arrow keys move the edge the way they point, Shift for bigger
	// steps; Home and End go to the narrowest and widest.
	function keys(e: KeyboardEvent) {
		const step = e.shiftKey ? 64 : 16;
		let next: number | null = null;
		if (e.key === 'ArrowRight') next = value + sign * step;
		else if (e.key === 'ArrowLeft') next = value - sign * step;
		else if (e.key === 'Home') next = min;
		else if (e.key === 'End') next = max;
		else if (e.key === 'Enter' && (onreset || reset !== undefined)) {
			e.preventDefault();
			return backToStart();
		}
		if (next === null) return;
		e.preventDefault();
		resize(next);
	}
</script>

<!-- A focusable separator is a control (ARIA's window splitter), moved with the arrow keys. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div
	class="splitter"
	class:grip
	title={grip ? 'Drag to resize. Double-click to reset.' : undefined}
	role="separator"
	tabindex={tabbable ? 0 : -1}
	aria-label={label}
	aria-controls={controls}
	aria-orientation="vertical"
	aria-valuenow={value}
	aria-valuemin={min}
	aria-valuemax={max}
	onpointerdown={down}
	onpointermove={move}
	onpointerup={up}
	onpointercancel={up}
	onkeydown={keys}
	ondblclick={backToStart}
>
	{#if grip}
		<span class="mark" aria-hidden="true">
			<svg width="16" height="16" viewBox="0 0 14 14" fill="none" stroke="currentColor">
				<path stroke-width="1.5" stroke-linecap="round" d="M13 3 3 13M13 7.5 7.5 13M13 12l-1 1" />
			</svg>
			{#if moved}<span class="size">{Math.round(value)} px</span>{/if}
		</span>
	{/if}
</div>

<style>
	.splitter {
		position: absolute;
		top: 0;
		bottom: 0;
		left: 0;
		z-index: 5;
		width: 9px;
		margin-left: -4px;
		cursor: col-resize;
		touch-action: none;
	}
	/* A line that shows where the edge is while it is hovered or focused. */
	.splitter::after {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		left: 3px;
		width: 3px;
		border-radius: var(--radius-full);
		background: transparent;
		transition: background-color var(--dur-micro) var(--ease);
	}
	.splitter:hover::after,
	.splitter:active::after {
		background: var(--border-strong);
	}
	.splitter:focus-visible {
		box-shadow: none;
	}
	.splitter:focus-visible::after {
		background: var(--accent);
	}
	/* A corner grip like a text box's, at the foot of what is on screen. Only the
	   width changes: a page is as tall as its text. */
	.grip {
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		cursor: ew-resize;
	}
	.mark {
		position: sticky;
		bottom: var(--space-1);
		display: grid;
		flex: none;
		place-items: center;
		width: max(var(--tap-min), 28px);
		height: max(var(--tap-min), 28px);
		margin-left: calc(4px - max(var(--tap-min), 28px));
		border-radius: var(--radius-sm);
		background: var(--surface-0);
		color: var(--text-muted);
		transition: color var(--dur-micro) var(--ease);
	}
	.grip:hover .mark,
	.grip:active .mark {
		color: var(--text-primary);
	}
	.grip:focus-visible .mark {
		color: var(--accent);
	}
	.size {
		position: absolute;
		right: 100%;
		bottom: var(--space-1);
		margin-right: var(--space-1);
		padding: 2px var(--space-2);
		border-radius: var(--radius-sm);
		background: var(--text-primary);
		color: var(--surface-0);
		font-size: var(--text-xs);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
		pointer-events: none;
	}
</style>
