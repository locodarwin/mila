// Drawings in wiki pages, pure: what a drawing and its shapes hold, their
// colors (names of theme tokens), where lines run between shapes, text
// wrapping, snapping, the starters, and the SVG each shape draws as. The
// page's view, the editor and the picture saved with the page all draw
// through shapeSvg, so the three never differ. Rows are page_drawing and
// drawing_shape (server/internal/syncer/entities.go).

export const SHAPE_KINDS = [
	'rect',
	'ellipse',
	'diamond',
	'cylinder',
	'text',
	'sticky',
	'arrow',
	'line',
	'pen',
	'image'
] as const;
export type ShapeKind = (typeof SHAPE_KINDS)[number];

/** Shapes that join others, or are drawn by hand, rather than being boxes. */
export const LINE_KINDS: readonly ShapeKind[] = ['arrow', 'line', 'pen'];
export const isLine = (kind: ShapeKind) => LINE_KINDS.includes(kind);

/** The grid shapes snap to, in the drawing's pixels. */
export const GRID = 8;
/** How near, in screen pixels, an edge or middle must come to snap to another's. */
export const SNAP_REACH = 6;
export const MAX_SHAPES = 2_000;
export const MAX_DRAWINGS = 50;
export const MAX_TEXT = 2_000;
export const MAX_POINTS = 5_000;

export type Side = 'top' | 'right' | 'bottom' | 'left';
export const SIDES: readonly Side[] = ['top', 'right', 'bottom', 'left'];
export type Point = [number, number];

export interface Box {
	x: number;
	y: number;
	w: number;
	h: number;
}

/** Fills are the page's highlight colors; lines the charts' series. */
export const FILLS = ['yellow', 'green', 'blue', 'pink', 'gray'] as const;
export type Fill = (typeof FILLS)[number] | 'none';
export const STROKES = ['1', '2', '3', '4', '5', '6'] as const;
export type Stroke = (typeof STROKES)[number];

export const TEXT_SIZES = { s: 13, m: 16, l: 22 } as const;
export type TextSize = keyof typeof TEXT_SIZES;
export const ROUTES = ['straight', 'elbow', 'curved'] as const;
export type Route = (typeof ROUTES)[number];
export const DASHES = ['solid', 'dashed', 'dotted'] as const;
export type Dash = (typeof DASHES)[number];

export interface ShapeStyle {
	fill?: Fill;
	stroke?: Stroke;
	rounded?: boolean;
	bold?: boolean;
	italic?: boolean;
	size?: TextSize;
	route?: Route;
	start?: 'none' | 'arrow';
	end?: 'none' | 'arrow';
	dash?: Dash;
	fromSide?: Side;
	toSide?: Side;
}

export interface Shape {
	id: string;
	drawingId: string;
	kind: ShapeKind;
	box: Box;
	/** A pen stroke's points, or a line's free ends, from the box's corner. */
	points: Point[] | null;
	text: string;
	style: ShapeStyle;
	fromId: string | null;
	toId: string | null;
	imageId: string | null;
	sortKey: string;
}

export interface Drawing {
	id: string;
	pageId: string;
	name: string;
	grid: boolean;
	snap: boolean;
	madeBy: string;
}

const num = (v: unknown, d = 0) => (typeof v === 'number' && Number.isFinite(v) ? v : d);

/** A shape's box as stored, whatever was stored. */
export function readBox(v: unknown): Box {
	const b = (v && typeof v === 'object' ? v : {}) as Record<string, unknown>;
	return { x: num(b.x), y: num(b.y), w: Math.max(0, num(b.w)), h: Math.max(0, num(b.h)) };
}

export function readPoints(v: unknown): Point[] | null {
	if (!Array.isArray(v)) return null;
	const out: Point[] = [];
	for (const p of v) if (Array.isArray(p) && p.length === 2) out.push([num(p[0]), num(p[1])]);
	return out;
}

/** A shape from its row's values. */
export function shapeOf(id: string, v: Record<string, unknown>): Shape {
	const kind = SHAPE_KINDS.includes(v.kind as ShapeKind) ? (v.kind as ShapeKind) : 'rect';
	const style = (v.style && typeof v.style === 'object' ? v.style : {}) as ShapeStyle;
	return {
		id,
		drawingId: String(v.drawing_id ?? ''),
		kind,
		box: readBox(v.box),
		points: readPoints(v.points),
		text: typeof v.text === 'string' ? v.text : '',
		style,
		fromId: (v.from_id as string | null) || null,
		toId: (v.to_id as string | null) || null,
		imageId: (v.image_id as string | null) || null,
		sortKey: String(v.sort_key ?? '')
	};
}

export function drawingOf(id: string, v: Record<string, unknown>): Drawing {
	return {
		id,
		pageId: String(v.page_id ?? ''),
		name: typeof v.name === 'string' ? v.name : '',
		grid: v.grid !== false,
		snap: v.snap !== false,
		madeBy: String(v.made_by ?? '')
	};
}

// Colors: names of theme tokens, never colors.

/** The token a shape is filled with, or null for none. */
export function fillToken(s: Shape): string | null {
	if (s.kind === 'text' || isLine(s.kind) || s.kind === 'image') return null;
	const fill = s.style.fill;
	if (fill === 'none') return null;
	if (fill && (FILLS as readonly string[]).includes(fill)) return `highlight-${fill}`;
	return s.kind === 'sticky' ? 'diagram-note-bg' : 'diagram-node-bg';
}

/** The token a shape's edge or line is drawn in, or null for none. */
export function strokeToken(s: Shape): string | null {
	const stroke = s.style.stroke;
	if (stroke && (STROKES as readonly string[]).includes(stroke)) return `chart-series-${stroke}`;
	if (s.kind === 'text' || s.kind === 'sticky' || s.kind === 'image') return null;
	return isLine(s.kind) ? 'diagram-line' : 'diagram-node-border';
}

export const TEXT_TOKEN = 'diagram-text';

/** Every token a drawing's colors may name, for a picture in fixed colors. */
export const DRAWING_TOKENS = [
	'diagram-node-bg',
	'diagram-node-border',
	'diagram-text',
	'diagram-line',
	'diagram-note-bg',
	'surface-0',
	...FILLS.map((f) => `highlight-${f}`),
	...STROKES.map((s) => `chart-series-${s}`)
];

// Geometry.

export const center = (b: Box): Point => [b.x + b.w / 2, b.y + b.h / 2];

/** The middle of a side of a box, where lines join it. */
export function anchor(b: Box, side: Side): Point {
	switch (side) {
		case 'top':
			return [b.x + b.w / 2, b.y];
		case 'bottom':
			return [b.x + b.w / 2, b.y + b.h];
		case 'left':
			return [b.x, b.y + b.h / 2];
		case 'right':
			return [b.x + b.w, b.y + b.h / 2];
	}
}

const OUT: Record<Side, Point> = { top: [0, -1], bottom: [0, 1], left: [-1, 0], right: [1, 0] };

/** The side of a box that faces a point. */
export function facing(b: Box, p: Point): Side {
	const [cx, cy] = center(b);
	const dx = (p[0] - cx) / Math.max(b.w, 1);
	const dy = (p[1] - cy) / Math.max(b.h, 1);
	if (Math.abs(dx) >= Math.abs(dy)) return dx >= 0 ? 'right' : 'left';
	return dy >= 0 ? 'bottom' : 'top';
}

/** The side of a box nearest a point, for a line dropped on it. */
export function nearestSide(b: Box, p: Point): Side {
	let best: Side = 'top';
	let d = Infinity;
	for (const side of SIDES) {
		const [x, y] = anchor(b, side);
		const e = Math.hypot(x - p[0], y - p[1]);
		if (e < d) {
			d = e;
			best = side;
		}
	}
	return best;
}

export interface Ends {
	a: Point;
	b: Point;
	aSide: Side | null;
	bSide: Side | null;
}

/** Where a line or arrow starts and ends: on its shapes, or at its own points. */
export function endsOf(line: Shape, byId: Map<string, Shape>): Ends {
	const pts = line.points ?? [];
	const free = (i: number): Point =>
		pts[i] ? [line.box.x + pts[i][0], line.box.y + pts[i][1]] : [line.box.x, line.box.y];
	const from = line.fromId ? byId.get(line.fromId) : undefined;
	const to = line.toId ? byId.get(line.toId) : undefined;
	let a = free(0);
	let b = free(pts.length - 1);
	const aTarget = to ? center(to.box) : b;
	const bTarget = from ? center(from.box) : a;
	let aSide: Side | null = null;
	let bSide: Side | null = null;
	if (from) {
		aSide = line.style.fromSide ?? facing(from.box, aTarget);
		a = anchor(from.box, aSide);
	}
	if (to) {
		bSide = line.style.toSide ?? facing(to.box, from ? a : bTarget);
		b = anchor(to.box, bSide);
	}
	return { a, b, aSide, bSide };
}

const fmt = (n: number) => String(Math.round(n * 10) / 10);

/** The path a line runs, and the direction it leaves each end in. */
export function routeOf(
	line: Shape,
	ends: Ends
): { d: string; startDir: Point; endDir: Point; mid: Point } {
	const { a, b } = ends;
	const route = line.style.route ?? (line.kind === 'arrow' ? 'elbow' : 'straight');
	const toward = (p: Point, q: Point): Point => {
		const l = Math.hypot(q[0] - p[0], q[1] - p[1]) || 1;
		return [(q[0] - p[0]) / l, (q[1] - p[1]) / l];
	};
	if (route === 'curved') {
		const k = Math.max(40, Math.hypot(b[0] - a[0], b[1] - a[1]) / 3);
		const da = ends.aSide ? OUT[ends.aSide] : toward(a, b);
		const db = ends.bSide ? OUT[ends.bSide] : toward(b, a);
		const c1: Point = [a[0] + da[0] * k, a[1] + da[1] * k];
		const c2: Point = [b[0] + db[0] * k, b[1] + db[1] * k];
		const mid: Point = [
			(a[0] + 3 * c1[0] + 3 * c2[0] + b[0]) / 8,
			(a[1] + 3 * c1[1] + 3 * c2[1] + b[1]) / 8
		];
		return {
			d: `M${fmt(a[0])} ${fmt(a[1])}C${fmt(c1[0])} ${fmt(c1[1])} ${fmt(c2[0])} ${fmt(c2[1])} ${fmt(b[0])} ${fmt(b[1])}`,
			startDir: toward(c1, a),
			endDir: toward(c2, b),
			mid
		};
	}
	if (route === 'elbow') {
		const horizontal = (s: Side | null, d: Point) =>
			s ? s === 'left' || s === 'right' : Math.abs(d[0]) >= Math.abs(d[1]);
		const delta: Point = [b[0] - a[0], b[1] - a[1]];
		const hA = horizontal(ends.aSide, delta);
		const hB = horizontal(ends.bSide, delta);
		let pts: Point[];
		if (hA && hB) {
			const mx = (a[0] + b[0]) / 2;
			pts = [a, [mx, a[1]], [mx, b[1]], b];
		} else if (!hA && !hB) {
			const my = (a[1] + b[1]) / 2;
			pts = [a, [a[0], my], [b[0], my], b];
		} else if (hA) {
			pts = [a, [b[0], a[1]], b];
		} else {
			pts = [a, [a[0], b[1]], b];
		}
		const d = pts.map((p, i) => `${i ? 'L' : 'M'}${fmt(p[0])} ${fmt(p[1])}`).join('');
		const m = Math.floor((pts.length - 1) / 2);
		return {
			d,
			startDir: toward(pts[1], pts[0]),
			endDir: toward(pts[pts.length - 2], pts[pts.length - 1]),
			mid: [(pts[m][0] + pts[m + 1][0]) / 2, (pts[m][1] + pts[m + 1][1]) / 2]
		};
	}
	return {
		d: `M${fmt(a[0])} ${fmt(a[1])}L${fmt(b[0])} ${fmt(b[1])}`,
		startDir: toward(b, a),
		endDir: toward(a, b),
		mid: [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2]
	};
}

/** An arrowhead at a point, pointing the way a line arrives there. */
function head(at: Point, dir: Point, size = 10): string {
	const [ux, uy] = dir;
	const back: Point = [at[0] - ux * size, at[1] - uy * size];
	const px = -uy * size * 0.5;
	const py = ux * size * 0.5;
	return `M${fmt(at[0])} ${fmt(at[1])}L${fmt(back[0] + px)} ${fmt(back[1] + py)}L${fmt(back[0] - px)} ${fmt(back[1] - py)}Z`;
}

/** The box around everything a shape draws, lines included. */
export function extentOf(s: Shape, byId: Map<string, Shape>): Box {
	if (s.kind === 'pen' && s.points?.length) {
		const xs = s.points.map((p) => s.box.x + p[0]);
		const ys = s.points.map((p) => s.box.y + p[1]);
		const x = Math.min(...xs);
		const y = Math.min(...ys);
		return { x, y, w: Math.max(...xs) - x, h: Math.max(...ys) - y };
	}
	if (s.kind === 'arrow' || s.kind === 'line') {
		const { a, b } = endsOf(s, byId);
		const x = Math.min(a[0], b[0]);
		const y = Math.min(a[1], b[1]);
		return { x, y, w: Math.abs(a[0] - b[0]), h: Math.abs(a[1] - b[1]) };
	}
	return s.box;
}

/** The box around a drawing's shapes, with room around it; a blank one is a small square. */
export function boundsOf(shapes: Shape[], pad = 24): Box {
	if (!shapes.length) return { x: 0, y: 0, w: 320, h: 160 };
	const byId = new Map(shapes.map((s) => [s.id, s]));
	let x0 = Infinity;
	let y0 = Infinity;
	let x1 = -Infinity;
	let y1 = -Infinity;
	for (const s of shapes) {
		const e = extentOf(s, byId);
		x0 = Math.min(x0, e.x);
		y0 = Math.min(y0, e.y);
		x1 = Math.max(x1, e.x + e.w);
		y1 = Math.max(y1, e.y + e.h);
	}
	return { x: x0 - pad, y: y0 - pad, w: x1 - x0 + pad * 2, h: y1 - y0 + pad * 2 };
}

export function contains(b: Box, p: Point): boolean {
	return p[0] >= b.x && p[0] <= b.x + b.w && p[1] >= b.y && p[1] <= b.y + b.h;
}

export function overlaps(a: Box, b: Box): boolean {
	return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;
}

/** A box from two corners, either way round. */
export function boxBetween(p: Point, q: Point): Box {
	return {
		x: Math.min(p[0], q[0]),
		y: Math.min(p[1], q[1]),
		w: Math.abs(q[0] - p[0]),
		h: Math.abs(q[1] - p[1])
	};
}

export const snapTo = (v: number, grid = GRID) => Math.round(v / grid) * grid;

/**
 * How far to nudge a box being moved so that an edge or middle lines up
 * with another shape's, within reach; and the lines that show it.
 */
export function alignTo(
	box: Box,
	others: Box[],
	reach: number
): { dx: number; dy: number; guides: { x?: number; y?: number }[] } {
	const xs = [box.x, box.x + box.w / 2, box.x + box.w];
	const ys = [box.y, box.y + box.h / 2, box.y + box.h];
	let dx = 0;
	let dy = 0;
	let bestX = reach + 1;
	let bestY = reach + 1;
	let gx: number | undefined;
	let gy: number | undefined;
	for (const o of others) {
		const oxs = [o.x, o.x + o.w / 2, o.x + o.w];
		const oys = [o.y, o.y + o.h / 2, o.y + o.h];
		for (const a of xs)
			for (const b of oxs)
				if (Math.abs(b - a) < bestX) {
					bestX = Math.abs(b - a);
					dx = b - a;
					gx = b;
				}
		for (const a of ys)
			for (const b of oys)
				if (Math.abs(b - a) < bestY) {
					bestY = Math.abs(b - a);
					dy = b - a;
					gy = b;
				}
	}
	const guides: { x?: number; y?: number }[] = [];
	if (bestX <= reach) guides.push({ x: gx });
	else dx = 0;
	if (bestY <= reach) guides.push({ y: gy });
	else dy = 0;
	return { dx, dy, guides };
}

// Text.

export type Measure = (text: string, size: number, bold: boolean) => number;

/** A rough width for text, where nothing can measure it (tests, the server's absence). */
export const roughMeasure: Measure = (text, size, bold) => text.length * size * (bold ? 0.6 : 0.55);

/** Text broken into lines that fit a width, at its own line breaks and between words. */
export function wrapText(
	text: string,
	width: number,
	size: number,
	bold: boolean,
	measure: Measure
) {
	const lines: string[] = [];
	for (const para of text.split('\n')) {
		const words = para.split(/(\s+)/).filter((w) => w !== '');
		let line = '';
		for (const w of words) {
			const next = line + w;
			if (line && measure(next.trimEnd(), size, bold) > width) {
				lines.push(line.trimEnd());
				line = w.trimStart();
			} else line = next;
			// A word longer than the width is broken where it must be.
			while (measure(line, size, bold) > width && line.length > 1) {
				let cut = line.length - 1;
				while (cut > 1 && measure(line.slice(0, cut), size, bold) > width) cut--;
				lines.push(line.slice(0, cut));
				line = line.slice(cut);
			}
		}
		lines.push(line.trimEnd());
	}
	return lines;
}

export const PAD = 8;
export const lineHeight = (size: number) => Math.round(size * 1.3);

/** The room a shape's text has inside it. */
export function textRoom(s: Shape): Box {
	const b = s.box;
	switch (s.kind) {
		case 'diamond':
			return { x: b.x + b.w * 0.2, y: b.y + b.h * 0.25, w: b.w * 0.6, h: b.h * 0.5 };
		case 'ellipse':
			return { x: b.x + b.w * 0.15, y: b.y + b.h * 0.15, w: b.w * 0.7, h: b.h * 0.7 };
		case 'cylinder': {
			const ry = cylinderRy(b);
			return { x: b.x + PAD, y: b.y + ry * 2, w: b.w - PAD * 2, h: b.h - ry * 3 };
		}
		default:
			return { x: b.x + PAD, y: b.y + PAD, w: Math.max(1, b.w - PAD * 2), h: b.h - PAD * 2 };
	}
}

/** How tall a shape must be for its text, at least as tall as it is: shapes grow for their words. */
export function heightFor(s: Shape, measure: Measure): number {
	if (!s.text.trim() || isLine(s.kind) || s.kind === 'image') return s.box.h;
	const size = TEXT_SIZES[s.style.size ?? 'm'];
	const room = textRoom(s);
	const lines = wrapText(s.text, room.w, size, !!s.style.bold, measure).length;
	const need = lines * lineHeight(size) + (s.box.h - room.h);
	return Math.max(s.box.h, Math.ceil(need / GRID) * GRID);
}

const cylinderRy = (b: Box) => Math.min(b.h * 0.15, 14);

// Drawing.

export interface Paint {
	/** A token's color: var(--token) in the app, its value in a picture. */
	color: (token: string) => string;
	measure: Measure;
	/** The font text is drawn in. */
	font: string;
	/** Where a page's file is, for a picture; or its contents, in a picture of its own. */
	image: (id: string) => string | null;
}

const esc = (s: string) =>
	s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

function dashOf(s: Shape): string {
	if (s.style.dash === 'dashed') return ' stroke-dasharray="8 6"';
	if (s.style.dash === 'dotted') return ' stroke-dasharray="2 5"';
	return '';
}

function textSvg(s: Shape, p: Paint, room: Box): string {
	if (!s.text.trim()) return '';
	const size = TEXT_SIZES[s.style.size ?? 'm'];
	const bold = !!s.style.bold;
	const lines = wrapText(s.text, room.w, size, bold, p.measure);
	const lh = lineHeight(size);
	const top = room.y + room.h / 2 - (lines.length * lh) / 2 + lh * 0.75;
	const cx = room.x + room.w / 2;
	const attrs =
		`font-family="${esc(p.font)}" font-size="${size}"` +
		(bold ? ' font-weight="600"' : '') +
		(s.style.italic ? ' font-style="italic"' : '');
	const tspans = lines
		.map((l, i) => `<tspan x="${fmt(cx)}" y="${fmt(top + i * lh)}">${esc(l) || ' '}</tspan>`)
		.join('');
	return `<text ${attrs} text-anchor="middle" fill="${p.color(TEXT_TOKEN)}">${tspans}</text>`;
}

/**
 * One shape as SVG markup. Lines draw a wide clear path under them too
 * when asked (`hit`), for the editor to pick them by.
 */
export function shapeSvg(s: Shape, byId: Map<string, Shape>, p: Paint, hit = false): string {
	const fillT = fillToken(s);
	const strokeT = strokeToken(s);
	const fill = fillT ? p.color(fillT) : 'none';
	const stroke = strokeT ? p.color(strokeT) : 'none';
	const sw = isLine(s.kind) ? 2 : 1.5;
	const paint = `fill="${fill}" stroke="${stroke}" stroke-width="${sw}"${dashOf(s)}`;
	const b = s.box;
	switch (s.kind) {
		case 'rect':
		case 'sticky': {
			const rx = s.kind === 'sticky' ? 2 : s.style.rounded ? 10 : 2;
			return (
				`<rect x="${fmt(b.x)}" y="${fmt(b.y)}" width="${fmt(b.w)}" height="${fmt(b.h)}" rx="${rx}" ${paint}/>` +
				textSvg(s, p, textRoom(s))
			);
		}
		case 'ellipse':
			return (
				`<ellipse cx="${fmt(b.x + b.w / 2)}" cy="${fmt(b.y + b.h / 2)}" rx="${fmt(b.w / 2)}" ry="${fmt(b.h / 2)}" ${paint}/>` +
				textSvg(s, p, textRoom(s))
			);
		case 'diamond': {
			const [cx, cy] = center(b);
			return (
				`<polygon points="${fmt(cx)},${fmt(b.y)} ${fmt(b.x + b.w)},${fmt(cy)} ${fmt(cx)},${fmt(b.y + b.h)} ${fmt(b.x)},${fmt(cy)}" ${paint}/>` +
				textSvg(s, p, textRoom(s))
			);
		}
		case 'cylinder': {
			const ry = cylinderRy(b);
			const rx = b.w / 2;
			const body = `M${fmt(b.x)} ${fmt(b.y + ry)}A${fmt(rx)} ${fmt(ry)} 0 0 0 ${fmt(b.x + b.w)} ${fmt(b.y + ry)}V${fmt(b.y + b.h - ry)}A${fmt(rx)} ${fmt(ry)} 0 0 1 ${fmt(b.x)} ${fmt(b.y + b.h - ry)}Z`;
			const lid = `M${fmt(b.x)} ${fmt(b.y + ry)}A${fmt(rx)} ${fmt(ry)} 0 0 1 ${fmt(b.x + b.w)} ${fmt(b.y + ry)}`;
			return (
				`<path d="${body}" ${paint}/><path d="${lid}" fill="none" stroke="${stroke}" stroke-width="${sw}"/>` +
				textSvg(s, p, textRoom(s))
			);
		}
		case 'text':
			return textSvg(s, p, { x: b.x, y: b.y, w: Math.max(1, b.w), h: b.h });
		case 'image': {
			const href = s.imageId ? p.image(s.imageId) : null;
			if (!href)
				return `<rect x="${fmt(b.x)}" y="${fmt(b.y)}" width="${fmt(b.w)}" height="${fmt(b.h)}" fill="none" stroke="${p.color('diagram-node-border')}" stroke-dasharray="4 4"/>`;
			return `<image href="${esc(href)}" x="${fmt(b.x)}" y="${fmt(b.y)}" width="${fmt(b.w)}" height="${fmt(b.h)}" preserveAspectRatio="xMidYMid meet"/>`;
		}
		case 'pen': {
			const pts = s.points ?? [];
			if (!pts.length) return '';
			const d = pts.map((q, i) => `${i ? 'L' : 'M'}${fmt(b.x + q[0])} ${fmt(b.y + q[1])}`).join('');
			const line = `<path d="${d}" fill="none" stroke="${stroke}" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"${dashOf(s)}/>`;
			return (
				(hit
					? `<path d="${d}" fill="none" stroke="transparent" stroke-width="14" class="hit"/>`
					: '') + line
			);
		}
		case 'arrow':
		case 'line': {
			const ends = endsOf(s, byId);
			const r = routeOf(s, ends);
			const start = s.style.start ?? 'none';
			const end = s.style.end ?? (s.kind === 'arrow' ? 'arrow' : 'none');
			let out = hit
				? `<path d="${r.d}" fill="none" stroke="transparent" stroke-width="14" class="hit"/>`
				: '';
			out += `<path d="${r.d}" fill="none" stroke="${stroke}" stroke-width="${sw}" stroke-linejoin="round"${dashOf(s)}/>`;
			if (end === 'arrow') out += `<path d="${head(ends.b, r.endDir)}" fill="${stroke}"/>`;
			if (start === 'arrow') out += `<path d="${head(ends.a, r.startDir)}" fill="${stroke}"/>`;
			if (s.text.trim()) {
				const size = TEXT_SIZES[s.style.size ?? 's'];
				const w = Math.min(240, p.measure(s.text, size, !!s.style.bold) + PAD * 2);
				const lines = wrapText(s.text, w - PAD * 2, size, !!s.style.bold, p.measure);
				const h = lines.length * lineHeight(size) + PAD;
				const room = { x: r.mid[0] - w / 2, y: r.mid[1] - h / 2, w, h };
				out += `<rect x="${fmt(room.x)}" y="${fmt(room.y)}" width="${fmt(w)}" height="${fmt(h)}" rx="3" fill="${p.color('surface-0')}"/>`;
				out += textSvg({ ...s, style: { ...s.style, size: s.style.size ?? 's' } }, p, {
					x: room.x + PAD,
					y: room.y,
					w: w - PAD * 2,
					h
				});
			}
			return out;
		}
	}
}

/** Shapes in the order they draw, lowest first. */
export function inOrder(shapes: Shape[]): Shape[] {
	return [...shapes].sort((a, b) =>
		a.sortKey < b.sortKey ? -1 : a.sortKey > b.sortKey ? 1 : a.id < b.id ? -1 : 1
	);
}

/** A whole drawing as an SVG file of its own, in the colors given. */
export function drawingSvg(shapes: Shape[], p: Paint, title: string, background: string): string {
	const ordered = inOrder(shapes);
	const byId = new Map(ordered.map((s) => [s.id, s]));
	const b = boundsOf(ordered);
	const body = ordered.map((s) => shapeSvg(s, byId, p)).join('');
	return (
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="${fmt(b.x)} ${fmt(b.y)} ${fmt(b.w)} ${fmt(b.h)}" width="${Math.ceil(b.w)}" height="${Math.ceil(b.h)}" role="img">` +
		`<title>${esc(title || 'Drawing')}</title>` +
		`<rect x="${fmt(b.x)}" y="${fmt(b.y)}" width="${fmt(b.w)}" height="${fmt(b.h)}" fill="${background}"/>` +
		body +
		'</svg>'
	);
}

// Words, for screen readers.

const KIND_NAMES: Record<ShapeKind, string> = {
	rect: 'Box',
	ellipse: 'Ellipse',
	diamond: 'Diamond',
	cylinder: 'Cylinder',
	text: 'Text',
	sticky: 'Sticky note',
	arrow: 'Arrow',
	line: 'Line',
	pen: 'Drawn line',
	image: 'Picture'
};

export const kindName = (k: ShapeKind) => KIND_NAMES[k];

/** What a screen reader hears for a shape. */
export function shapeLabel(s: Shape, byId: Map<string, Shape>): string {
	const words = s.text.trim().replace(/\s+/g, ' ');
	const named = (id: string | null) => {
		const o = id ? byId.get(id) : undefined;
		return o ? o.text.trim().replace(/\s+/g, ' ') || kindName(o.kind).toLowerCase() : null;
	};
	if (s.kind === 'arrow' || s.kind === 'line') {
		const from = named(s.fromId);
		const to = named(s.toId);
		const kind = kindName(s.kind);
		const joins =
			from && to ? ` from ${from} to ${to}` : from ? ` from ${from}` : to ? ` to ${to}` : '';
		return `${kind}${joins}${words ? `, ${words}` : ''}`;
	}
	return words ? `${kindName(s.kind)}: ${words}` : kindName(s.kind);
}

/** What a drawing holds, in a sentence. */
export function drawingSummary(shapes: Shape[]): string {
	const lines = shapes.filter((s) => s.kind === 'arrow' || s.kind === 'line').length;
	const rest = shapes.length - lines;
	if (!shapes.length) return 'An empty drawing';
	const part = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;
	return lines
		? `${part(rest, 'shape', 'shapes')} and ${part(lines, 'line', 'lines')}`
		: part(rest, 'shape', 'shapes');
}

// Starters: shapes to begin from, under made-up ids the caller replaces.

export interface StarterShape {
	key: string;
	kind: ShapeKind;
	box: Box;
	text?: string;
	style?: ShapeStyle;
	from?: string;
	to?: string;
}

export interface DrawingStarter {
	id: string;
	label: string;
	detail: string;
	name: string;
	shapes: StarterShape[];
}

const box = (x: number, y: number, w = 160, h = 64): Box => ({ x, y, w, h });

export const DRAWING_STARTERS: DrawingStarter[] = [
	{ id: 'blank', label: 'Blank', detail: 'An empty canvas', name: '', shapes: [] },
	{
		id: 'flowchart',
		label: 'Flowchart',
		detail: 'Steps and a decision',
		name: 'Flowchart',
		shapes: [
			{ key: 'a', kind: 'rect', box: box(0, 0), text: 'Start', style: { rounded: true } },
			{ key: 'b', kind: 'diamond', box: box(240, -24, 160, 112), text: 'Approved?' },
			{ key: 'c', kind: 'rect', box: box(480, 0), text: 'Do the work', style: { rounded: true } },
			{ key: 'd', kind: 'rect', box: box(240, 160), text: 'Revise', style: { rounded: true } },
			{ key: 'ab', kind: 'arrow', box: box(0, 0, 0, 0), from: 'a', to: 'b' },
			{ key: 'bc', kind: 'arrow', box: box(0, 0, 0, 0), from: 'b', to: 'c', text: 'Yes' },
			{ key: 'bd', kind: 'arrow', box: box(0, 0, 0, 0), from: 'b', to: 'd', text: 'No' }
		]
	},
	{
		id: 'org',
		label: 'Org chart',
		detail: 'Teams and who reports to whom',
		name: 'Team structure',
		shapes: [
			{ key: 'lead', kind: 'rect', box: box(240, 0), text: 'Team lead', style: { fill: 'blue' } },
			{ key: 'd', kind: 'rect', box: box(0, 144), text: 'Design' },
			{ key: 'e', kind: 'rect', box: box(240, 144), text: 'Engineering' },
			{ key: 'o', kind: 'rect', box: box(480, 144), text: 'Operations' },
			{
				key: 'ld',
				kind: 'line',
				box: box(0, 0, 0, 0),
				from: 'lead',
				to: 'd',
				style: { route: 'elbow' }
			},
			{
				key: 'le',
				kind: 'line',
				box: box(0, 0, 0, 0),
				from: 'lead',
				to: 'e',
				style: { route: 'elbow' }
			},
			{
				key: 'lo',
				kind: 'line',
				box: box(0, 0, 0, 0),
				from: 'lead',
				to: 'o',
				style: { route: 'elbow' }
			}
		]
	},
	{
		id: 'architecture',
		label: 'Architecture',
		detail: 'Services and how they connect',
		name: 'Architecture',
		shapes: [
			{ key: 'u', kind: 'ellipse', box: box(0, 8, 128, 56), text: 'People' },
			{
				key: 'w',
				kind: 'rect',
				box: box(208, 0, 160, 72),
				text: 'Web app',
				style: { rounded: true }
			},
			{ key: 'a', kind: 'rect', box: box(448, 0, 160, 72), text: 'API', style: { rounded: true } },
			{ key: 'db', kind: 'cylinder', box: box(448, 152, 160, 96), text: 'Database' },
			{ key: 'q', kind: 'sticky', box: box(208, 152, 160, 96), text: 'Notes about the setup' },
			{
				key: 'uw',
				kind: 'arrow',
				box: box(0, 0, 0, 0),
				from: 'u',
				to: 'w',
				style: { route: 'straight' }
			},
			{
				key: 'wa',
				kind: 'arrow',
				box: box(0, 0, 0, 0),
				from: 'w',
				to: 'a',
				style: { route: 'straight' }
			},
			{
				key: 'adb',
				kind: 'arrow',
				box: box(0, 0, 0, 0),
				from: 'a',
				to: 'db',
				style: { route: 'straight' }
			}
		]
	}
];
