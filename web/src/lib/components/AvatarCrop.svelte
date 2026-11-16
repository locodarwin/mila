<script lang="ts">
	// Cropping a picture to a circle before it is sent: the picture is
	// dragged, pinched, wheeled or moved with the keys behind a round mask,
	// and what the circle's square holds is drawn as a PNG of at most 512
	// pixels. Loaded only when a picture is chosen (ProfilePicture).
	import { onMount, tick } from 'svelte';
	import { announce } from '../announce.svelte';
	import { messageOf } from '../api/client';
	import {
		clampPan,
		coverScale,
		cropRect,
		initialView,
		maxScale,
		outputSide,
		PAN_STEP,
		PAN_STEP_BIG,
		zoomAbout,
		ZOOM_STEP,
		type View
	} from '../avatarCrop';
	import { toasts } from '../toasts.svelte';
	import Announcer from './Announcer.svelte';
	import Button from './Button.svelte';

	interface Props {
		/** The picture chosen, in any format the browser can show. */
		file: Blob;
		/** Sends the cropped PNG; a rejection's message is shown in the dialog. */
		onsave: (png: Blob) => Promise<void>;
		/** The dialog closed, saved or not. */
		onclose: () => void;
	}

	let { file, onsave, onclose }: Props = $props();

	// The stage in its own units; it is drawn at whatever size it has.
	const STAGE = 280;
	const MAX_FILE = 20 * 1024 * 1024;
	const MAX_PIXELS = 50_000_000;
	const hintId = `crop-hint-${Math.random().toString(36).slice(2)}`;

	let dialog = $state<HTMLDialogElement>();
	let canvas = $state<HTMLCanvasElement>();
	let stageEl = $state<HTMLElement>();
	let width = $state(0);
	let bitmap = $state.raw<ImageBitmap | null>(null);
	let view = $state<View>({ scale: 1, x: 0, y: 0 });
	let saving = $state(false);
	let error = $state('');

	const img = $derived(bitmap ? { w: bitmap.width, h: bitmap.height } : null);
	const cover = $derived(img ? coverScale(img.w, img.h, STAGE) : 1);
	const most = $derived(img ? maxScale(img, STAGE) : 1);
	// The zoom slider runs from 0 (just covering) to 100 (the most), evenly
	// in ratio.
	const zoomValue = $derived(
		most > cover ? Math.round((Math.log(view.scale / cover) / Math.log(most / cover)) * 100) : 0
	);

	function fail(message: string) {
		toasts.error(message);
		onclose();
	}

	onMount(() => {
		let gone = false;
		void (async () => {
			if (file.size > MAX_FILE) return fail('That picture is too large. Choose one under 20 MB.');
			let b: ImageBitmap;
			try {
				// Applies the photo's orientation, as the browser shows it.
				b = await createImageBitmap(file);
			} catch {
				return fail("Mila can't read that picture here. Try a JPEG or PNG.");
			}
			if (gone) return b.close();
			if (b.width * b.height > MAX_PIXELS) {
				b.close();
				return fail('That picture is too large. Choose one under 50 megapixels.');
			}
			bitmap = b;
			view = initialView({ w: b.width, h: b.height }, STAGE);
			dialog?.showModal();
			await tick();
			stageEl?.focus();
		})();
		return () => {
			gone = true;
			bitmap?.close();
		};
	});

	// Draws the picture at the stage's size on screen, sharp at any pixel ratio.
	$effect(() => {
		if (!canvas || !bitmap || !width) return;
		const px = Math.round(width * devicePixelRatio);
		if (canvas.width !== px) canvas.width = canvas.height = px;
		const ctx = canvas.getContext('2d');
		if (!ctx) return;
		const k = px / STAGE;
		ctx.setTransform(k, 0, 0, k, 0, 0);
		ctx.imageSmoothingQuality = 'high';
		ctx.clearRect(0, 0, STAGE, STAGE);
		ctx.drawImage(bitmap, view.x, view.y, bitmap.width * view.scale, bitmap.height * view.scale);
	});

	function set(v: View) {
		if (img) view = clampPan(v, img, STAGE);
	}

	const center = { x: STAGE / 2, y: STAGE / 2 };

	function zoomBy(factor: number, at = center) {
		set(zoomAbout(view, factor, at));
	}

	function saidZoom() {
		return `Zoom ${zoomValue} percent`;
	}

	// Pointers: one drags, two pinch. Positions are in stage units.
	// eslint-disable-next-line svelte/prefer-svelte-reactivity -- plain: nothing draws from it
	const pointers = new Map<number, { x: number; y: number }>();

	function toStage(e: PointerEvent | WheelEvent) {
		const r = stageEl!.getBoundingClientRect();
		return {
			x: ((e.clientX - r.left) * STAGE) / r.width,
			y: ((e.clientY - r.top) * STAGE) / r.height
		};
	}

	function down(e: PointerEvent) {
		if (e.pointerType === 'mouse' && e.button !== 0) return;
		stageEl?.setPointerCapture(e.pointerId);
		pointers.set(e.pointerId, toStage(e));
	}

	function move(e: PointerEvent) {
		const was = pointers.get(e.pointerId);
		if (!was) return;
		const now = toStage(e);
		if (pointers.size === 1) {
			set({ ...view, x: view.x + now.x - was.x, y: view.y + now.y - was.y });
		} else if (pointers.size === 2) {
			const other = [...pointers].find(([id]) => id !== e.pointerId)![1];
			const before = Math.hypot(was.x - other.x, was.y - other.y);
			const after = Math.hypot(now.x - other.x, now.y - other.y);
			const mid0 = { x: (was.x + other.x) / 2, y: (was.y + other.y) / 2 };
			const mid1 = { x: (now.x + other.x) / 2, y: (now.y + other.y) / 2 };
			const panned = { ...view, x: view.x + mid1.x - mid0.x, y: view.y + mid1.y - mid0.y };
			set(before > 0 ? zoomAbout(panned, after / before, mid1) : panned);
		}
		pointers.set(e.pointerId, now);
	}

	function up(e: PointerEvent) {
		pointers.delete(e.pointerId);
	}

	// The wheel zooms about the pointer; it has to be a listener that may
	// prevent scrolling the page.
	$effect(() => {
		const el = stageEl;
		if (!el) return;
		const wheel = (e: WheelEvent) => {
			e.preventDefault();
			zoomBy(Math.exp(-e.deltaY * 0.0015), toStage(e));
		};
		el.addEventListener('wheel', wheel, { passive: false });
		return () => el.removeEventListener('wheel', wheel);
	});

	// The arrow keys move the picture the way they point, Shift for bigger
	// steps; plus and minus zoom about the middle.
	function keys(e: KeyboardEvent) {
		if (e.ctrlKey || e.metaKey || e.altKey) return;
		const step = e.shiftKey ? PAN_STEP_BIG : PAN_STEP;
		const moves: Record<string, [number, number, string]> = {
			ArrowLeft: [-step, 0, 'left'],
			ArrowRight: [step, 0, 'right'],
			ArrowUp: [0, -step, 'up'],
			ArrowDown: [0, step, 'down']
		};
		const m = moves[e.key];
		if (m) {
			e.preventDefault();
			const before = view;
			set({ ...view, x: view.x + m[0], y: view.y + m[1] });
			announce(
				before.x === view.x && before.y === view.y
					? `The picture is at its ${m[2] === 'left' ? 'right' : m[2] === 'right' ? 'left' : m[2] === 'up' ? 'bottom' : 'top'} edge`
					: `Moved ${m[2]}`
			);
		} else if (e.key === '+' || e.key === '=') {
			e.preventDefault();
			zoomBy(ZOOM_STEP);
			announce(saidZoom());
		} else if (e.key === '-' || e.key === '_') {
			e.preventDefault();
			zoomBy(1 / ZOOM_STEP);
			announce(saidZoom());
		}
	}

	function slide(e: Event & { currentTarget: HTMLInputElement }) {
		const t = Number(e.currentTarget.value) / 100;
		zoomBy((cover * Math.pow(most / cover, t)) / view.scale);
	}

	async function save() {
		if (!bitmap || !img) return;
		const r = cropRect(view, img, STAGE);
		const side = Math.max(1, outputSide(r.side));
		const out = document.createElement('canvas');
		out.width = out.height = side;
		const ctx = out.getContext('2d');
		if (!ctx) return;
		ctx.imageSmoothingQuality = 'high';
		ctx.drawImage(bitmap, r.sx, r.sy, r.side, r.side, 0, 0, side, side);
		const png = await new Promise<Blob | null>((done) => out.toBlob(done, 'image/png'));
		if (!png) {
			error = "Mila can't read that picture here. Try a JPEG or PNG.";
			return;
		}
		saving = true;
		error = '';
		try {
			await onsave(png);
			dialog?.close();
		} catch (err) {
			error = messageOf(err);
		} finally {
			saving = false;
		}
	}
</script>

<dialog
	bind:this={dialog}
	class="crop"
	aria-labelledby="crop-title"
	oncancel={(e) => {
		// Once sent, the picture is on its way: the dialog waits for it.
		if (saving) e.preventDefault();
	}}
	onclose={() => onclose()}
>
	<h2 id="crop-title">Crop your picture</h2>
	<!-- A focusable stage the arrow keys and plus and minus work on, like a slider in two directions. -->
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div
		class="stage"
		role="application"
		tabindex="0"
		aria-label="Picture position"
		aria-describedby={hintId}
		bind:this={stageEl}
		bind:clientWidth={width}
		onpointerdown={down}
		onpointermove={move}
		onpointerup={up}
		onpointercancel={up}
		onkeydown={keys}
	>
		<canvas bind:this={canvas} aria-hidden="true"></canvas>
		<div class="mask" aria-hidden="true"></div>
	</div>
	<p id={hintId} class="hint">
		Drag to move the picture, and zoom to fit. Arrow keys move the picture; plus and minus zoom.
	</p>
	<label class="zoom">
		<span aria-hidden="true">Zoom</span>
		<input
			type="range"
			aria-label="Zoom"
			aria-valuetext={`${zoomValue} percent`}
			min="0"
			max="100"
			step="1"
			value={zoomValue}
			disabled={most <= cover}
			oninput={slide}
		/>
	</label>
	{#if error}<p class="error" role="alert">{error}</p>{/if}
	<div class="actions">
		<Button variant="ghost" disabled={saving} onclick={() => dialog?.close()}>Cancel</Button>
		<Button variant="primary" disabled={saving || !bitmap} onclick={() => void save()}>
			{saving ? 'Saving…' : 'Save'}
		</Button>
	</div>
	{#if bitmap}<Announcer inDialog />{/if}
</dialog>

<style>
	.crop {
		width: min(calc(340px * var(--ui-scale)), calc(100vw - 32px));
		max-height: 90vh;
		padding: var(--space-5);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--dialog-bg);
		color: var(--text-primary);
		box-shadow: var(--shadow-lg);
	}
	.crop::backdrop {
		/* With the value as written, for browsers whose ::backdrop does not inherit custom properties. */
		background: var(--scrim, oklch(0 0 0 / 0.25));
	}
	h2 {
		font-size: var(--text-xl);
	}
	.stage {
		position: relative;
		width: min(calc(280px * var(--ui-scale)), 100%);
		aspect-ratio: 1;
		margin: var(--space-4) auto 0;
		overflow: hidden;
		border-radius: var(--radius-md);
		background: var(--surface-2);
		cursor: grab;
		touch-action: none;
	}
	.stage:active {
		cursor: grabbing;
	}
	canvas {
		display: block;
		width: 100%;
		height: 100%;
	}
	/* The circle kept; what lies outside it is dimmed in the surface's color. */
	.mask {
		position: absolute;
		inset: 0;
		border: 2px solid var(--surface-0);
		border-radius: 50%;
		box-shadow: 0 0 0 100vmax color-mix(in oklch, var(--surface-0) 60%, transparent);
		pointer-events: none;
	}
	.hint {
		margin-top: var(--space-3);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.zoom {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		margin-top: var(--space-3);
		font-size: var(--text-sm);
	}
	.zoom input {
		flex: 1;
		min-height: max(var(--tap-min), 20px);
		accent-color: var(--accent);
	}
	.error {
		margin-top: var(--space-3);
		font-size: var(--text-sm);
		color: var(--danger);
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
		margin-top: var(--space-4);
	}
	.actions :global(.button) {
		min-height: max(var(--tap-min), calc(32px * var(--control-scale)));
	}

	/* A phone: the whole screen, risen from the bottom. */
	@media (max-width: 720px) {
		.crop {
			width: 100%;
			max-width: none;
			height: 100%;
			max-height: none;
			margin: 0;
			padding: var(--space-4) var(--space-4) calc(var(--space-4) + env(safe-area-inset-bottom));
			border: none;
			border-radius: 0;
		}
		.crop[open] {
			animation: sheet-in var(--dur-panel) var(--ease);
		}
	}
	@keyframes sheet-in {
		from {
			opacity: 0;
			transform: translateY(calc(100% * var(--motion)));
		}
	}
</style>
