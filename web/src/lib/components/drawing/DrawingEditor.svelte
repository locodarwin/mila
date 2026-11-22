<script lang="ts">
	// The drawing editor, full screen over the page: tools along the top,
	// the canvas, and what the picked shapes look like at the side (under
	// the canvas on a phone). Every action is one step of its own history
	// and written as it ends, so others see it at once and two people
	// editing different shapes keep both. Opened from DrawingView; with
	// readonly, it only shows the drawing large.
	import Circle from '@lucide/svelte/icons/circle';
	import Cylinder from '@lucide/svelte/icons/cylinder';
	import Diamond from '@lucide/svelte/icons/diamond';
	import Grid3x3 from '@lucide/svelte/icons/grid-3x3';
	import ImageIcon from '@lucide/svelte/icons/image';
	import Magnet from '@lucide/svelte/icons/magnet';
	import Minus from '@lucide/svelte/icons/minus';
	import MousePointer2 from '@lucide/svelte/icons/mouse-pointer-2';
	import MoveUpRight from '@lucide/svelte/icons/move-up-right';
	import PenLine from '@lucide/svelte/icons/pen-line';
	import Plus from '@lucide/svelte/icons/plus';
	import Redo2 from '@lucide/svelte/icons/redo-2';
	import Scan from '@lucide/svelte/icons/scan';
	import Slash from '@lucide/svelte/icons/slash';
	import Square from '@lucide/svelte/icons/square';
	import StickyNote from '@lucide/svelte/icons/sticky-note';
	import Type from '@lucide/svelte/icons/type';
	import Undo2 from '@lucide/svelte/icons/undo-2';
	import { onMount, tick, type Component } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { announce } from '../../announce.svelte';
	import {
		alignTo,
		boundsOf,
		boxBetween,
		center,
		contains,
		DASHES,
		drawingOf,
		endsOf,
		extentOf,
		FILLS,
		GRID,
		heightFor,
		inOrder,
		isLine,
		kindName,
		lineHeight,
		MAX_POINTS,
		MAX_SHAPES,
		MAX_TEXT,
		nearestSide,
		overlaps,
		ROUTES,
		SNAP_REACH,
		snapTo,
		STROKES,
		shapeLabel,
		shapeSvg,
		textRoom,
		TEXT_SIZES,
		type Box,
		type Point,
		type Shape,
		type ShapeKind,
		type ShapeStyle,
		type Side
	} from '../../drawing';
	import { attachmentOf, livePaint } from '../../drawingPaint';
	import {
		diff,
		DrawingHistory,
		renameDrawing,
		rowOf,
		setDrawingOption,
		shapesOf,
		type Change
	} from '../../sync/drawings.svelte';
	import { engine } from '../../sync/engine.svelte';
	import { keyBetween } from '../../sync/sortkey';
	import { tokenVar } from '../../theme/css';
	import { toasts } from '../../toasts.svelte';
	import { uuid } from '../../uuid';
	import Announcer from '../Announcer.svelte';

	interface Props {
		ws: string;
		pageId: string;
		id: string;
		readonly?: boolean;
		onclose: (changed: boolean) => void;
	}

	let { ws, pageId, id, readonly = false, onclose }: Props = $props();

	let dialog = $state<HTMLDialogElement>();
	let stage = $state<HTMLDivElement>();
	const replica = $derived(engine.replica(ws));
	const drawing = $derived.by(() => {
		const row = replica?.get('page_drawing', id);
		return row ? drawingOf(id, row.values) : null;
	});
	const stored = $derived(replica ? shapesOf(ws, pageId, id) : []);
	/** Shapes a gesture is changing, drawn in place of their rows until it ends. */
	let draft = $state<Map<string, Shape> | null>(null);
	/** A shape being drawn, before it is made. */
	let adding = $state<Shape | null>(null);
	const shapes = $derived.by(() => {
		const list = stored.map((s) => draft?.get(s.id) ?? s);
		return adding ? [...list, adding] : list;
	});
	const byId = $derived(new Map(shapes.map((s) => [s.id, s])));
	const selection = new SvelteSet<string>();
	const picked = $derived(shapes.filter((s) => selection.has(s.id)));
	// The editor is made for one workspace and closed before another.
	// svelte-ignore state_referenced_locally
	const history = new DrawingHistory(ws);

	type Tool = 'select' | ShapeKind;
	let tool = $state<Tool>('select');
	const TOOLS: { id: Tool; label: string; icon: Component }[] = [
		{ id: 'select', label: 'Select', icon: MousePointer2 as never },
		{ id: 'rect', label: 'Box', icon: Square as never },
		{ id: 'ellipse', label: 'Ellipse', icon: Circle as never },
		{ id: 'diamond', label: 'Diamond', icon: Diamond as never },
		{ id: 'cylinder', label: 'Cylinder', icon: Cylinder as never },
		{ id: 'text', label: 'Text', icon: Type as never },
		{ id: 'sticky', label: 'Sticky note', icon: StickyNote as never },
		{ id: 'arrow', label: 'Arrow', icon: MoveUpRight as never },
		{ id: 'line', label: 'Line', icon: Slash as never },
		{ id: 'pen', label: 'Pen', icon: PenLine as never },
		{ id: 'image', label: 'Picture', icon: ImageIcon as never }
	];

	// The canvas: the drawing's pixels, moved and zoomed.
	let view = $state({ x: 0, y: 0, zoom: 1 });
	let font = $state('sans-serif');
	const paint = $derived(livePaint(ws, font));
	const markup = (s: Shape) => shapeSvg(s, byId, paint, true);

	function fit() {
		if (!stage) return;
		const r = stage.getBoundingClientRect();
		const b = boundsOf(stored, 48);
		const zoom = Math.max(0.2, Math.min(1.5, r.width / b.w, r.height / b.h));
		view = {
			zoom,
			x: (r.width - b.w * zoom) / 2 - b.x * zoom,
			y: (r.height - b.h * zoom) / 2 - b.y * zoom
		};
	}
	function zoomBy(f: number, at?: Point) {
		if (!stage) return;
		const r = stage.getBoundingClientRect();
		const [sx, sy] = at ?? [r.width / 2, r.height / 2];
		const zoom = Math.max(0.2, Math.min(4, view.zoom * f));
		const k = zoom / view.zoom;
		view = { zoom, x: sx - (sx - view.x) * k, y: sy - (sy - view.y) * k };
		announce(`Zoom ${Math.round(zoom * 100)} percent`);
	}

	onMount(() => {
		dialog?.showModal();
		if (stage)
			font = getComputedStyle(stage).getPropertyValue('--font-page').trim() || 'sans-serif';
		// The canvas, not the name, so a phone's keyboard stays down.
		stage?.focus();
		void tick().then(fit);
	});

	function close() {
		if (dialog?.open) dialog.close();
	}

	/** Where the pointer is, in the drawing's pixels. */
	function at(e: { clientX: number; clientY: number }): Point {
		const r = stage!.getBoundingClientRect();
		return [(e.clientX - r.left - view.x) / view.zoom, (e.clientY - r.top - view.y) / view.zoom];
	}

	/**
	 * The shape under the pointer. The canvas holds the pointer while a
	 * gesture lasts, so the event's own target is the canvas.
	 */
	function shapeUnder(e: { clientX: number; clientY: number }): string | undefined {
		return (
			document.elementFromPoint(e.clientX, e.clientY)?.closest<SVGElement>('[data-shape]')?.dataset
				.shape ?? undefined
		);
	}

	/** The topmost shape a line may join under a point. */
	function joinableAt(p: Point, except?: string): Shape | undefined {
		const reach = 8 / view.zoom;
		for (const s of [...inOrder(shapes)].reverse()) {
			if (s.id === except || isLine(s.kind)) continue;
			const b = s.box;
			if (contains({ x: b.x - reach, y: b.y - reach, w: b.w + reach * 2, h: b.h + reach * 2 }, p))
				return s;
		}
		return undefined;
	}

	const nextKey = () => keyBetween(stored.at(-1)?.sortKey ?? null, null);

	async function make(shape: Shape) {
		if (stored.length >= MAX_SHAPES) {
			toasts.error(`A drawing holds ${MAX_SHAPES.toLocaleString('en-US')} shapes at most.`);
			return;
		}
		await history.apply([{ id: shape.id, before: null, after: rowOf(shape, pageId) }]);
		selection.clear();
		selection.add(shape.id);
		announce(`Added ${shapeLabel(shape, byId).toLowerCase()}`);
	}

	async function commit(before: Shape[], after: Shape[], said?: string) {
		const changes = after
			.map((a) => {
				const b = before.find((x) => x.id === a.id);
				return b ? diff(b, a, pageId) : null;
			})
			.filter((c): c is Change => !!c);
		await history.apply(changes);
		if (said && changes.length) announce(said);
	}

	function blank(kind: ShapeKind, b: Box, extra: Partial<Shape> = {}): Shape {
		return {
			id: uuid(),
			drawingId: id,
			kind,
			box: b,
			points: null,
			text: '',
			style: {},
			fromId: null,
			toId: null,
			imageId: null,
			sortKey: nextKey(),
			...extra
		};
	}

	const DEFAULT_SIZE: Partial<Record<ShapeKind, [number, number]>> = {
		rect: [160, 72],
		ellipse: [160, 80],
		diamond: [160, 112],
		cylinder: [128, 112],
		sticky: [160, 160],
		text: [160, 40]
	};

	// Gestures: one at a time, from pointer down to up.
	type Gesture =
		| { type: 'move'; start: Point; origin: Shape[]; moved: boolean; freely: boolean }
		| { type: 'resize'; handle: string; origin: Shape; start: Point }
		| { type: 'end'; which: 'a' | 'b'; origin: Shape }
		| { type: 'create'; kind: ShapeKind; start: Point }
		| { type: 'line'; kind: ShapeKind; start: Point; from: Shape | undefined }
		| { type: 'pen'; points: Point[] }
		| { type: 'marquee'; start: Point; base: string[] }
		| { type: 'pan'; start: Point; origin: { x: number; y: number } };
	let gesture: Gesture | null = null;
	let marquee = $state<Box | null>(null);
	let guides = $state<{ x?: number; y?: number }[]>([]);
	let hover = $state<string | null>(null);

	const snapping = $derived(!!drawing?.snap);

	/** A box moved by a step, snapped to the grid and lined up with the other shapes. */
	function snapBox(b: Box, freely: boolean, others: Box[]): Box {
		if (freely || !snapping) {
			guides = [];
			return b;
		}
		const g = { ...b, x: snapTo(b.x), y: snapTo(b.y) };
		const a = alignTo(g, others, SNAP_REACH / view.zoom);
		guides = a.guides;
		return { ...g, x: g.x + a.dx, y: g.y + a.dy };
	}

	function onpointerdown(e: PointerEvent) {
		if (!stage || editing) return;
		const p = at(e);
		const target = e.target as Element;
		if (e.button === 1 || (e.button === 0 && spaceDown)) {
			gesture = { type: 'pan', start: [e.clientX, e.clientY], origin: { x: view.x, y: view.y } };
			stage.setPointerCapture(e.pointerId);
			return;
		}
		if (e.button !== 0) return;
		if (readonly) {
			gesture = { type: 'pan', start: [e.clientX, e.clientY], origin: { x: view.x, y: view.y } };
			stage.setPointerCapture(e.pointerId);
			return;
		}
		stage.setPointerCapture(e.pointerId);
		const handle = target.closest<SVGElement>('[data-handle]');
		const shapeEl = target.closest<SVGElement>('[data-shape]');
		const shape = shapeEl ? byId.get(shapeEl.dataset.shape!) : undefined;
		if (handle && picked.length === 1) {
			const h = handle.dataset.handle!;
			const origin = picked[0];
			gesture =
				h === 'a' || h === 'b'
					? { type: 'end', which: h, origin }
					: { type: 'resize', handle: h, origin, start: p };
			return;
		}
		if (tool === 'select') {
			if (shape) {
				if (e.shiftKey) {
					if (selection.has(shape.id)) selection.delete(shape.id);
					else selection.add(shape.id);
				} else if (!selection.has(shape.id)) {
					selection.clear();
					selection.add(shape.id);
				}
				gesture = {
					type: 'move',
					start: p,
					origin: shapes.filter((s) => selection.has(s.id)),
					moved: false,
					freely: e.altKey
				};
			} else {
				if (!e.shiftKey) selection.clear();
				gesture = { type: 'marquee', start: p, base: [...selection] };
			}
			return;
		}
		if (tool === 'arrow' || tool === 'line') {
			const from = joinableAt(p);
			gesture = { type: 'line', kind: tool, start: p, from };
			return;
		}
		if (tool === 'pen') {
			gesture = { type: 'pen', points: [p] };
			return;
		}
		if (tool === 'image') {
			pictureAt = p;
			choosingPicture = true;
			return;
		}
		gesture = { type: 'create', kind: tool, start: snapping ? [snapTo(p[0]), snapTo(p[1])] : p };
	}

	function onpointermove(e: PointerEvent) {
		const p = at(e);
		if (!gesture) {
			if (tool === 'arrow' || tool === 'line') hover = joinableAt(p)?.id ?? null;
			return;
		}
		const g = gesture;
		switch (g.type) {
			case 'pan':
				view = {
					...view,
					x: g.origin.x + e.clientX - g.start[0],
					y: g.origin.y + e.clientY - g.start[1]
				};
				return;
			case 'move': {
				const dx = p[0] - g.start[0];
				const dy = p[1] - g.start[1];
				if (!g.moved && Math.hypot(dx, dy) * view.zoom < 3) return;
				g.moved = true;
				const lead = g.origin.find((s) => !isLine(s.kind)) ?? g.origin[0];
				const moving = new Set(g.origin.map((s) => s.id));
				const others = stored.filter((s) => !moving.has(s.id) && !isLine(s.kind)).map((s) => s.box);
				const target = snapBox(
					{ ...lead.box, x: lead.box.x + dx, y: lead.box.y + dy },
					g.freely || e.altKey,
					others
				);
				const ddx = target.x - lead.box.x;
				const ddy = target.y - lead.box.y;
				draft = new Map(
					g.origin.map((s) => [
						s.id,
						{ ...s, box: { ...s.box, x: s.box.x + ddx, y: s.box.y + ddy } }
					])
				);
				return;
			}
			case 'resize': {
				const o = g.origin.box;
				let { x, y, w, h } = o;
				const dx = p[0] - g.start[0];
				const dy = p[1] - g.start[1];
				if (g.handle.includes('e')) w = o.w + dx;
				if (g.handle.includes('s')) h = o.h + dy;
				if (g.handle.includes('w')) {
					x = o.x + dx;
					w = o.w - dx;
				}
				if (g.handle.includes('n')) {
					y = o.y + dy;
					h = o.h - dy;
				}
				if (snapping && !e.altKey) {
					const x1 = snapTo(x + w);
					const y1 = snapTo(y + h);
					if (g.handle.includes('w')) x = snapTo(x);
					if (g.handle.includes('n')) y = snapTo(y);
					w = x1 - x;
					h = y1 - y;
				}
				const min = 16;
				const next: Box = { x, y, w: Math.max(min, w), h: Math.max(min, h) };
				draft = new Map([[g.origin.id, { ...g.origin, box: next }]]);
				return;
			}
			case 'end': {
				const s = g.origin;
				const ends = endsOf(s, byId);
				const join = joinableAt(p);
				hover = join?.id ?? null;
				const a = g.which === 'a' ? p : ends.a;
				const b = g.which === 'b' ? p : ends.b;
				draft = new Map([
					[
						s.id,
						lineThrough(s, a, b, g.which === 'a' ? null : s.fromId, g.which === 'b' ? null : s.toId)
					]
				]);
				return;
			}
			case 'create': {
				const q: Point = snapping && !e.altKey ? [snapTo(p[0]), snapTo(p[1])] : p;
				adding = blank(g.kind, boxBetween(g.start, q));
				return;
			}
			case 'line': {
				const join = joinableAt(p, g.from?.id);
				hover = join?.id ?? null;
				const start = g.from ? center(g.from.box) : g.start;
				adding = lineThrough(
					blank(g.kind, { x: 0, y: 0, w: 0, h: 0 }, { style: g.kind === 'arrow' ? {} : {} }),
					start,
					p,
					g.from?.id ?? null,
					null
				);
				return;
			}
			case 'pen': {
				const last = g.points.at(-1)!;
				if (
					Math.hypot(p[0] - last[0], p[1] - last[1]) * view.zoom < 2 ||
					g.points.length >= MAX_POINTS
				)
					return;
				g.points.push(p);
				adding = penThrough(g.points);
				return;
			}
			case 'marquee': {
				marquee = boxBetween(g.start, p);
				selection.clear();
				for (const sid of g.base) selection.add(sid);
				for (const s of stored) if (overlaps(marquee, extentOf(s, byId))) selection.add(s.id);
				return;
			}
		}
	}

	/** A line between two points, joined to the shapes given. */
	function lineThrough(
		s: Shape,
		a: Point,
		b: Point,
		fromId: string | null,
		toId: string | null
	): Shape {
		const x = Math.min(a[0], b[0]);
		const y = Math.min(a[1], b[1]);
		return {
			...s,
			box: { x, y, w: Math.abs(b[0] - a[0]), h: Math.abs(b[1] - a[1]) },
			points: [
				[a[0] - x, a[1] - y],
				[b[0] - x, b[1] - y]
			],
			fromId,
			toId
		};
	}

	function penThrough(points: Point[]): Shape {
		const xs = points.map((q) => q[0]);
		const ys = points.map((q) => q[1]);
		const x = Math.min(...xs);
		const y = Math.min(...ys);
		const b = { x, y, w: Math.max(...xs) - x, h: Math.max(...ys) - y };
		const base = adding?.kind === 'pen' ? adding : blank('pen', b);
		return { ...base, box: b, points: points.map((q) => [q[0] - x, q[1] - y]) };
	}

	async function onpointerup(e: PointerEvent) {
		const g = gesture;
		gesture = null;
		guides = [];
		hover = null;
		if (!g) return;
		const p = at(e);
		switch (g.type) {
			case 'move': {
				const after = draft ? [...draft.values()] : [];
				draft = null;
				if (g.moved)
					await commit(
						g.origin,
						after,
						`Moved ${after.length === 1 ? shapeLabel(after[0], byId) : `${after.length} shapes`}`
					);
				else if (!e.shiftKey && g.origin.length > 1) {
					const sid = shapeUnder(e);
					if (sid) {
						selection.clear();
						selection.add(sid);
					}
				}
				return;
			}
			case 'resize': {
				const after = draft?.get(g.origin.id);
				draft = null;
				if (after)
					await commit(
						[g.origin],
						[{ ...after, box: { ...after.box, h: heightFor(after, paint.measure) } }],
						`Resized ${shapeLabel(after, byId)}`
					);
				return;
			}
			case 'end': {
				const s = draft?.get(g.origin.id);
				draft = null;
				if (!s) return;
				const join = joinableAt(
					p,
					g.which === 'a' ? (s.toId ?? undefined) : (s.fromId ?? undefined)
				);
				const ends = endsOf(s, byId);
				let next = lineThrough(s, ends.a, ends.b, s.fromId, s.toId);
				const style: ShapeStyle = { ...s.style };
				if (join) {
					const side = nearestSide(join.box, p);
					if (g.which === 'a') {
						next = { ...next, fromId: join.id };
						style.fromSide = side;
					} else {
						next = { ...next, toId: join.id };
						style.toSide = side;
					}
				} else if (g.which === 'a') delete style.fromSide;
				else delete style.toSide;
				await commit(
					[g.origin],
					[{ ...next, style }],
					`Moved the end of ${shapeLabel(next, byId)}`
				);
				return;
			}
			case 'create': {
				const a = adding;
				adding = null;
				const size = DEFAULT_SIZE[g.kind] ?? [160, 72];
				const small = !a || (a.box.w < 12 && a.box.h < 12);
				const b: Box = small
					? { x: g.start[0] - size[0] / 2, y: g.start[1] - size[1] / 2, w: size[0], h: size[1] }
					: a!.box;
				const shape = blank(g.kind, snapping ? { ...b, x: snapTo(b.x), y: snapTo(b.y) } : b);
				await make(shape);
				tool = 'select';
				if (g.kind === 'text' || g.kind === 'sticky') await startEditing(shape.id);
				return;
			}
			case 'line': {
				const a = adding;
				adding = null;
				const to = joinableAt(p, g.from?.id);
				if (!a || (!to && Math.hypot(p[0] - g.start[0], p[1] - g.start[1]) * view.zoom < 8)) return;
				const ends = endsOf(a, byId);
				const style: ShapeStyle = {};
				if (to) style.toSide = nearestSide(to.box, p);
				const line = {
					...lineThrough(
						a,
						g.from ? ends.a : g.start,
						to ? center(to.box) : p,
						g.from?.id ?? null,
						to?.id ?? null
					),
					style
				};
				await make(line);
				tool = 'select';
				return;
			}
			case 'pen': {
				const a = adding;
				adding = null;
				if (a && (a.points?.length ?? 0) > 1) await make(a);
				return;
			}
			case 'marquee':
				marquee = null;
				if (selection.size)
					announce(`${selection.size} ${selection.size === 1 ? 'shape' : 'shapes'} picked`);
				return;
			case 'pan':
				return;
		}
	}

	function onwheel(e: WheelEvent) {
		e.preventDefault();
		if (e.ctrlKey || e.metaKey) {
			const r = stage!.getBoundingClientRect();
			zoomBy(e.deltaY < 0 ? 1.1 : 1 / 1.1, [e.clientX - r.left, e.clientY - r.top]);
		} else view = { ...view, x: view.x - e.deltaX, y: view.y - e.deltaY };
	}

	// Text in a shape: a field over it while it is written.
	let editing = $state<{ id: string; value: string } | null>(null);
	let field = $state<HTMLTextAreaElement>();
	async function startEditing(sid: string) {
		await tick();
		const s = byId.get(sid);
		if (!s || readonly || s.kind === 'pen' || s.kind === 'image') return;
		editing = { id: sid, value: s.text };
		await tick();
		field?.focus();
		field?.select();
	}
	async function stopEditing(keep: boolean) {
		const e = editing;
		editing = null;
		if (!e) return;
		const s = byId.get(e.id);
		focusShape(e.id);
		if (!keep || !s || s.text === e.value) return;
		const next = { ...s, text: e.value.slice(0, MAX_TEXT) };
		next.box = { ...next.box, h: heightFor(next, paint.measure) };
		await commit([s], [next], `Changed the words of ${shapeLabel(next, byId)}`);
	}
	const editBox = $derived.by(() => {
		if (!editing) return null;
		const s = byId.get(editing.id);
		if (!s) return null;
		let room = textRoom(s);
		if (isLine(s.kind)) {
			const ends = endsOf(s, byId);
			const m: Point = [(ends.a[0] + ends.b[0]) / 2, (ends.a[1] + ends.b[1]) / 2];
			room = { x: m[0] - 100, y: m[1] - 20, w: 200, h: 40 };
		}
		const size = TEXT_SIZES[s.style.size ?? (isLine(s.kind) ? 's' : 'm')];
		return {
			left: view.x + room.x * view.zoom,
			top: view.y + room.y * view.zoom,
			width: room.w * view.zoom,
			height: Math.max(room.h, lineHeight(size) * 2) * view.zoom,
			size: size * view.zoom,
			bold: !!s.style.bold,
			italic: !!s.style.italic
		};
	});

	// Changes to what is picked, from the side panel and the keys.
	async function restyle(change: Partial<ShapeStyle>, words: string) {
		const before = picked;
		const after = before.map((s) => {
			const style = { ...s.style, ...change };
			for (const k of Object.keys(style) as (keyof ShapeStyle)[])
				if (style[k] === undefined) delete style[k];
			const next = { ...s, style };
			return isLine(s.kind)
				? next
				: { ...next, box: { ...next.box, h: heightFor(next, paint.measure) } };
		});
		await commit(before, after, words);
	}

	async function removePicked() {
		if (!picked.length) return;
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- plain: local to this deletion
		const gone = new Set(picked.map((s) => s.id));
		// Lines on a shape that goes go with it.
		for (const s of stored)
			if ((s.fromId && gone.has(s.fromId)) || (s.toId && gone.has(s.toId))) gone.add(s.id);
		const now = new Date().toISOString();
		await history.apply(
			[...gone].map((sid) => ({
				id: sid,
				before: { deleted_at: null },
				after: { deleted_at: now }
			}))
		);
		selection.clear();
		announce(`Deleted ${gone.size} ${gone.size === 1 ? 'shape' : 'shapes'}`);
		stage?.focus();
	}

	async function nudge(dx: number, dy: number) {
		const before = picked;
		if (!before.length) return;
		await commit(
			before,
			before.map((s) => ({ ...s, box: { ...s.box, x: s.box.x + dx, y: s.box.y + dy } })),
			`Moved ${before.length === 1 ? shapeLabel(before[0], byId) : `${before.length} shapes`}`
		);
	}

	async function duplicatePicked() {
		const source = picked.filter((s) => !isLine(s.kind) || (!s.fromId && !s.toId));
		if (!source.length) return;
		let key = stored.at(-1)?.sortKey ?? null;
		const copies = source.map((s) => {
			key = keyBetween(key, null);
			return {
				...s,
				id: uuid(),
				sortKey: key,
				box: { ...s.box, x: s.box.x + GRID * 2, y: s.box.y + GRID * 2 }
			};
		});
		if (stored.length + copies.length > MAX_SHAPES) {
			toasts.error(`A drawing holds ${MAX_SHAPES.toLocaleString('en-US')} shapes at most.`);
			return;
		}
		await history.apply(copies.map((c) => ({ id: c.id, before: null, after: rowOf(c, pageId) })));
		selection.clear();
		for (const c of copies) selection.add(c.id);
		announce(`Duplicated ${copies.length} ${copies.length === 1 ? 'shape' : 'shapes'}`);
	}

	async function order(front: boolean) {
		const before = inOrder(picked);
		if (!before.length) return;
		const rest = stored.filter((s) => !selection.has(s.id));
		let key = front ? (rest.at(-1)?.sortKey ?? null) : null;
		const limit = front ? null : (rest[0]?.sortKey ?? null);
		const after = before.map((s) => {
			key = keyBetween(key, limit);
			return { ...s, sortKey: key };
		});
		await commit(before, after, front ? 'Brought to the front' : 'Sent to the back');
	}

	async function align(how: 'left' | 'center' | 'right' | 'top' | 'middle' | 'bottom') {
		const boxes = picked.filter((s) => !isLine(s.kind));
		if (boxes.length < 2) return;
		const x0 = Math.min(...boxes.map((s) => s.box.x));
		const x1 = Math.max(...boxes.map((s) => s.box.x + s.box.w));
		const y0 = Math.min(...boxes.map((s) => s.box.y));
		const y1 = Math.max(...boxes.map((s) => s.box.y + s.box.h));
		const after = boxes.map((s) => {
			const b = { ...s.box };
			if (how === 'left') b.x = x0;
			if (how === 'right') b.x = x1 - b.w;
			if (how === 'center') b.x = (x0 + x1) / 2 - b.w / 2;
			if (how === 'top') b.y = y0;
			if (how === 'bottom') b.y = y1 - b.h;
			if (how === 'middle') b.y = (y0 + y1) / 2 - b.h / 2;
			return { ...s, box: b };
		});
		await commit(boxes, after, `Lined up ${boxes.length} shapes`);
	}

	async function distribute(across: boolean) {
		const boxes = [...picked.filter((s) => !isLine(s.kind))].sort((a, b) =>
			across ? a.box.x - b.box.x : a.box.y - b.box.y
		);
		if (boxes.length < 3) return;
		const first = boxes[0].box;
		const last = boxes.at(-1)!.box;
		const total = boxes.reduce((n, s) => n + (across ? s.box.w : s.box.h), 0);
		const span = across ? last.x + last.w - first.x : last.y + last.h - first.y;
		const gap = (span - total) / (boxes.length - 1);
		let pos = across ? first.x : first.y;
		const after = boxes.map((s) => {
			const b = { ...s.box };
			if (across) b.x = pos;
			else b.y = pos;
			pos += (across ? b.w : b.h) + gap;
			return { ...s, box: b };
		});
		await commit(boxes, after, `Spaced out ${boxes.length} shapes`);
	}

	// The keys, on the canvas and its shapes; fields keep their own.
	let spaceDown = false;
	async function onkeydown(e: KeyboardEvent) {
		const t = e.target as HTMLElement;
		// The editor sits inside the page's editable text, which is not a field of its own.
		const own = t.closest('input, textarea, select, [contenteditable="true"]');
		if (own && dialog?.contains(own)) return;
		const mod = e.ctrlKey || e.metaKey;
		if (e.key === ' ' && !readonly && t.closest('.stage')) {
			spaceDown = true;
			e.preventDefault();
			return;
		}
		if (e.key === 'Escape') {
			if (selection.size || tool !== 'select') {
				e.preventDefault();
				selection.clear();
				tool = 'select';
				stage?.focus();
				announce('Nothing picked');
			}
			return;
		}
		if (readonly) return;
		if (mod && e.key.toLowerCase() === 'z') {
			e.preventDefault();
			e.stopPropagation();
			if (e.shiftKey ? await history.redo() : await history.undo())
				announce(e.shiftKey ? 'Redone' : 'Undone');
			return;
		}
		if (mod && e.key.toLowerCase() === 'y') {
			e.preventDefault();
			e.stopPropagation();
			if (await history.redo()) announce('Redone');
			return;
		}
		if (mod && e.key.toLowerCase() === 'a') {
			e.preventDefault();
			for (const s of stored) selection.add(s.id);
			announce(`${selection.size} shapes picked`);
			return;
		}
		if (mod && e.key.toLowerCase() === 'd') {
			e.preventDefault();
			await duplicatePicked();
			return;
		}
		if (!picked.length || mod || e.altKey) return;
		const step = e.shiftKey ? 1 : GRID;
		const moves: Record<string, Point> = {
			ArrowLeft: [-step, 0],
			ArrowRight: [step, 0],
			ArrowUp: [0, -step],
			ArrowDown: [0, step]
		};
		if (moves[e.key]) {
			e.preventDefault();
			await nudge(...moves[e.key]);
		} else if (e.key === 'Delete' || e.key === 'Backspace') {
			e.preventDefault();
			await removePicked();
		} else if (e.key === 'Enter' && picked.length === 1) {
			e.preventDefault();
			await startEditing(picked[0].id);
		}
	}
	function onkeyup(e: KeyboardEvent) {
		if (e.key === ' ') spaceDown = false;
	}

	function focusShape(sid: string) {
		void tick().then(() => stage?.querySelector<SVGElement>(`[data-shape="${sid}"]`)?.focus());
	}
	function onfocusShape(sid: string) {
		if (gesture) return;
		if (!selection.has(sid)) {
			selection.clear();
			selection.add(sid);
		}
	}

	// A picture from the page's files, or a new one.
	let choosingPicture = $state(false);
	let pictureAt: Point = [0, 0];
	let Picker = $state<Component<Record<string, unknown>> | null>(null);
	$effect(() => {
		if (choosingPicture && !Picker)
			void import('../doc/ImagePicker.svelte').then((m) => (Picker = m.default as never));
	});
	function placePicture(src: string) {
		const file = attachmentOf(src);
		choosingPicture = false;
		tool = 'select';
		if (!file) return;
		const img = new Image();
		const put = (w: number, h: number) => {
			const k = Math.min(1, 320 / Math.max(w, h, 1));
			const b = {
				x: pictureAt[0] - (w * k) / 2,
				y: pictureAt[1] - (h * k) / 2,
				w: w * k,
				h: h * k
			};
			void make(
				blank('image', snapping ? { ...b, x: snapTo(b.x), y: snapTo(b.y) } : b, { imageId: file })
			);
		};
		img.onload = () => put(img.naturalWidth || 240, img.naturalHeight || 160);
		img.onerror = () => put(240, 160);
		img.src = src;
	}

	// The handles of what is picked.
	const single = $derived(picked.length === 1 ? picked[0] : null);
	const pickedBox = $derived.by(() => {
		if (!picked.length) return null;
		return boundsOf(picked, 0);
	});
	const HANDLES = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w'];
	function handleAt(b: Box, h: string): Point {
		const x = h.includes('w') ? b.x : h.includes('e') ? b.x + b.w : b.x + b.w / 2;
		const y = h.includes('n') ? b.y : h.includes('s') ? b.y + b.h : b.y + b.h / 2;
		return [x, y];
	}
	const lineEnds = $derived(
		single && (single.kind === 'arrow' || single.kind === 'line') ? endsOf(single, byId) : null
	);
	const hoverShape = $derived(hover ? byId.get(hover) : undefined);
	const SIDE_LIST: Side[] = ['top', 'right', 'bottom', 'left'];

	function svgMarkup(el: SVGGElement, html: string) {
		el.innerHTML = html;
		return { update: (h: string) => (el.innerHTML = h) };
	}

	const name = $derived(drawing?.name ?? '');
	let nameField = $derived(name);
	function saveName() {
		if (nameField.trim() !== name) void renameDrawing(ws, id, nameField);
	}

	const boxKinds = $derived(picked.filter((s) => !isLine(s.kind) && s.kind !== 'image'));
	const lineKinds = $derived(picked.filter((s) => s.kind === 'arrow' || s.kind === 'line'));
	const common = <K extends keyof ShapeStyle>(
		list: Shape[],
		k: K
	): ShapeStyle[K] | 'mixed' | undefined => {
		const vals = new Set(list.map((s) => s.style[k]));
		return vals.size === 1 ? [...vals][0] : vals.size ? 'mixed' : undefined;
	};
</script>

<dialog
	bind:this={dialog}
	class="editor"
	aria-label={readonly ? `${name || 'Drawing'}` : `Edit ${name || 'drawing'}`}
	onclose={() => onclose(history.changed)}
	{onkeydown}
	{onkeyup}
>
	<header>
		{#if readonly}
			<h2>{name || 'Drawing'}</h2>
		{:else}
			<label class="name">
				<span class="visually-hidden">Drawing name</span>
				<input
					type="text"
					bind:value={nameField}
					maxlength="200"
					placeholder="Untitled drawing"
					onblur={saveName}
					onkeydown={(e) => e.key === 'Enter' && (e.currentTarget as HTMLInputElement).blur()}
				/>
			</label>
			<div class="tools" role="toolbar" aria-label="Tools">
				{#each TOOLS as t (t.id)}
					<button
						type="button"
						class="tool icon"
						class:on={tool === t.id}
						aria-pressed={tool === t.id}
						aria-label={t.label}
						title={t.label}
						onclick={() => {
							tool = t.id;
							if (t.id !== 'select') selection.clear();
							announce(t.label);
						}}><t.icon size={17} aria-hidden="true" /></button
					>
				{/each}
			</div>
			<div class="group" role="group" aria-label="History">
				<button
					type="button"
					class="tool icon"
					disabled={!history.canUndo}
					aria-label="Undo"
					title="Undo"
					onclick={() => history.undo().then((d) => d && announce('Undone'))}
					><Undo2 size={17} aria-hidden="true" /></button
				>
				<button
					type="button"
					class="tool icon"
					disabled={!history.canRedo}
					aria-label="Redo"
					title="Redo"
					onclick={() => history.redo().then((d) => d && announce('Redone'))}
					><Redo2 size={17} aria-hidden="true" /></button
				>
			</div>
		{/if}
		<div class="group" role="group" aria-label="Zoom">
			<button
				type="button"
				class="tool icon"
				aria-label="Zoom out"
				title="Zoom out"
				onclick={() => zoomBy(1 / 1.25)}><Minus size={16} aria-hidden="true" /></button
			>
			<span class="percent" aria-hidden="true">{Math.round(view.zoom * 100)}%</span>
			<button
				type="button"
				class="tool icon"
				aria-label="Zoom in"
				title="Zoom in"
				onclick={() => zoomBy(1.25)}><Plus size={16} aria-hidden="true" /></button
			>
			<button type="button" class="tool" title="Fit the drawing to the window" onclick={fit}
				><Scan size={16} aria-hidden="true" /><span>Fit</span></button
			>
		</div>
		{#if !readonly && drawing}
			<div class="group" role="group" aria-label="Grid">
				<button
					type="button"
					class="tool icon"
					class:on={drawing.grid}
					aria-pressed={drawing.grid}
					aria-label="Show the grid"
					title="Show the grid"
					onclick={() => setDrawingOption(ws, id, 'grid', !drawing.grid)}
					><Grid3x3 size={16} aria-hidden="true" /></button
				>
				<button
					type="button"
					class="tool icon"
					class:on={drawing.snap}
					aria-pressed={drawing.snap}
					aria-label="Snap to the grid and shapes"
					title="Snap to the grid and shapes"
					onclick={() => setDrawingOption(ws, id, 'snap', !drawing.snap)}
					><Magnet size={16} aria-hidden="true" /></button
				>
			</div>
		{/if}
		<button type="button" class="done" onclick={close}>{readonly ? 'Close' : 'Done'}</button>
	</header>

	<div class="body">
		<!-- The canvas: a focusable surface the keys work on, with each shape a stop of its own. -->
		<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
		<div
			class="stage"
			class:drawing-tool={tool !== 'select'}
			class:panning={readonly}
			bind:this={stage}
			tabindex="0"
			role="application"
			aria-label={readonly
				? `${name || 'Drawing'}, ${stored.length} shapes`
				: 'Canvas. Tab moves between shapes; arrow keys move them.'}
			{onpointerdown}
			{onpointermove}
			{onpointerup}
			onpointercancel={onpointerup}
			{onwheel}
			ondblclick={(e) => {
				const sid = shapeUnder(e);
				if (sid) void startEditing(sid);
			}}
		>
			<svg class="canvas" aria-hidden={readonly ? undefined : undefined}>
				<defs>
					<pattern
						id="drawing-dots-{id}"
						width={GRID * 2}
						height={GRID * 2}
						patternUnits="userSpaceOnUse"
					>
						<circle cx="1" cy="1" r="1" fill={tokenVar('border-strong')} />
					</pattern>
				</defs>
				<g transform="translate({view.x} {view.y}) scale({view.zoom})">
					{#if drawing?.grid && !readonly}
						<rect
							x="-100000"
							y="-100000"
							width="200000"
							height="200000"
							fill="url(#drawing-dots-{id})"
						/>
					{/if}
					{#each inOrder(shapes) as s (s.id)}
						<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
						<g
							class="shape"
							class:picked={selection.has(s.id)}
							data-shape={s.id}
							tabindex={readonly || s === adding ? -1 : 0}
							role={readonly ? 'img' : 'button'}
							aria-pressed={readonly ? undefined : selection.has(s.id)}
							aria-label={shapeLabel(s, byId)}
							onfocus={() => onfocusShape(s.id)}
							use:svgMarkup={markup(s)}
						></g>
					{/each}
					{#if hoverShape}
						{#each SIDE_LIST as side (side)}
							{@const pt =
								side === 'top'
									? [hoverShape.box.x + hoverShape.box.w / 2, hoverShape.box.y]
									: side === 'bottom'
										? [hoverShape.box.x + hoverShape.box.w / 2, hoverShape.box.y + hoverShape.box.h]
										: side === 'left'
											? [hoverShape.box.x, hoverShape.box.y + hoverShape.box.h / 2]
											: [
													hoverShape.box.x + hoverShape.box.w,
													hoverShape.box.y + hoverShape.box.h / 2
												]}
							<circle class="joint" cx={pt[0]} cy={pt[1]} r={5 / view.zoom} />
						{/each}
						<rect
							class="target"
							x={hoverShape.box.x}
							y={hoverShape.box.y}
							width={hoverShape.box.w}
							height={hoverShape.box.h}
							stroke-width={2 / view.zoom}
						/>
					{/if}
					{#if !readonly && pickedBox}
						{#if lineEnds}
							<circle
								class="handle"
								data-handle="a"
								cx={lineEnds.a[0]}
								cy={lineEnds.a[1]}
								r={6 / view.zoom}
								stroke-width={1.5 / view.zoom}
							/>
							<circle
								class="handle"
								data-handle="b"
								cx={lineEnds.b[0]}
								cy={lineEnds.b[1]}
								r={6 / view.zoom}
								stroke-width={1.5 / view.zoom}
							/>
						{:else}
							<rect
								class="outline"
								x={pickedBox.x}
								y={pickedBox.y}
								width={pickedBox.w}
								height={pickedBox.h}
								stroke-width={1.5 / view.zoom}
							/>
							{#if single && !isLine(single.kind)}
								{#each HANDLES as h (h)}
									{@const pt = handleAt(single.box, h)}
									<rect
										class="handle {h}"
										data-handle={h}
										x={pt[0] - 5 / view.zoom}
										y={pt[1] - 5 / view.zoom}
										width={10 / view.zoom}
										height={10 / view.zoom}
										stroke-width={1.5 / view.zoom}
									/>
								{/each}
							{/if}
						{/if}
					{/if}
					{#each guides as g, i (i)}
						{#if g.x !== undefined}
							<line
								class="guide"
								x1={g.x}
								x2={g.x}
								y1="-100000"
								y2="100000"
								stroke-width={1 / view.zoom}
							/>
						{:else if g.y !== undefined}
							<line
								class="guide"
								y1={g.y}
								y2={g.y}
								x1="-100000"
								x2="100000"
								stroke-width={1 / view.zoom}
							/>
						{/if}
					{/each}
					{#if marquee}
						<rect
							class="marquee"
							x={marquee.x}
							y={marquee.y}
							width={marquee.w}
							height={marquee.h}
							stroke-width={1 / view.zoom}
						/>
					{/if}
				</g>
			</svg>
			{#if editing && editBox}
				<textarea
					class="words"
					bind:this={field}
					bind:value={editing.value}
					aria-label="Words in the shape"
					maxlength={MAX_TEXT}
					style:left="{editBox.left}px"
					style:top="{editBox.top}px"
					style:width="{editBox.width}px"
					style:height="{editBox.height}px"
					style:font-size="{editBox.size}px"
					style:font-weight={editBox.bold ? 600 : 400}
					style:font-style={editBox.italic ? 'italic' : 'normal'}
					onblur={() => stopEditing(true)}
					onkeydown={(e) => {
						if (e.key === 'Escape') {
							e.preventDefault();
							e.stopPropagation();
							void stopEditing(true);
						} else if (e.key === 'Enter' && !e.shiftKey) {
							e.preventDefault();
							void stopEditing(true);
						}
					}}></textarea>
			{/if}
			{#if !stored.length && !adding && !readonly}
				<p class="hint">Choose a shape above, then click or drag on the canvas.</p>
			{/if}
		</div>

		{#if !readonly}
			<div class="panel" role="region" aria-label="Shape">
				{#if picked.length}
					<h3>{picked.length === 1 ? kindName(picked[0].kind) : `${picked.length} shapes`}</h3>
					{#if boxKinds.length}
						<fieldset>
							<legend>Fill</legend>
							<div class="swatches">
								<button
									type="button"
									class="swatch"
									class:on={common(boxKinds, 'fill') === undefined}
									aria-pressed={common(boxKinds, 'fill') === undefined}
									aria-label="Standard fill"
									title="Standard fill"
									onclick={() => restyle({ fill: undefined }, 'Standard fill')}
									style:background={tokenVar('diagram-node-bg')}
								></button>
								{#each FILLS as f (f)}
									<button
										type="button"
										class="swatch"
										class:on={common(boxKinds, 'fill') === f}
										aria-pressed={common(boxKinds, 'fill') === f}
										aria-label="{f[0].toUpperCase()}{f.slice(1)} fill"
										title="{f[0].toUpperCase()}{f.slice(1)}"
										onclick={() => restyle({ fill: f }, `${f} fill`)}
										style:background={tokenVar(`highlight-${f}`)}
									></button>
								{/each}
								<button
									type="button"
									class="swatch none"
									class:on={common(boxKinds, 'fill') === 'none'}
									aria-pressed={common(boxKinds, 'fill') === 'none'}
									aria-label="No fill"
									title="No fill"
									onclick={() => restyle({ fill: 'none' }, 'No fill')}
								></button>
							</div>
						</fieldset>
					{/if}
					<fieldset>
						<legend>{lineKinds.length === picked.length ? 'Line color' : 'Edge color'}</legend>
						<div class="swatches">
							<button
								type="button"
								class="swatch line"
								class:on={common(picked, 'stroke') === undefined}
								aria-pressed={common(picked, 'stroke') === undefined}
								aria-label="Standard color"
								title="Standard color"
								onclick={() => restyle({ stroke: undefined }, 'Standard color')}
								style:color={tokenVar('diagram-line')}
							></button>
							{#each STROKES as c (c)}
								<button
									type="button"
									class="swatch line"
									class:on={common(picked, 'stroke') === c}
									aria-pressed={common(picked, 'stroke') === c}
									aria-label="Color {c}"
									title="Color {c}"
									onclick={() => restyle({ stroke: c }, `Color ${c}`)}
									style:color={tokenVar(`chart-series-${c}`)}
								></button>
							{/each}
						</div>
					</fieldset>
					{#if picked.some((s) => s.kind !== 'pen' && s.kind !== 'image')}
						<fieldset>
							<legend>Text</legend>
							<div class="choices">
								{#each Object.keys(TEXT_SIZES) as z (z)}
									<button
										type="button"
										class="choice"
										class:on={common(picked, 'size') === z}
										aria-pressed={common(picked, 'size') === z}
										onclick={() =>
											restyle(
												{ size: z as 's' | 'm' | 'l' },
												`Text size ${z === 's' ? 'small' : z === 'm' ? 'medium' : 'large'}`
											)}>{z === 's' ? 'Small' : z === 'm' ? 'Medium' : 'Large'}</button
									>
								{/each}
							</div>
							<div class="choices">
								<button
									type="button"
									class="choice"
									class:on={common(picked, 'bold') === true}
									aria-pressed={common(picked, 'bold') === true}
									onclick={() =>
										restyle({ bold: common(picked, 'bold') === true ? undefined : true }, 'Bold')}
									><strong>Bold</strong></button
								>
								<button
									type="button"
									class="choice"
									class:on={common(picked, 'italic') === true}
									aria-pressed={common(picked, 'italic') === true}
									onclick={() =>
										restyle(
											{ italic: common(picked, 'italic') === true ? undefined : true },
											'Italic'
										)}><em>Italic</em></button
								>
							</div>
						</fieldset>
					{/if}
					{#if picked.some((s) => s.kind === 'rect')}
						<label class="check"
							><input
								type="checkbox"
								checked={common(
									picked.filter((s) => s.kind === 'rect'),
									'rounded'
								) === true}
								onchange={(e) =>
									restyle(
										{ rounded: e.currentTarget.checked || undefined },
										e.currentTarget.checked ? 'Rounded corners' : 'Square corners'
									)}
							/> Rounded corners</label
						>
					{/if}
					{#if lineKinds.length}
						<fieldset>
							<legend>Route</legend>
							<div class="choices">
								{#each ROUTES as r (r)}
									<button
										type="button"
										class="choice"
										class:on={(common(lineKinds, 'route') ??
											(lineKinds[0].kind === 'arrow' ? 'elbow' : 'straight')) === r}
										aria-pressed={(common(lineKinds, 'route') ??
											(lineKinds[0].kind === 'arrow' ? 'elbow' : 'straight')) === r}
										onclick={() => restyle({ route: r }, `${r} route`)}
										>{r[0].toUpperCase()}{r.slice(1)}</button
									>
								{/each}
							</div>
						</fieldset>
						<fieldset>
							<legend>Arrowheads</legend>
							<label class="check"
								><input
									type="checkbox"
									checked={common(lineKinds, 'start') === 'arrow'}
									onchange={(e) =>
										restyle(
											{ start: e.currentTarget.checked ? 'arrow' : 'none' },
											'Arrowhead at the start'
										)}
								/> At the start</label
							>
							<label class="check"
								><input
									type="checkbox"
									checked={(common(lineKinds, 'end') ??
										(lineKinds[0].kind === 'arrow' ? 'arrow' : 'none')) === 'arrow'}
									onchange={(e) =>
										restyle(
											{ end: e.currentTarget.checked ? 'arrow' : 'none' },
											'Arrowhead at the end'
										)}
								/> At the end</label
							>
						</fieldset>
					{/if}
					{#if picked.some((s) => s.kind !== 'text' && s.kind !== 'image')}
						<fieldset>
							<legend>Line style</legend>
							<div class="choices">
								{#each DASHES as d (d)}
									<button
										type="button"
										class="choice"
										class:on={(common(picked, 'dash') ?? 'solid') === d}
										aria-pressed={(common(picked, 'dash') ?? 'solid') === d}
										onclick={() => restyle({ dash: d === 'solid' ? undefined : d }, `${d} line`)}
										>{d[0].toUpperCase()}{d.slice(1)}</button
									>
								{/each}
							</div>
						</fieldset>
					{/if}
					<fieldset>
						<legend>Arrange</legend>
						<div class="choices">
							<button type="button" class="choice" onclick={() => order(true)}>To front</button>
							<button type="button" class="choice" onclick={() => order(false)}>To back</button>
							<button type="button" class="choice" onclick={duplicatePicked}>Duplicate</button>
						</div>
						{#if picked.filter((s) => !isLine(s.kind)).length > 1}
							<div class="choices">
								<button type="button" class="choice" onclick={() => align('left')}>Left</button>
								<button type="button" class="choice" onclick={() => align('center')}>Center</button>
								<button type="button" class="choice" onclick={() => align('right')}>Right</button>
							</div>
							<div class="choices">
								<button type="button" class="choice" onclick={() => align('top')}>Top</button>
								<button type="button" class="choice" onclick={() => align('middle')}>Middle</button>
								<button type="button" class="choice" onclick={() => align('bottom')}>Bottom</button>
							</div>
							{#if picked.filter((s) => !isLine(s.kind)).length > 2}
								<div class="choices">
									<button type="button" class="choice" onclick={() => distribute(true)}
										>Space across</button
									>
									<button type="button" class="choice" onclick={() => distribute(false)}
										>Space down</button
									>
								</div>
							{/if}
						{/if}
					</fieldset>
					<button type="button" class="delete" onclick={removePicked}>Delete</button>
				{:else}
					<h3>Shapes</h3>
					{#if stored.length}
						<ul class="list">
							{#each stored as s (s.id)}
								<li>
									<button
										type="button"
										onclick={() => {
											selection.clear();
											selection.add(s.id);
											focusShape(s.id);
										}}>{shapeLabel(s, byId)}</button
									>
								</li>
							{/each}
						</ul>
					{:else}
						<p class="muted">No shapes yet.</p>
					{/if}
				{/if}
			</div>
		{/if}
	</div>
	{#if choosingPicture && Picker}
		<Picker
			{ws}
			target={{ kind: 'page', id: pageId }}
			bind:open={choosingPicture}
			onpick={placePicture}
		/>
	{/if}
	<Announcer inDialog />
</dialog>

<style>
	.editor {
		width: 100vw;
		max-width: none;
		height: 100vh;
		height: 100dvh;
		max-height: none;
		margin: 0;
		padding: 0;
		border: 0;
		background: var(--dialog-bg);
		color: var(--text-primary);
	}
	.editor[open] {
		display: flex;
		flex-direction: column;
	}
	header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
		border-bottom: var(--border-width) solid var(--border);
	}
	h2 {
		flex: 1;
		font-size: var(--text-lg);
	}
	.name {
		flex: 1 1 160px;
		min-width: 120px;
		max-width: 280px;
	}
	.name input {
		width: 100%;
		min-height: max(var(--tap-min), 32px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid transparent;
		border-radius: var(--input-radius);
		background: transparent;
		color: var(--text-primary);
		font-weight: 600;
		font-size: var(--text-md);
	}
	.name input:hover,
	.name input:focus {
		border-color: var(--border-control);
		background: var(--input-bg);
	}
	.tools,
	.group {
		display: flex;
		align-items: center;
		gap: 2px;
		padding: 2px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	.tool {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 32px);
		padding: 0 var(--space-2);
		border: 0;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		cursor: pointer;
	}
	.tool.icon {
		min-width: max(var(--tap-min), 32px);
		justify-content: center;
		padding: 0;
	}
	.tool:hover:not(:disabled) {
		background: var(--button-ghost-hover);
		color: var(--text-primary);
	}
	.tool.on {
		background: var(--accent-soft);
		color: var(--accent-text);
	}
	.tool:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.percent {
		min-width: 4ch;
		text-align: center;
		font-size: var(--text-xs);
		color: var(--text-secondary);
		font-variant-numeric: tabular-nums;
	}
	.done {
		margin-left: auto;
		min-height: max(var(--tap-min), 32px);
		padding: 0 var(--space-4);
		border: 0;
		border-radius: var(--button-radius);
		background: var(--button-primary-bg);
		color: var(--button-primary-text);
		font-weight: 600;
		cursor: pointer;
	}
	.done:hover {
		background: var(--button-primary-hover);
	}
	.body {
		position: relative;
		display: flex;
		flex: 1;
		min-height: 0;
	}
	.stage {
		position: relative;
		flex: 1;
		min-width: 0;
		overflow: hidden;
		background: var(--surface-0);
		touch-action: none;
		cursor: default;
	}
	.stage.drawing-tool {
		cursor: crosshair;
	}
	.stage.panning {
		cursor: grab;
	}
	.canvas {
		display: block;
		width: 100%;
		height: 100%;
	}
	.shape {
		cursor: move;
		outline: none;
	}
	.panning .shape,
	.drawing-tool .shape {
		cursor: inherit;
	}
	.shape:focus-visible :global(*:not(.hit)) {
		filter: drop-shadow(0 0 2px var(--focus-ring-color));
	}
	.outline {
		fill: none;
		stroke: var(--accent);
		stroke-dasharray: 4 3;
		pointer-events: none;
	}
	.handle {
		fill: var(--surface-0);
		stroke: var(--accent);
	}
	.handle.n,
	.handle.s {
		cursor: ns-resize;
	}
	.handle.e,
	.handle.w {
		cursor: ew-resize;
	}
	.handle.nw,
	.handle.se {
		cursor: nwse-resize;
	}
	.handle.ne,
	.handle.sw {
		cursor: nesw-resize;
	}
	circle.handle {
		cursor: grab;
	}
	.joint {
		fill: var(--accent);
		pointer-events: none;
	}
	.target {
		fill: none;
		stroke: var(--accent);
		pointer-events: none;
	}
	.guide {
		stroke: var(--accent);
		pointer-events: none;
	}
	.marquee {
		fill: var(--accent-soft);
		fill-opacity: 0.4;
		stroke: var(--accent);
		pointer-events: none;
	}
	.words {
		position: absolute;
		z-index: 2;
		min-width: 60px;
		padding: 2px;
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-sm);
		background: var(--input-bg);
		color: var(--diagram-text);
		font-family: var(--font-page);
		line-height: 1.3;
		text-align: center;
		resize: none;
	}
	.hint {
		position: absolute;
		top: 40%;
		left: 0;
		right: 0;
		text-align: center;
		color: var(--text-muted);
		font-size: var(--text-sm);
		pointer-events: none;
	}
	.panel {
		width: 260px;
		flex: none;
		overflow-y: auto;
		padding: var(--space-3);
		border-left: var(--border-width) solid var(--border);
		background: var(--panel-bg);
		font-size: var(--text-sm);
	}
	.panel h3 {
		margin-bottom: var(--space-2);
		font-size: var(--text-md);
	}
	fieldset {
		margin: 0 0 var(--space-3);
		padding: 0;
		border: 0;
	}
	legend {
		margin-bottom: var(--space-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	.swatches,
	.choices {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
		margin-bottom: var(--space-1);
	}
	.swatch {
		width: max(var(--tap-min), 26px);
		height: max(var(--tap-min), 26px);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-sm);
		cursor: pointer;
	}
	.swatch.line {
		background: linear-gradient(
			135deg,
			transparent 42%,
			currentColor 42%,
			currentColor 58%,
			transparent 58%
		);
	}
	.swatch.none {
		background: linear-gradient(
			135deg,
			transparent 46%,
			var(--danger) 46%,
			var(--danger) 54%,
			transparent 54%
		);
	}
	.swatch.on {
		outline: 2px solid var(--accent);
		outline-offset: 1px;
	}
	.choice {
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-sm);
		background: var(--surface-1);
		color: var(--text-primary);
		font-size: var(--text-xs);
		cursor: pointer;
	}
	.choice:hover {
		border-color: var(--border-strong);
	}
	.choice.on {
		border-color: var(--accent);
		background: var(--accent-soft);
		color: var(--accent-text);
	}
	.check {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 28px);
		font-size: var(--text-xs);
	}
	.delete {
		min-height: max(var(--tap-min), 32px);
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--button-secondary-border);
		border-radius: var(--button-radius);
		background: var(--button-secondary-bg);
		color: var(--button-danger);
		cursor: pointer;
	}
	.list {
		display: grid;
		gap: 2px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.list button {
		width: 100%;
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-2);
		overflow: hidden;
		border: 0;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--text-primary);
		font-size: var(--text-xs);
		text-align: left;
		text-overflow: ellipsis;
		white-space: nowrap;
		cursor: pointer;
	}
	.list button:hover {
		background: var(--row-hover-bg);
	}
	.muted {
		color: var(--text-muted);
	}
	@media (max-width: 720px) {
		.body {
			flex-direction: column;
		}
		.panel {
			width: auto;
			max-height: 38vh;
			border-left: 0;
			border-top: var(--border-width) solid var(--border);
		}
		.tools {
			order: 3;
			width: 100%;
			overflow-x: auto;
		}
	}
</style>
