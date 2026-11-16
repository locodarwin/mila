// OKLCH color math for the accent picker: conversion to sRGB, gamut checks
// and WCAG contrast. Formulas from Björn Ottosson's OKLab definition.

export type Scheme = 'light' | 'dark';

/** Linear-light sRGB channels, each 0..1 when in gamut. */
export type LinearRGB = [number, number, number];

export function oklchToLinearSrgb(l: number, c: number, hDeg: number): LinearRGB {
	const h = (hDeg * Math.PI) / 180;
	const a = c * Math.cos(h);
	const b = c * Math.sin(h);
	const l_ = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
	const m_ = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
	const s_ = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
	return [
		4.0767416621 * l_ - 3.3077115913 * m_ + 0.2309699292 * s_,
		-1.2684380046 * l_ + 2.6097574011 * m_ - 0.3413193965 * s_,
		-0.0041960863 * l_ - 0.7034186147 * m_ + 1.707614701 * s_
	];
}

/** OKLCH from linear-light sRGB: the inverse of oklchToLinearSrgb. */
export function linearSrgbToOklch([r, g, b]: LinearRGB): Lch {
	const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
	const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
	const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
	const L = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s;
	const A = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s;
	const B = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s;
	const c = Math.hypot(A, B);
	// A gray has no hue; 0 keeps it stable.
	const h = c < 1e-4 ? 0 : ((Math.atan2(B, A) * 180) / Math.PI + 360) % 360;
	return [L, c < 1e-4 ? 0 : c, h];
}

/** One sRGB channel (0..1, as CSS and hex write it) in linear light. */
export function toLinear(v: number): number {
	return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
}

/**
 * OKLCH from sRGB channels as CSS and hex write them (0..1 each), for a
 * hex field or <input type="color">.
 */
export function srgbToOklch(r: number, g: number, b: number): Lch {
	return linearSrgbToOklch([toLinear(r), toLinear(g), toLinear(b)]);
}

/** OKLCH from #rgb or #rrggbb, or null. */
export function hexToOklch(hex: string): Lch | null {
	const m = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(hex.trim());
	if (!m) return null;
	const h = m[1].length === 3 ? [...m[1]].map((x) => x + x).join('') : m[1];
	const [r, g, b] = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
	return srgbToOklch(r, g, b);
}

export function inGamut([r, g, b]: LinearRGB, epsilon = 1e-4): boolean {
	return [r, g, b].every((v) => v >= -epsilon && v <= 1 + epsilon);
}

/** WCAG relative luminance of a linear sRGB color. */
export function luminance([r, g, b]: LinearRGB): number {
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrastRatio(a: LinearRGB, b: LinearRGB): number {
	const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	return (hi + 0.05) / (lo + 0.05);
}

/** Text on the accent: near-white in the light scheme, near-black in dark. */
export function accentTextColor(scheme: Scheme, hue: number): LinearRGB {
	return scheme === 'light' ? oklchToLinearSrgb(0.995, 0, 0) : oklchToLinearSrgb(0.18, 0.01, hue);
}

export interface Accent {
	l: number;
	c: number;
}

/** The accent, and its lightness and chroma as text or a link on a surface. */
export interface AccentColors extends Accent {
	textL: number;
	textC: number;
}

/** The theme characters, as tokens.css names them. */
export type Character = 'standard' | 'calm' | 'playful' | 'contrast';

const START: Record<Scheme, Accent> = {
	light: { l: 0.56, c: 0.17 },
	dark: { l: 0.72, c: 0.14 }
};

/** WCAG AA for normal text. */
export const MIN_CONTRAST = 4.5;
/** WCAG AAA for normal text, which High contrast aims for. */
export const AAA_CONTRAST = 7;

/** The contrast a theme's accent and accent text must reach. */
export function contrastTarget(character: Character): number {
	return character === 'contrast' ? AAA_CONTRAST : MIN_CONTRAST;
}

/** The same for a custom theme's contrast goal. */
export function targetContrast(target: 'AA' | 'AAA'): number {
	return target === 'AAA' ? AAA_CONTRAST : MIN_CONTRAST;
}

// The browser rounds each color to 8 bits a channel, which can take a
// hair off the contrast worked out here; aiming this much higher keeps
// what is drawn above the target.
export const MARGIN = 0.08;

/** A color as OKLCH: lightness 0..1, chroma, hue in degrees. */
export type Lch = [number, number, number];
const neutral = (hue: number, chroma: number, ls: number[]): Lch[] =>
	ls.map((l) => [l, chroma, hue]);

/**
 * surface-0 to surface-3 for each character and scheme, as tokens.css sets
 * them (surfaces.spec.ts checks the two agree). Accent text has to be
 * readable on every one.
 */
export const SURFACES: Record<Character, Record<Scheme, Lch[]>> = {
	standard: {
		light: neutral(260, 0.006, [0.995, 0.975, 0.955, 0.925]),
		dark: neutral(260, 0.006, [0.18, 0.21, 0.245, 0.285])
	},
	calm: {
		light: [
			[0.985, 0.012, 85],
			[0.965, 0.016, 82],
			[0.945, 0.018, 80],
			[0.915, 0.02, 78]
		],
		dark: [
			[0.2, 0.012, 70],
			[0.225, 0.014, 70],
			[0.255, 0.016, 70],
			[0.29, 0.018, 70]
		]
	},
	playful: {
		light: neutral(295, 0.012, [0.995, 0.975, 0.955, 0.925]),
		dark: neutral(295, 0.012, [0.18, 0.21, 0.245, 0.285])
	},
	contrast: {
		light: neutral(0, 0, [1, 1, 0.96, 0.92]),
		dark: neutral(0, 0, [0, 0, 0.16, 0.24])
	}
};

/** The share of the accent in --accent-soft, mixed into surface-0. */
const SOFT_SHARE = 0.14;

/**
 * Lowers chroma until the color is a real sRGB color, to three places, so
 * the numbers checked are the numbers the page uses.
 */
function fit(l: number, c: number, hue: number): number {
	while (c > 0 && !inGamut(oklchToLinearSrgb(l, c, hue))) c = Math.max(0, c - 0.005);
	return Math.floor(c * 1000) / 1000;
}

/**
 * Picks the accent lightness and chroma for a hue so that the color is a
 * real sRGB color and text on it reaches the theme's target (AA, or AAA
 * in High contrast). Chroma is lowered until the color fits the gamut;
 * lightness moves away from the text color until the contrast passes.
 */
export function accentFor(hue: number, scheme: Scheme, character: Character = 'standard'): Accent {
	return accentOn(hue, scheme, contrastTarget(character), accentTextColor(scheme, hue));
}

/** accentFor, for a given contrast and color of text on the accent. */
function accentOn(hue: number, scheme: Scheme, target: number, text: LinearRGB): Accent {
	const { l, c } = START[scheme];
	return walk(l, c, hue, [text], target, scheme === 'light' ? -0.005 : 0.005);
}

/**
 * Moves a color's lightness by `step` at a time, lowering chroma to stay
 * a real sRGB color, until it reaches its target (plus the margin)
 * against every color given, or 200 steps have passed. The target is one
 * for all, or one for each color.
 */
function walk(
	l: number,
	c: number,
	hue: number,
	against: LinearRGB[],
	target: number | number[],
	step: number
): Accent {
	for (let i = 0; i < 200; i++) {
		l = round(l);
		c = fit(l, c, hue);
		if (passes([l, c, hue], against, target)) break;
		l += step;
	}
	return { l, c };
}

function passes([l, c, h]: Lch, against: LinearRGB[], target: number | number[]): boolean {
	const color = oklchToLinearSrgb(l, c, h);
	return against.every(
		(b, i) => contrastRatio(color, b) >= (typeof target === 'number' ? target : target[i]) + MARGIN
	);
}

/**
 * The nearest color to `lch` (lighter or darker, whichever is the smaller
 * change) that reaches `target` against every color given (one target for
 * all, or one for each), for a theme's Fix. Where neither way gets there,
 * the way that comes closest.
 */
export function nudge(lch: Lch, against: LinearRGB[], target: number | number[]): Lch {
	const [l0, c0, h] = lch;
	if (passes(lch, against, target)) return lch;
	const tries = [-0.005, 0.005].map((step) => {
		const { l, c } = walk(l0, c0, h, against, target, step);
		const clamped = Math.min(1, Math.max(0, l));
		const color: Lch = [clamped, fit(clamped, c, h), h];
		const need = (i: number) => (typeof target === 'number' ? target : target[i]);
		const worst = Math.min(
			...against.map((b, i) => contrastRatio(oklchToLinearSrgb(...color), b) - need(i))
		);
		return { color, ok: passes(color, against, target), worst, change: Math.abs(clamped - l0) };
	});
	const ok = tries.filter((t) => t.ok).sort((a, b) => a.change - b.change);
	if (ok.length) return ok[0].color;
	return tries.sort((a, b) => b.worst - a.worst)[0].color;
}

/**
 * The accent and the accent as text: text starts a little darker (light
 * scheme) or lighter (dark) than the accent, as tokens.css shifts it, and
 * moves further until it is readable on each of the theme's surfaces and
 * on --accent-soft.
 */
export function accentColors(hue: number, scheme: Scheme, character?: Character): AccentColors;
/**
 * The same for a custom theme: its contrast target, its own surfaces
 * (surface-0 first, as the browser works them out) and, when it sets
 * one, its own color for text on the accent.
 */
export function accentColors(
	hue: number,
	scheme: Scheme,
	target: number,
	surfaces: Lch[],
	onAccent?: LinearRGB
): AccentColors;
export function accentColors(
	hue: number,
	scheme: Scheme,
	characterOrTarget: Character | number = 'standard',
	ownSurfaces?: Lch[],
	onAccent?: LinearRGB
): AccentColors {
	const target =
		typeof characterOrTarget === 'number' ? characterOrTarget : contrastTarget(characterOrTarget);
	const surfaces =
		typeof characterOrTarget === 'number' ? ownSurfaces! : SURFACES[characterOrTarget][scheme];
	const accent = accentOn(hue, scheme, target, onAccent ?? accentTextColor(scheme, hue));
	const [l0, c0] = surfaces[0];
	// --accent-soft, mixed in OKLCH; the surfaces are near gray, so the
	// mix keeps the accent's hue.
	const soft: Lch = [
		accent.l * SOFT_SHARE + l0 * (1 - SOFT_SHARE),
		accent.c * SOFT_SHARE + c0 * (1 - SOFT_SHARE),
		hue
	];
	const backgrounds = [...surfaces, soft].map(([l, c, h]) => oklchToLinearSrgb(l, c, h));
	const step = scheme === 'light' ? -0.005 : 0.005;
	const text = walk(
		accent.l + (scheme === 'light' ? -0.12 : 0.08),
		accent.c,
		hue,
		backgrounds,
		target,
		step
	);
	return { ...accent, textL: text.l, textC: text.c };
}

function round(v: number): number {
	return Math.round(v * 1000) / 1000;
}

/** A linear sRGB color as #rrggbb, clipped to the gamut. */
export function toHex(rgb: LinearRGB): string {
	return (
		'#' +
		rgb
			.map((v) => {
				const x = Math.min(1, Math.max(0, v));
				const s = x <= 0.0031308 ? 12.92 * x : 1.055 * x ** (1 / 2.4) - 0.055;
				return Math.round(s * 255)
					.toString(16)
					.padStart(2, '0');
			})
			.join('')
	);
}

/**
 * The theme's page color (surface-0) as hex, for the browser's own chrome:
 * the theme-color metas, which do not read CSS variables.
 */
export function pageColor(character: Character, scheme: Scheme): string {
	const [l, c, h] = SURFACES[character][scheme][0];
	return toHex(oklchToLinearSrgb(l, c, h));
}
