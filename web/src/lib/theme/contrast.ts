// Which colors of a theme must be readable on which, and how far apart.
// The checks run on colors the browser resolved (resolve.ts), so refs and
// color-mix() count exactly as drawn. They warn; only the floors (text
// and the focus ring on the page, 3:1 in both schemes) keep a theme from
// being used.
import {
	contrastRatio,
	inGamut,
	linearSrgbToOklch,
	MARGIN,
	oklchToLinearSrgb,
	type Lch
} from './color';
import type { Target } from './document';
import { linear, type Color, type Resolved } from './resolve';
import { PALETTE, PRIORITIES, STATUS_CATEGORIES } from './palette';

/** text: body text; large: big or bold text; ui: marks and edges (WCAG 1.4.11). */
export type Need = 'text' | 'large' | 'ui';

/** A background mixed in OKLCH, as a component draws it: a share of one token over another. */
export interface Mix {
	mix: [string, string, number];
	label: string;
}
export type Background = string | Mix;

export interface Pair {
	fg: string;
	bg: Background[];
	min: Need;
	label: string;
}

const SURFACES = ['surface-0', 'surface-1', 'surface-2', 'surface-3'];
const PANELS = ['panel-bg', 'card-bg', 'menu-bg', 'sidebar-bg', 'dialog-bg'];

export const PAIRS: readonly Pair[] = [
	{ fg: 'text-primary', bg: [...SURFACES, ...PANELS], min: 'text', label: 'Text' },
	{ fg: 'text-secondary', bg: [...SURFACES, ...PANELS], min: 'text', label: 'Secondary text' },
	{
		fg: 'text-primary',
		bg: [
			'highlight-yellow',
			'highlight-green',
			'highlight-blue',
			'highlight-pink',
			'highlight-gray'
		],
		min: 'text',
		label: 'Highlighted words'
	},
	{ fg: 'text-muted', bg: [...SURFACES, ...PANELS], min: 'text', label: 'Muted text' },
	{ fg: 'accent-text', bg: [...SURFACES, 'accent-soft'], min: 'text', label: 'Accent text' },
	{
		fg: 'accent-contrast',
		bg: ['accent', 'accent-hover', 'button-primary-bg'],
		min: 'text',
		label: 'Text on the accent'
	},
	{
		fg: 'border-control',
		bg: ['input-bg', 'surface-0', 'surface-1'],
		min: 'ui',
		label: 'Control edges'
	},
	{
		fg: 'input-border',
		bg: ['input-bg', 'surface-0', 'surface-1'],
		min: 'ui',
		label: 'Text field edges'
	},
	{ fg: 'focus-ring-color', bg: ['surface-0', 'surface-1'], min: 'ui', label: 'Focus ring' },
	{ fg: 'danger', bg: ['surface-0', 'surface-1'], min: 'text', label: 'Danger' },
	{ fg: 'toast-text', bg: ['toast-bg'], min: 'text', label: 'Toast text' },
	{
		fg: 'row-meta-text',
		bg: ['surface-0', 'row-hover-bg'],
		min: 'text',
		label: 'Row details'
	},
	...STATUS_CATEGORIES.map((s): Pair => ({
		fg: `status-${s}`,
		bg: ['surface-0'],
		min: 'ui',
		label: `Status icon: ${s}`
	})),
	...PRIORITIES.map((p): Pair => ({
		fg: `priority-${p}`,
		bg: ['surface-0'],
		min: 'ui',
		label: `Priority icon: ${p}`
	})),
	...PALETTE.map((c): Pair => ({
		fg: `palette-${c}`,
		bg: ['surface-0'],
		min: 'ui',
		label: `Label color: ${c}`
	})),
	...PALETTE.map((c): Pair => ({
		fg: 'text-primary',
		bg: [
			`palette-${c}-soft`,
			// A chosen option of a select field (FieldInput.svelte).
			{ mix: [`palette-${c}`, 'surface-0', 0.18], label: `${c} option` }
		],
		min: 'text',
		label: `Text on ${c} chips`
	})),
	{
		fg: 'sidebar-text',
		bg: ['sidebar-bg', 'sidebar-item-hover'],
		min: 'text',
		label: 'Sidebar text'
	},
	{
		fg: 'sidebar-text-active',
		bg: ['sidebar-bg', 'sidebar-item-hover', 'sidebar-item-active'],
		min: 'text',
		label: 'Sidebar text, current'
	},
	{
		fg: 'button-primary-text',
		bg: ['button-primary-bg', 'button-primary-hover'],
		min: 'text',
		label: 'Primary button text'
	},
	{
		fg: 'button-secondary-text',
		bg: ['button-secondary-bg', 'button-ghost-hover'],
		min: 'text',
		label: 'Secondary button text'
	},
	{ fg: 'button-danger', bg: ['surface-0'], min: 'text', label: 'Danger button' },
	{ fg: 'tabbar-active', bg: ['tabbar-bg'], min: 'text', label: 'Current tab' },
	{ fg: 'text-secondary', bg: ['tabbar-bg'], min: 'text', label: 'Other tabs' },
	{
		fg: 'text-primary',
		bg: ['row-hover-bg', 'row-selected-bg', 'board-column-bg'],
		min: 'text',
		label: 'Text on rows and columns'
	},
	{ fg: 'group-header-text', bg: ['surface-0'], min: 'text', label: 'Group headings' },
	// Markdown in task notes (on the panel) and wiki pages (on the page).
	...(
		[
			['page-heading-1', 'Headings 1'],
			['page-heading-2', 'Headings 2'],
			['page-heading-3', 'Smaller headings'],
			['page-link', 'Links in pages'],
			['page-list-marker', 'List bullets and numbers'],
			['page-quote-text', 'Quoted words']
		] as const
	).map(([fg, label]): Pair => ({ fg, bg: ['surface-0', 'panel-bg'], min: 'text', label })),
	{ fg: 'page-code-text', bg: ['page-code-bg'], min: 'text', label: 'Code in pages' },
	{
		fg: 'page-table-head-text',
		bg: ['page-table-head-bg'],
		min: 'text',
		label: 'Table header text'
	},
	{ fg: 'text-primary', bg: ['page-table-stripe'], min: 'text', label: 'Text on table stripes' },
	{ fg: 'accent-contrast', bg: ['calendar-today-bg'], min: 'text', label: "Today's date" },
	{ fg: 'input-text', bg: ['input-bg'], min: 'text', label: 'Text in fields' },
	{ fg: 'input-placeholder', bg: ['input-bg'], min: 'text', label: 'Placeholders' },
	{ fg: 'checkbox-mark', bg: ['checkbox-on-bg'], min: 'ui', label: 'Check marks' },
	{ fg: 'calendar-today-bg', bg: ['surface-0'], min: 'ui', label: 'Marker for today' },
	...[1, 2, 3, 4, 5, 6].map((n): Pair => ({
		fg: `chart-series-${n}`,
		bg: ['surface-0', 'surface-1'],
		min: 'ui',
		label: `Chart series ${n}`
	}))
];

/** The pairs no theme may fall below, in either scheme, or it is never used. */
export const FLOORS: readonly { fg: string; bg: string }[] = [
	{ fg: 'text-primary', bg: 'surface-0' },
	{ fg: 'focus-ring-color', bg: 'surface-0' }
];
export const FLOOR = 3;

/** Every token PAIRS reads, for resolveTokens. */
export function pairTokens(pairs: readonly Pair[] = PAIRS): string[] {
	const out = new Set<string>();
	for (const p of pairs) {
		out.add(p.fg);
		for (const b of p.bg) {
			for (const name of typeof b === 'string' ? [b] : b.mix.slice(0, 2)) out.add(name as string);
		}
	}
	return [...out];
}

/** The ratio a need asks for at a target. */
export function needed(min: Need, target: Target): number {
	if (min === 'ui') return 3;
	if (min === 'large') return target === 'AAA' ? 4.5 : 3;
	return target === 'AAA' ? 7 : 4.5;
}

export interface Failure {
	pair: Pair;
	bg: Background;
	ratio: number;
	need: number;
}

const colorOf = (v: Resolved | undefined): Color | null =>
	v && typeof v === 'object' && 'lch' in v ? v : null;

/**
 * A background as a color: a token, or a mix of two worked out as the
 * browser mixes in OKLCH. `swap` puts a color in place of one token's
 * (Fix trying a new color, which a mix it is part of follows).
 */
function background(
	resolved: Map<string, Resolved>,
	bg: Background,
	swap?: [string, Color]
): Color | null {
	const get = (name: string) => (swap && swap[0] === name ? swap[1] : colorOf(resolved.get(name)));
	if (typeof bg === 'string') return get(bg);
	const [a, b, share] = bg.mix;
	const x = get(a);
	const y = get(b);
	if (!x || !y) return null;
	// Hue goes the shorter way round; a gray takes the other's hue.
	let [h1, h2] = [x.lch[2], y.lch[2]];
	if (x.lch[1] < 1e-4) h1 = h2;
	if (y.lch[1] < 1e-4) h2 = h1;
	if (h2 - h1 > 180) h1 += 360;
	else if (h1 - h2 > 180) h2 += 360;
	const mixed: Lch = [
		x.lch[0] * share + y.lch[0] * (1 - share),
		x.lch[1] * share + y.lch[1] * (1 - share),
		(h1 * share + h2 * (1 - share)) % 360
	];
	return { lch: mixed, alpha: 1 };
}

/**
 * The contrast of a foreground on a background, both resolved. A
 * see-through background is laid over the page first, and a see-through
 * foreground over the background.
 */
export function ratio(fg: Color, bg: Color, page?: Color): number {
	const solid: Color =
		bg.alpha < 1 && page ? { lch: linearSrgbToOklch(linear(bg, page)), alpha: 1 } : bg;
	return contrastRatio(linear(fg, solid), oklchToLinearSrgb(...solid.lch));
}

/**
 * Every pair's contrast, passing or not, with what it needs: the editor's
 * badges. Pairs missing a color are skipped.
 */
export function measure(
	resolved: Map<string, Resolved>,
	target: Target,
	pairs: readonly Pair[] = PAIRS
): Failure[] {
	const page = colorOf(resolved.get('surface-0')) ?? undefined;
	const out: Failure[] = [];
	for (const pair of pairs) {
		const fg = colorOf(resolved.get(pair.fg));
		if (!fg) continue;
		const need = needed(pair.min, target);
		for (const bg of pair.bg) {
			const b = background(resolved, bg);
			if (!b) continue;
			out.push({ pair, bg, ratio: ratio(fg, b, page), need });
		}
	}
	return out;
}

/** Every pair below its target, with the ratio it has. Pairs missing a color are skipped. */
export function check(
	resolved: Map<string, Resolved>,
	target: Target,
	pairs: readonly Pair[] = PAIRS
): Failure[] {
	return measure(resolved, target, pairs).filter((m) => m.ratio < m.need);
}

/** The token a background is, or the first of the two it mixes. */
export function bgToken(bg: Background): string {
	return typeof bg === 'string' ? bg : bg.mix[0];
}

/** Whether a scheme of a theme clears the floors. */
export function clearsFloors(resolved: Map<string, Resolved>): boolean {
	return FLOORS.every(({ fg, bg }) => {
		const f = colorOf(resolved.get(fg));
		const b = colorOf(resolved.get(bg));
		return !!f && !!b && ratio(f, b) >= FLOOR;
	});
}

/**
 * A new color for a token that makes every pair it is in pass: moved
 * away from what it is read on (or, for a background, from what is read
 * on it, a mix it is one side of included, worked out again for each
 * color tried), as little as it takes: lighter or darker, whichever is
 * the smaller change, with chroma lowered to stay a real sRGB color. Where
 * neither way gets there, the way that comes closest. The color as it is
 * when it passes already. Null when the token is in no pair, or has no
 * color to start from.
 */
export function fix(
	resolved: Map<string, Resolved>,
	token: string,
	target: Target,
	pairs: readonly Pair[] = PAIRS
): Lch | null {
	const own = colorOf(resolved.get(token));
	if (!own) return null;
	const page = colorOf(resolved.get('surface-0')) ?? undefined;
	// How far past what it needs each pair is with a color tried for the token.
	const margins: ((c: Color) => number)[] = [];
	for (const pair of pairs) {
		const need = needed(pair.min, target) + MARGIN;
		for (const bg of pair.bg) {
			const sides = typeof bg === 'string' ? [bg] : [bg.mix[0], bg.mix[1]];
			if (pair.fg === token && !sides.includes(token)) {
				margins.push((c) => {
					const b = background(resolved, bg);
					return b ? ratio(c, b, page) - need : Infinity;
				});
			} else if (pair.fg !== token && sides.includes(token)) {
				const f = colorOf(resolved.get(pair.fg));
				if (f)
					margins.push((c) => {
						const b = background(resolved, bg, [token, c]);
						return b ? ratio(f, b, token === 'surface-0' ? c : page) - need : Infinity;
					});
			}
		}
	}
	if (!margins.length) return null;
	const worst = (c: Color) => Math.min(...margins.map((m) => m(c)));
	if (worst(own) >= 0) return own.lch;
	const [l0, c0, h] = own.lch;
	const tries = [-0.005, 0.005].map((step) => {
		let best: { lch: Lch; worst: number } | null = null;
		for (let i = 1; i <= 200; i++) {
			const l = Math.min(1, Math.max(0, Math.round((l0 + step * i) * 1000) / 1000));
			let ch = c0;
			while (ch > 0 && !inGamut(oklchToLinearSrgb(l, ch, h))) ch = Math.max(0, ch - 0.005);
			const lch: Lch = [l, Math.floor(ch * 1000) / 1000, h];
			const w = worst({ lch, alpha: own.alpha });
			if (w >= 0) return { lch, worst: w, ok: true, change: i };
			if (!best || w > best.worst) best = { lch, worst: w };
			if (l === 0 || l === 1) break;
		}
		return { ...best!, ok: false, change: Infinity };
	});
	const ok = tries.filter((t) => t.ok).sort((a, b) => a.change - b.change);
	if (ok.length) return ok[0].lch;
	return tries.sort((a, b) => b.worst - a.worst)[0].lch;
}
