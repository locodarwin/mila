// Token values as the browser works them out on an element: refs,
// calc() and color-mix() resolved exactly as they are drawn. A theme
// preview container (data-theme, data-scheme, data-density and
// data-font-size, plus the theme's inline values) gets the whole set for
// itself, so this reads any theme in either scheme without touching the
// page.
import { linearSrgbToOklch, oklchToLinearSrgb, srgbToOklch, toLinear, type Lch } from './color';
import { byName } from './tokens';

/** A resolved color: OKLCH and its opacity (1 when solid). */
export interface Color {
	lch: Lch;
	alpha: number;
}

export type Resolved = Color | number | string;

const NUMBER = String.raw`[+-]?(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?`;

/** A CSS number, percentage (as a share of `whole`) or `none` (0). */
function part(s: string, whole = 1): number | null {
	const t = s.trim().toLowerCase();
	if (t === 'none') return 0;
	const m = new RegExp(`^(${NUMBER})(%|deg)?$`).exec(t);
	if (!m) return null;
	const n = Number(m[1]);
	if (!Number.isFinite(n)) return null;
	return m[2] === '%' ? (n / 100) * whole : n;
}

function parts(body: string): { values: string[]; alpha: string | null } | null {
	// Either "a, b, c[, d]" or "a b c[ / d]".
	if (body.includes(',')) {
		const v = body.split(',').map((s) => s.trim());
		if (v.length !== 3 && v.length !== 4) return null;
		return { values: v.slice(0, 3), alpha: v[3] ?? null };
	}
	const [main, alpha, extra] = body.split('/');
	if (extra !== undefined) return null;
	const v = main.trim().split(/\s+/);
	return { values: v, alpha: alpha?.trim() ?? null };
}

/**
 * A color as getComputedStyle writes it: oklch(), oklab(), rgb() or
 * rgba(), or color(srgb ...) / color(srgb-linear ...), as different
 * browsers serialize. Null for anything else.
 */
export function parseColor(input: string): Color | null {
	const s = input.trim().toLowerCase();
	if (s === 'transparent') return { lch: [0, 0, 0], alpha: 0 };
	const m = /^([a-z-]+)\((.*)\)$/.exec(s);
	if (!m) return null;
	const [, fn, inner] = m;
	let space = fn;
	let body = inner;
	if (fn === 'color') {
		const c = /^(srgb|srgb-linear)\s+(.*)$/.exec(inner.trim());
		if (!c) return null;
		space = c[1];
		body = c[2];
	}
	const p = parts(body);
	if (!p || p.values.length !== 3) return null;
	const alpha = p.alpha === null ? 1 : part(p.alpha);
	if (alpha === null) return null;
	const [a, b, c] = p.values;
	let lch: Lch;
	switch (space) {
		case 'oklch': {
			const l = part(a);
			const ch = part(b, 0.4);
			const h = part(c);
			if (l === null || ch === null || h === null) return null;
			lch = [l, ch, ((h % 360) + 360) % 360];
			break;
		}
		case 'oklab': {
			const l = part(a);
			const x = part(b, 0.4);
			const y = part(c, 0.4);
			if (l === null || x === null || y === null) return null;
			const ch = Math.hypot(x, y);
			lch = [l, ch, ch < 1e-4 ? 0 : ((Math.atan2(y, x) * 180) / Math.PI + 360) % 360];
			break;
		}
		case 'rgb':
		case 'rgba': {
			const v = [a, b, c].map((x) => part(x, 255));
			if (v.some((x) => x === null)) return null;
			const [r, g, bl] = (v as number[]).map((x) => x / 255);
			lch = srgbToOklch(r, g, bl);
			break;
		}
		case 'srgb':
		case 'srgb-linear': {
			const v = [a, b, c].map((x) => part(x));
			if (v.some((x) => x === null)) return null;
			const [r, g, bl] = v as number[];
			lch = space === 'srgb' ? srgbToOklch(r, g, bl) : linearSrgbToOklch([r, g, bl]);
			break;
		}
		default:
			return null;
	}
	return { lch, alpha: Math.min(1, Math.max(0, alpha)) };
}

/** A color in linear light, laid over a solid background when it is see-through. */
export function linear(color: Color, over?: Color): [number, number, number] {
	const fg = oklchToLinearSrgb(...color.lch);
	if (color.alpha >= 1 || !over) return fg;
	const bg = oklchToLinearSrgb(...over.lch);
	// Blending happens on the encoded values, as browsers draw.
	const enc = (v: number) =>
		v <= 0.0031308 ? 12.92 * v : 1.055 * Math.max(0, v) ** (1 / 2.4) - 0.055;
	return fg.map((v, i) => toLinear(enc(v) * color.alpha + enc(bg[i]) * (1 - color.alpha))) as [
		number,
		number,
		number
	];
}

/**
 * Reads tokens on an element: colors through a hidden child's color (so
 * the browser resolves var(), calc() and color-mix()), sizes through its
 * width, durations through its transition, and the rest as written.
 * Names the registry does not know are read as text.
 */
export function resolveTokens(el: HTMLElement, names: Iterable<string>): Map<string, Resolved> {
	const out = new Map<string, Resolved>();
	const probe = document.createElement('span');
	probe.setAttribute('aria-hidden', 'true');
	probe.style.cssText =
		'position:absolute;visibility:hidden;pointer-events:none;display:block;box-sizing:content-box;';
	el.append(probe);
	try {
		const style = getComputedStyle(probe);
		const own = getComputedStyle(el);
		for (const name of names) {
			const meta = byName(name);
			const raw = own.getPropertyValue(`--${name}`).trim();
			switch (meta?.type) {
				case 'color': {
					probe.style.color = `var(--${name})`;
					const c = parseColor(style.color);
					if (c) out.set(name, c);
					break;
				}
				case 'px': {
					probe.style.width = `var(--${name})`;
					const n = parseFloat(style.width);
					if (Number.isFinite(n)) out.set(name, n);
					break;
				}
				case 'ms': {
					probe.style.transitionDuration = `var(--${name})`;
					const d = /^([\d.]+)(m?s)/.exec(style.transitionDuration);
					if (d) out.set(name, Number(d[1]) * (d[2] === 's' ? 1000 : 1));
					break;
				}
				case 'number':
				case 'hue': {
					const n = parseFloat(raw);
					if (Number.isFinite(n)) out.set(name, n);
					break;
				}
				default:
					out.set(name, raw);
			}
		}
	} finally {
		probe.remove();
	}
	return out;
}
