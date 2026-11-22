<script lang="ts">
	// A diagram in a document, drawn from its source (docs/diagrams.ts): the
	// drawing, its buttons and full screen. Its source is the block's text,
	// which the editor shows beside or under this while the caret is in it
	// (nodeviews.svelte.ts). Mermaid loads only when the first one draws.
	import Check from '@lucide/svelte/icons/check';
	import Code from '@lucide/svelte/icons/code';
	import Copy from '@lucide/svelte/icons/copy';
	import Download from '@lucide/svelte/icons/download';
	import Maximize2 from '@lucide/svelte/icons/maximize-2';
	import Minus from '@lucide/svelte/icons/minus';
	import Pencil from '@lucide/svelte/icons/pencil';
	import Plus from '@lucide/svelte/icons/plus';
	import Scan from '@lucide/svelte/icons/scan';
	import { onDestroy, tick, untrack } from 'svelte';
	import { announce } from '../../announce.svelte';
	import { copyText } from '../../clipboard';
	import { accessibleText, kindName, mistakeOf, sourceKind } from '../../docs/diagrams';
	import { anchored } from '../../popover';
	import { toasts } from '../../toasts.svelte';
	import { uuid } from '../../uuid';
	import Announcer from '../Announcer.svelte';

	interface Props {
		source: string;
		/** Whether its source shows, the caret being in it. */
		editing: boolean;
		/** Whether the document can be changed here. */
		editable: boolean;
		/** Puts the caret in the source. */
		onedit: () => void;
		/** Leaves the source, the diagram picked. */
		ondone?: () => void;
	}

	let { source, editing, editable, onedit, ondone }: Props = $props();

	let host = $state<HTMLDivElement>();
	/** The last drawing that worked, kept while a mistake is fixed. */
	let svg = $state('');
	let type = $state<string | undefined>(undefined);
	let mistake = $state<{ message: string; line: number | null } | null>(null);
	let drawing = $state(true);
	let drawn = '';
	let timer: ReturnType<typeof setTimeout> | undefined;
	let gone = false;

	const words = $derived(accessibleText(source));
	const kind = $derived(kindName(type ?? sourceKind(source)));
	const name = $derived(words.title || kind);
	const empty = $derived(!source.trim());

	async function redraw(text: string) {
		const { draw } = await import('../../docs/mermaid');
		if (gone) return;
		const font = host ? getComputedStyle(host).fontFamily : '';
		try {
			const d = await draw(text, font);
			if (gone || text !== source) return;
			svg = d.svg;
			type = d.type;
			mistake = null;
		} catch (e) {
			if (gone || text !== source) return;
			mistake = mistakeOf(e);
		} finally {
			if (text === source) drawing = false;
		}
	}

	// Drawn at once the first time, then after typing pauses.
	$effect(() => {
		const text = source;
		untrack(() => {
			if (text === drawn) return;
			const first = drawn === '' && !svg;
			drawn = text;
			clearTimeout(timer);
			if (!text.trim()) {
				svg = '';
				mistake = null;
				drawing = false;
				return;
			}
			drawing = true;
			if (first) void redraw(text);
			else timer = setTimeout(() => void redraw(text), 300);
		});
	});
	onDestroy(() => {
		gone = true;
		clearTimeout(timer);
	});

	// The drawing goes in as Mermaid made it: its output is sanitized (strict
	// security level), and the screen reader hears this block's name instead.
	function place(el: HTMLElement, markup: string) {
		const put = (m: string) => {
			el.innerHTML = m;
			const s = el.querySelector('svg');
			s?.setAttribute('aria-hidden', 'true');
			s?.removeAttribute('tabindex');
			// Mermaid fits it to the block and never past the size it was drawn;
			// a wide one stays big enough to read and scrolls sideways instead.
			const drawnWidth = parseFloat(s?.style.maxWidth ?? '');
			if (s && drawnWidth > 0) s.style.minWidth = `min(${Math.round(drawnWidth)}px, 560px)`;
		};
		put(markup);
		return { update: put };
	}

	async function copySource() {
		const ok = await copyText(source);
		if (ok) announce('Diagram source copied');
		else toasts.error('The source could not be copied.');
	}

	/** The drawing as a file of its own: the theme's colors written in, since a file has no tokens. */
	function standalone(): { markup: string; width: number; height: number } | null {
		const live = host?.querySelector('.drawing svg');
		if (!(live instanceof SVGSVGElement)) return null;
		const copy = live.cloneNode(true) as SVGSVGElement;
		const from = [live, ...live.querySelectorAll('*')];
		const to = [copy, ...copy.querySelectorAll('*')];
		from.forEach((el, i) => {
			const target = to[i] as SVGElement;
			const cs = getComputedStyle(el);
			for (const prop of ['fill', 'stroke', 'color', 'opacity', 'font-family', 'font-size']) {
				const v = cs.getPropertyValue(prop);
				if (v) target.style.setProperty(prop, v);
			}
		});
		copy.querySelectorAll('style').forEach((s) => s.remove());
		copy.removeAttribute('aria-hidden');
		const box = live.viewBox.baseVal;
		const width = Math.ceil(box?.width || live.getBoundingClientRect().width);
		const height = Math.ceil(box?.height || live.getBoundingClientRect().height);
		copy.setAttribute('width', String(width));
		copy.setAttribute('height', String(height));
		copy.style.removeProperty('max-width');
		copy.style.setProperty('background', getComputedStyle(host!).backgroundColor);
		copy.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
		return { markup: new XMLSerializer().serializeToString(copy), width, height };
	}

	function fileName(ext: string) {
		const base = name.replace(/[^\p{L}\p{N} _-]+/gu, '').trim() || 'Diagram';
		return `${base}.${ext}`;
	}

	function save(blob: Blob, file: string) {
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = file;
		a.click();
		setTimeout(() => URL.revokeObjectURL(url), 1000);
	}

	const menuId = `diagram-menu-${uuid()}`;
	const descId = `diagram-desc-${uuid()}`;
	let menu = $state<HTMLDivElement>();
	function download(format: 'svg' | 'png') {
		menu?.hidePopover();
		const s = standalone();
		if (!s) return;
		const blob = new Blob([s.markup], { type: 'image/svg+xml' });
		if (format === 'svg') return save(blob, fileName('svg'));
		const scale = 2;
		const img = new Image();
		const url = URL.createObjectURL(blob);
		img.onload = () => {
			const canvas = document.createElement('canvas');
			canvas.width = s.width * scale;
			canvas.height = s.height * scale;
			const ctx = canvas.getContext('2d');
			ctx?.scale(scale, scale);
			ctx?.drawImage(img, 0, 0, s.width, s.height);
			URL.revokeObjectURL(url);
			canvas.toBlob((png) => {
				if (png) save(png, fileName('png'));
				else toasts.error('The picture could not be made.');
			}, 'image/png');
		};
		img.onerror = () => {
			URL.revokeObjectURL(url);
			toasts.error('The picture could not be made.');
		};
		img.src = url;
	}

	// Full screen: the drawing as large as the window allows, zoomed and moved
	// by the buttons, the wheel, dragging, or plus, minus and the arrow keys.
	let dialog = $state<HTMLDialogElement>();
	let stage = $state<HTMLDivElement>();
	let full = $state(false);
	let zoom = $state(1);
	let pan = $state({ x: 0, y: 0 });
	async function openFull() {
		full = true;
		zoom = 1;
		pan = { x: 0, y: 0 };
		await tick();
		dialog?.showModal();
		stage?.focus();
	}
	function setZoom(z: number) {
		zoom = Math.min(8, Math.max(0.25, Math.round(z * 100) / 100));
		announce(`Zoom ${Math.round(zoom * 100)} percent`);
	}
	function fit() {
		zoom = 1;
		pan = { x: 0, y: 0 };
		announce('Fit to window');
	}
	function stageKeys(e: KeyboardEvent) {
		if (e.ctrlKey || e.metaKey || e.altKey) return;
		const step = e.shiftKey ? 120 : 40;
		const moves: Record<string, [number, number]> = {
			ArrowLeft: [step, 0],
			ArrowRight: [-step, 0],
			ArrowUp: [0, step],
			ArrowDown: [0, -step]
		};
		const m = moves[e.key];
		if (m) {
			e.preventDefault();
			pan = { x: pan.x + m[0], y: pan.y + m[1] };
		} else if (e.key === '+' || e.key === '=') {
			e.preventDefault();
			setZoom(zoom * 1.25);
		} else if (e.key === '-' || e.key === '_') {
			e.preventDefault();
			setZoom(zoom / 1.25);
		} else if (e.key === '0') {
			e.preventDefault();
			fit();
		}
	}
	function wheel(e: WheelEvent) {
		e.preventDefault();
		zoom = Math.min(8, Math.max(0.25, zoom * (e.deltaY < 0 ? 1.1 : 1 / 1.1)));
	}
	let dragFrom: { x: number; y: number; px: number; py: number } | null = null;
	function down(e: PointerEvent) {
		if (e.button !== 0) return;
		dragFrom = { x: e.clientX, y: e.clientY, px: pan.x, py: pan.y };
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}
	function move(e: PointerEvent) {
		if (!dragFrom) return;
		pan = { x: dragFrom.px + e.clientX - dragFrom.x, y: dragFrom.py + e.clientY - dragFrom.y };
	}
	function up() {
		dragFrom = null;
	}
</script>

<div
	class="diagram"
	class:stale={!!mistake && !!svg}
	class:editing
	bind:this={host}
	data-kind={type ?? sourceKind(source) ?? ''}
>
	<div class="bar" role="toolbar" aria-label="{name} tools">
		{#if editable && !editing}
			<button type="button" class="tool" onclick={onedit} title="Edit the diagram's source">
				<Pencil size={15} aria-hidden="true" /><span>Edit</span>
			</button>
		{:else if editable && editing && ondone}
			<button type="button" class="tool" onclick={ondone} title="Close the diagram's source">
				<Check size={15} aria-hidden="true" /><span>Done</span>
			</button>
		{/if}
		<button
			type="button"
			class="tool icon"
			onclick={openFull}
			disabled={!svg}
			aria-label="Full screen"
			title="Show the diagram full screen"
		>
			<Maximize2 size={15} aria-hidden="true" />
		</button>
		<button
			type="button"
			class="tool icon"
			onclick={copySource}
			aria-label="Copy source"
			title="Copy the diagram's source"
		>
			<Copy size={15} aria-hidden="true" />
		</button>
		<button
			type="button"
			class="tool icon"
			disabled={!svg}
			aria-label="Download"
			title="Download the diagram as a picture"
			popovertarget={menuId}
		>
			<Download size={15} aria-hidden="true" />
		</button>
		<div class="menu" popover id={menuId} bind:this={menu} use:anchored>
			<button type="button" class="option" onclick={() => download('svg')}>SVG picture</button>
			<button type="button" class="option" onclick={() => download('png')}>PNG picture</button>
		</div>
	</div>
	{#if empty}
		<div class="placeholder">
			<Code size={18} aria-hidden="true" />
			<span>Empty diagram</span>
		</div>
	{:else if svg}
		<!-- A wide drawing scrolls sideways on a phone, so it takes focus for the arrow keys. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
		<div
			class="drawing"
			tabindex="0"
			role="img"
			aria-label={name}
			aria-describedby={words.description ? descId : undefined}
			aria-busy={drawing}
			use:place={svg}
			ondblclick={() => editable && onedit()}
			onkeydown={(e) => {
				if (e.key === 'Enter' && editable && !editing) {
					e.preventDefault();
					onedit();
				}
			}}
		></div>
	{:else if mistake && !editing}
		<!-- Nothing drawn yet on this device: the source shows, as other apps show it. -->
		<pre class="as-text" aria-label="{name} source">{source}</pre>
	{:else if mistake}
		<div class="placeholder error">
			<span>This diagram could not be drawn.</span>
		</div>
	{:else}
		<div class="placeholder" aria-busy="true"><span>Drawing the diagram</span></div>
	{/if}
	{#if words.description}
		<p class="visually-hidden" id={descId}>{words.description}</p>
	{/if}
	{#if mistake && editing}
		<div class="mistake" role="status">
			<strong>{mistake.line ? `Error on line ${mistake.line}` : 'Diagram error'}</strong>
			<pre>{mistake.message}</pre>
		</div>
	{:else if mistake && !editing}
		<p class="mistake-short" role="status">
			{svg
				? 'The source has an error, so the last drawing shows.'
				: `The source has an error${mistake.line ? ` on line ${mistake.line}` : ''}, so it shows as text.`}
		</p>
	{/if}
</div>

{#if full}
	<dialog
		bind:this={dialog}
		class="full"
		aria-label={name}
		onclose={() => (full = false)}
		onkeydown={(e) => e.key === 'Escape' && e.stopPropagation()}
	>
		<header>
			<h2>{name}</h2>
			<div class="zoom" role="group" aria-label="Zoom">
				<button
					type="button"
					class="tool icon"
					aria-label="Zoom out"
					title="Zoom out"
					onclick={() => setZoom(zoom / 1.25)}><Minus size={16} aria-hidden="true" /></button
				>
				<span class="percent" aria-hidden="true">{Math.round(zoom * 100)}%</span>
				<button
					type="button"
					class="tool icon"
					aria-label="Zoom in"
					title="Zoom in"
					onclick={() => setZoom(zoom * 1.25)}><Plus size={16} aria-hidden="true" /></button
				>
				<button type="button" class="tool" onclick={fit} title="Fit the diagram to the window">
					<Scan size={16} aria-hidden="true" /><span>Fit</span>
				</button>
			</div>
			<form method="dialog"><button class="close">Close</button></form>
		</header>
		<!-- A focusable stage the arrow keys and plus and minus work on, like the picture cropper's. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
		<div
			class="stage"
			bind:this={stage}
			role="img"
			aria-label="{name}. Arrow keys move it; plus and minus zoom."
			aria-describedby={words.description ? descId : undefined}
			tabindex="0"
			onkeydown={stageKeys}
			onwheel={wheel}
			onpointerdown={down}
			onpointermove={move}
			onpointerup={up}
			onpointercancel={up}
		>
			<div
				class="moved"
				style:transform="translate({pan.x}px, {pan.y}px) scale({zoom})"
				use:place={svg}
			></div>
		</div>
		{#if full}<Announcer inDialog />{/if}
	</dialog>
{/if}

<style>
	.diagram {
		position: relative;
		display: grid;
		gap: var(--space-2);
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		font-family: var(--font-page, var(--font-ui));
	}
	.bar {
		position: absolute;
		top: var(--space-1);
		right: var(--space-1);
		z-index: 1;
		display: flex;
		gap: 2px;
		padding: 2px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--menu-bg);
		box-shadow: var(--shadow-sm);
		opacity: 0;
		transition: opacity var(--dur-fade) var(--ease);
	}
	.diagram:hover .bar,
	.diagram:focus-within .bar,
	.diagram.editing .bar {
		opacity: 1;
	}
	/* With no hover to bring them, the buttons stay, above the drawing rather than over it. */
	@media (pointer: coarse) {
		.bar {
			position: static;
			justify-self: end;
			opacity: 1;
		}
	}
	.tool {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-2);
		border: 0;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-family: var(--font-ui);
		cursor: pointer;
	}
	.tool.icon {
		min-width: max(var(--tap-min), 28px);
		justify-content: center;
		padding: 0;
	}
	.tool:hover:not(:disabled) {
		background: var(--button-ghost-hover);
		color: var(--text-primary);
	}
	.tool:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.drawing {
		overflow-x: auto;
		text-align: center;
		border-radius: var(--radius-sm);
	}
	/* Fitted to the block, and no taller than 640 px: Full screen shows the rest. */
	.drawing > :global(svg) {
		display: inline-block;
		height: auto;
		max-height: 640px;
	}
	.as-text {
		margin: 0;
		padding: var(--space-3);
		border-radius: var(--radius-sm);
		background: var(--page-code-bg);
		color: var(--page-code-text);
		font-family: var(--font-mono);
		font-size: var(--text-sm);
		white-space: pre-wrap;
	}
	.stale .drawing {
		opacity: 0.45;
	}
	.placeholder {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
		min-height: 96px;
		color: var(--text-muted);
		font-size: var(--text-sm);
		font-family: var(--font-ui);
	}
	.placeholder.error {
		color: var(--danger);
	}
	.mistake {
		padding: var(--space-2) var(--space-3);
		border-left: 3px solid var(--danger);
		border-radius: var(--radius-sm);
		background: var(--surface-1);
		font-family: var(--font-ui);
		font-size: var(--text-sm);
		color: var(--text-primary);
	}
	.mistake strong {
		color: var(--danger);
	}
	.mistake pre {
		margin: var(--space-1) 0 0;
		padding: 0;
		background: none;
		white-space: pre-wrap;
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.mistake-short {
		margin: 0;
		font-family: var(--font-ui);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}

	.full {
		width: calc(100vw - 32px);
		max-width: none;
		height: calc(100vh - 32px);
		max-height: none;
		padding: 0;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.full[open] {
		display: flex;
		flex-direction: column;
	}
	.full::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	.full header {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-3);
		border-bottom: var(--border-width) solid var(--border);
	}
	.full h2 {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: var(--text-lg);
	}
	.zoom {
		display: flex;
		align-items: center;
		gap: 2px;
	}
	.percent {
		min-width: 4ch;
		text-align: center;
		font-size: var(--text-xs);
		color: var(--text-secondary);
		font-variant-numeric: tabular-nums;
	}
	.close {
		min-height: max(var(--tap-min), 32px);
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--button-secondary-border);
		border-radius: var(--button-radius);
		background: var(--button-secondary-bg);
		color: var(--button-secondary-text);
		cursor: pointer;
	}
	.stage {
		position: relative;
		flex: 1;
		overflow: hidden;
		display: grid;
		place-items: center;
		background: var(--surface-0);
		cursor: grab;
		touch-action: none;
	}
	.stage:active {
		cursor: grabbing;
	}
	.moved {
		transform-origin: center;
		width: min(100%, 1400px);
		padding: var(--space-5);
	}
	.moved :global(svg) {
		width: 100%;
		max-width: none !important;
		height: auto;
		max-height: calc(100vh - 140px);
	}
</style>
