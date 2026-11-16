// Mila themes made from Omarchy's (github.com/basecamp/omarchy), one
// from each theme's colors.toml. scripts/omarchy-themes.mjs runs this
// over a folder of Omarchy themes and writes presets/omarchy.json, which
// the app ships; the app never loads this module.
//
// Each theme is a theme document on Standard with every color it needs
// set in both schemes, so nothing depends on the base's arithmetic:
// surfaces, text, borders and a hand-set accent from the palette, the
// label colors from the palette where it has that hue (Mila's hue at the
// palette's lightness where it does not, so urgent stays red), and
// Mila's chart colors. The scheme Omarchy lacks is made from the same
// hues. Then every contrast pair the editor checks is held to AA with the
// editor's own check and Fix, moving only lightness (and chroma where a
// color would leave sRGB); the browser test checks the result again as
// drawn.
import { hexToOklch, inGamut, oklchToLinearSrgb, type Lch, type Scheme } from './color';
import { bgToken, check, fix, PAIRS, pairTokens, type Failure, type Pair } from './contrast';
import { parseTheme, VERSION, type ThemeDoc, type ThemeValue } from './document';
import { PALETTE } from './palette';
import { PRESET_PREFIX } from './presetId';
import { byName } from './tokens';
import type { Preset } from './presets.svelte';
import type { Color, Resolved } from './resolve';

/** What the generator takes from tokens.css: the palette's hues, and the chart colors per scheme. */
export interface Base {
	hues: Record<string, number>;
	chart: Record<Scheme, Lch[]>;
}

/** The palette's hues and the chart colors, read from tokens.css as written. */
export function readBase(tokensCss: string): Base {
	const css = tokensCss.replace(/\/\*[\s\S]*?\*\//g, '');
	const block = (selector: string) => {
		for (const [, sel, body] of css.matchAll(/([^{}]+)\{([^{}]*)\}/g))
			if (sel.trim().replace(/\s+/g, ' ') === selector) return body;
		throw new Error(`tokens.css has no rule for ${selector}`);
	};
	const hues: Record<string, number> = {};
	for (const [, name, n] of block(':root, [data-theme]').matchAll(/--hue-([a-z]+):\s*([\d.]+);/g))
		hues[name] = Number(n);
	const chart = (body: string) =>
		[...body.matchAll(/--chart-series-\d:\s*oklch\(([^)]+)\);/g)].map(
			(m) => m[1].trim().split(/\s+/).map(Number) as Lch
		);
	return {
		hues,
		chart: {
			light: chart(block(":root, [data-scheme='light']")),
			dark: chart(block("[data-scheme='dark']"))
		}
	};
}

/** Display names; a theme not listed is named from its folder. */
const NAMES: Record<string, string> = {
	'flexoki-light': 'Flexoki Light',
	'retro-82': 'Retro 82',
	'rose-pine': 'Rosé Pine'
};

/** Themes Omarchy keeps as two folders that Mila shows as one: dark, then light. */
const PAIRED: Record<string, [string, string]> = {
	catppuccin: ['catppuccin', 'catppuccin-latte']
};

/** Reads a colors.toml: its `key = "value"` lines, keys and values as written. */
export function readColors(toml: string): Record<string, string> {
	const out: Record<string, string> = {};
	for (const line of toml.split('\n')) {
		const m = /^\s*([a-z_]+)\s*=\s*"([^"]*)"\s*$/.exec(line);
		if (m) out[m[1]] = m[2];
	}
	return out;
}

function nameOf(slug: string): string {
	return (
		NAMES[slug] ??
		slug
			.split('-')
			.map((w) => w[0].toUpperCase() + w.slice(1))
			.join(' ')
	);
}

/** A stable id from the folder's name: FNV-1a, twice, for 12 hex digits. */
export function presetId(slug: string): string {
	const hash = (seed: number) => {
		let h = seed;
		for (let i = 0; i < slug.length; i++) {
			h ^= slug.charCodeAt(i);
			h = Math.imul(h, 0x01000193);
		}
		return h >>> 0;
	};
	const a = hash(0x811c9dc5).toString(16).padStart(8, '0');
	const b = hash(0x050c5d1f).toString(16).padStart(8, '0').slice(0, 4);
	return PRESET_PREFIX + a + b;
}

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

/** A color brought into sRGB by lowering its chroma, and rounded as a theme stores it. */
function real([l, c, h]: Lch): Lch {
	l = clamp(l, 0, 1);
	c = clamp(c, 0, 0.37);
	while (c > 0 && !inGamut(oklchToLinearSrgb(l, c, h))) c = Math.max(0, c - 0.002);
	return [round(l, 3), round(c, 3), round(((h % 360) + 360) % 360, 1)];
}

function round(n: number, places: number): number {
	const f = 10 ** places;
	return Math.round(n * f) / f;
}

/** Part of the way from a to b, in OKLCH (hue the shorter way round; a gray takes the other's hue). */
function mix(a: Lch, b: Lch, t: number): Lch {
	let [h1, h2] = [a[2], b[2]];
	if (a[1] < 0.01) h1 = h2;
	if (b[1] < 0.01) h2 = h1;
	if (h2 - h1 > 180) h1 += 360;
	else if (h1 - h2 > 180) h2 += 360;
	return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, (h1 + (h2 - h1) * t) % 360];
}

const hueGap = (a: number, b: number) => {
	const d = Math.abs(a - b) % 360;
	return Math.min(d, 360 - d);
};

function hex(p: Record<string, string>, ...keys: string[]): Lch | null {
	for (const k of keys) {
		const v = p[k] && hexToOklch(p[k]);
		if (v) return v;
	}
	return null;
}

const median = (xs: number[], fallback: number) => {
	if (!xs.length) return fallback;
	const s = [...xs].sort((a, b) => a - b);
	return s[Math.floor(s.length / 2)];
};

/** The palette's own colors, the ones with a hue to speak of. */
function hued(p: Record<string, string>): Lch[] {
	const keys = ['red', 'orange', 'yellow', 'green', 'cyan', 'blue', 'magenta'];
	return [...keys, ...keys.map((k) => `bright_${k}`)]
		.map((k) => hex(p, k))
		.filter((c): c is Lch => !!c && c[1] >= 0.06);
}

/**
 * The twelve label colors: each the palette's own color nearest Mila's
 * hue for it, where one is within 18 degrees, or Mila's hue at the
 * palette's typical lightness and chroma.
 */
function labelColors(
	p: Record<string, string>,
	hues: Record<string, number>,
	scheme: Scheme,
	native: boolean
): Record<string, Lch> {
	const own = hued(p);
	const l = native
		? median(
				own.map((c) => c[0]),
				scheme === 'dark' ? 0.72 : 0.58
			)
		: 0;
	const c = clamp(
		median(
			own.map((x) => x[1]),
			0.13
		),
		0.08,
		0.16
	);
	const out: Record<string, Lch> = {};
	for (const name of PALETTE) {
		const want = hues[name];
		const near = own
			.filter((x) => hueGap(x[2], want) <= 18)
			.sort((a, b) => hueGap(a[2], want) - hueGap(b[2], want))[0];
		if (native) out[name] = near ?? [l, c, want];
		else
			out[name] = [
				scheme === 'dark' ? 0.72 : 0.56,
				near ? clamp(near[1], 0.08, 0.16) : c,
				near?.[2] ?? want
			];
	}
	return out;
}

/** Steps surfaces apart: each at least `step` further from the page than the one before, none past `reach`. */
function stepped(s: Lch[], scheme: Scheme, steps: number[], reach: number): Lch[] {
	const dir = scheme === 'dark' ? 1 : -1;
	const out = [s[0]];
	for (let i = 1; i < s.length; i++) {
		const prev = out[i - 1][0];
		let l =
			dir > 0 ? Math.max(s[i][0], prev + steps[i - 1]) : Math.min(s[i][0], prev - steps[i - 1]);
		l = dir > 0 ? Math.min(l, s[0][0] + reach) : Math.max(l, s[0][0] - reach);
		out.push([l, s[i][1], s[i][2]]);
	}
	return out;
}

/** Moves a color's lightness at least `gap` away from the page's. */
function apart(c: Lch, page: Lch, scheme: Scheme, gap: number): Lch {
	const l = scheme === 'dark' ? Math.max(c[0], page[0] + gap) : Math.min(c[0], page[0] - gap);
	return [l, c[1], c[2]];
}

/** A scheme's starting colors, by token: the palette as Omarchy has it. */
function nativeScheme(p: Record<string, string>, scheme: Scheme, base: Base): Map<string, Lch> {
	const bg = hex(p, 'background')!;
	const lb = hex(p, 'lighter_background') ?? bg;
	const sel = hex(p, 'selection') ?? lb;
	const fg = hex(p, 'foreground')!;
	const dim = hex(p, 'dark_foreground', 'muted') ?? mix(fg, bg, 0.5);
	const muted = hex(p, 'muted') ?? sel;
	const raw =
		scheme === 'dark'
			? [bg, mix(bg, lb, 0.5), lb, mix(lb, sel, 0.5)]
			: [bg, mix(bg, lb, 0.4), mix(bg, lb, 0.7), lb];
	const surfaces = stepped(
		raw,
		scheme,
		scheme === 'dark' ? [0.02, 0.02, 0.03] : [0.015, 0.015, 0.02],
		scheme === 'dark' ? 0.14 : 0.1
	);
	return start(scheme, base, {
		surfaces,
		text: [fg, mix(fg, dim, 0.45), dim],
		border: apart(sel, bg, scheme, 0.07),
		strong: apart(muted, bg, scheme, 0.15),
		accent: hex(p, 'accent') ?? hex(p, 'blue')!,
		labels: labelColors(p, base.hues, scheme, true)
	});
}

/** The scheme Omarchy lacks, made from the same hues on Mila's lightness steps. */
function derivedScheme(p: Record<string, string>, scheme: Scheme, base: Base): Map<string, Lch> {
	const bg = hex(p, 'background')!;
	const fg = hex(p, 'foreground')!;
	const accent = hex(p, 'accent') ?? hex(p, 'blue')!;
	const h = bg[2];
	const c = clamp(bg[1] * 0.6, 0, 0.02);
	const tint = clamp(fg[1], 0, 0.03);
	const at = (l: number, ch = c, hue = h): Lch => [l, ch, hue];
	const dark = scheme === 'dark';
	return start(scheme, base, {
		surfaces: (dark ? [0.19, 0.215, 0.245, 0.285] : [0.992, 0.972, 0.952, 0.92]).map((l) =>
			at(l, dark ? Math.max(c, 0.012) : c)
		),
		text: (dark ? [0.93, 0.78, 0.68] : [0.25, 0.42, 0.5]).map((l) => at(l, tint, fg[2])),
		border: at(dark ? 0.31 : 0.9),
		strong: at(dark ? 0.4 : 0.82),
		accent: [
			dark ? Math.max(accent[0], 0.72) : Math.min(accent[0], 0.55),
			Math.min(accent[1], 0.16),
			accent[2]
		],
		labels: labelColors(p, base.hues, scheme, false)
	});
}

interface Start {
	surfaces: Lch[];
	text: Lch[];
	border: Lch;
	strong: Lch;
	accent: Lch;
	labels: Record<string, Lch>;
}

function start(scheme: Scheme, base: Base, s: Start): Map<string, Lch> {
	const v = new Map<string, Lch>();
	const page = s.surfaces[0];
	s.surfaces.forEach((c, i) => v.set(`surface-${i}`, c));
	v.set('text-primary', s.text[0]);
	v.set('text-secondary', s.text[1]);
	v.set('text-muted', s.text[2]);
	v.set('border', s.border);
	v.set('border-strong', s.strong);
	v.set('border-control', mix(page, s.text[2], 0.7));
	v.set('accent', s.accent);
	v.set('accent-hover', [s.accent[0] - 0.04, s.accent[1], s.accent[2]]);
	v.set('accent-text', s.accent);
	v.set('accent-soft', mix(page, s.accent, 0.16));
	// Whichever of the page, near-white and near-black reads best on the accent.
	const ink = [page, [0.995, 0, 0] as Lch, [0.18, 0.01, s.accent[2]] as Lch];
	v.set('accent-contrast', ink.sort((a, b) => contrast(b, s.accent) - contrast(a, s.accent))[0]);
	for (const [name, c] of Object.entries(s.labels)) {
		v.set(`palette-${name}`, c);
		const share = scheme === 'dark' ? 0.25 : 0.12;
		v.set(`palette-${name}-soft`, [
			page[0] + (c[0] - page[0]) * share,
			c[1] * (scheme === 'dark' ? 0.4 : 0.32),
			c[2]
		]);
	}
	v.set('danger', s.labels.red);
	base.chart[scheme].forEach((c, i) => v.set(`chart-series-${i + 1}`, c));
	for (const [k, c] of v) v.set(k, real(c));
	return v;
}

function contrast(a: Lch, b: Lch): number {
	const lum = (c: Lch) => {
		const [r, g, bl] = oklchToLinearSrgb(...c).map((x) => clamp(x, 0, 1));
		return 0.2126 * r + 0.7152 * g + 0.0722 * bl;
	};
	const [x, y] = [lum(a), lum(b)];
	return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

/** The token a token comes from: itself where the theme sets it, or what it refers to. */
function root(name: string, set: Map<string, Lch>): string {
	for (let n = name, i = 0; i < 10; i++) {
		if (set.has(n)) return n;
		const ref = byName(n)?.default?.ref;
		if (!ref) throw new Error(`${name}: the theme sets no color for ${n}`);
		n = ref;
	}
	throw new Error(`${name}: refs loop`);
}

/** The pairs with every token replaced by the one it comes from, so moving one moves all that follow it. */
function rootPairs(set: Map<string, Lch>): Pair[] {
	return PAIRS.map((p) => ({
		...p,
		fg: root(p.fg, set),
		bg: p.bg.map((b) =>
			typeof b === 'string'
				? root(b, set)
				: {
						...b,
						mix: [root(b.mix[0], set), root(b.mix[1], set), b.mix[2]] as [string, string, number]
					}
		)
	}));
}

const resolved = (set: Map<string, Lch>) =>
	new Map<string, Resolved>([...set].map(([k, lch]): [string, Color] => [k, { lch, alpha: 1 }]));

/** What text is set on: moved last, so a theme keeps its page and its text where it can. */
const ANCHORS = new Set(['surface-0', 'surface-1', 'surface-2', 'surface-3', 'text-primary']);

/**
 * Holds every pair to AA: for each that falls short, Fix moves what is
 * read (or, where that is the page or its text, what it sits on) until
 * it reads, as little as it takes. Returns the failures left, if any.
 */
export function settle(set: Map<string, Lch>): string[] {
	for (const name of pairTokens()) root(name, set);
	const pairs = rootPairs(set);
	for (let round = 0; round < 60; round++) {
		const fails = check(resolved(set), 'AA', pairs);
		if (!fails.length) return [];
		const f = fails[0];
		const bg = bgToken(f.bg);
		const token = ANCHORS.has(f.pair.fg) && !ANCHORS.has(bg) ? bg : f.pair.fg;
		const next = fix(resolved(set), token, 'AA', pairs);
		if (!next) break;
		const was = set.get(token)!;
		set.set(token, real(next));
		if (set.get(token)!.every((x, i) => x === was[i])) {
			// Fix could do no better for this token: try the other side once.
			const other = token === bg ? f.pair.fg : bg;
			const alt = fix(resolved(set), other, 'AA', pairs);
			if (alt) set.set(other, real(alt));
		}
	}
	return check(resolved(set), 'AA', pairs).map(said);
}

const said = (f: Failure) =>
	`${f.pair.label} on ${bgToken(f.bg)}: ${f.ratio.toFixed(2)} of ${f.need}`;

/** Every pair below AA in one scheme of a theme made here, worked out as the generator does. */
export function shortfalls(doc: ThemeDoc, scheme: Scheme): string[] {
	const set = new Map<string, Lch>();
	for (const [k, v] of Object.entries(doc[scheme])) if ('oklch' in v) set.set(k, v.oklch);
	return check(resolved(set), 'AA', rootPairs(set)).map(said);
}

/**
 * Keeps secondary text between text and muted text: lifting muted text to
 * AA can carry it past secondary, which would then read as the fainter.
 */
function ordered(set: Map<string, Lch>) {
	const [primary, secondary, muted] = ['text-primary', 'text-secondary', 'text-muted'].map((k) =>
		set.get(k)!
	);
	const toward = primary[0] - muted[0];
	const least = muted[0] + toward / 3;
	if ((secondary[0] - least) * Math.sign(toward) < 0)
		set.set('text-secondary', real([least, secondary[1], secondary[2]]));
}

const values = (set: Map<string, Lch>): Record<string, ThemeValue> =>
	Object.fromEntries([...set].map(([k, lch]) => [k, { oklch: lch }]));

/**
 * The Mila themes for a folder of Omarchy palettes, by folder name, in
 * name order. Throws if one cannot be held to AA.
 */
export function buildPresets(
	palettes: Record<string, Record<string, string>>,
	base: Base
): Preset[] {
	const second = new Set(Object.values(PAIRED).map(([, light]) => light));
	const out: Preset[] = [];
	for (const slug of Object.keys(palettes).sort()) {
		if (second.has(slug)) continue;
		const p = palettes[slug];
		if (!p.background || !p.foreground) continue;
		let light: Map<string, Lch>;
		let dark: Map<string, Lch>;
		let mode: Preset['mode'];
		const pair = PAIRED[slug];
		if (pair && palettes[pair[1]]) {
			dark = nativeScheme(palettes[pair[0]], 'dark', base);
			light = nativeScheme(palettes[pair[1]], 'light', base);
			mode = 'both';
		} else {
			mode = p.mode === 'light' ? 'light' : 'dark';
			dark = mode === 'dark' ? nativeScheme(p, 'dark', base) : derivedScheme(p, 'dark', base);
			light = mode === 'light' ? nativeScheme(p, 'light', base) : derivedScheme(p, 'light', base);
		}
		for (const [scheme, set] of [
			['light', light],
			['dark', dark]
		] as const) {
			settle(set);
			ordered(set);
			const left = settle(set);
			if (left.length) throw new Error(`${slug} ${scheme}: ${left.join('; ')}`);
		}
		const page = (mode === 'light' ? light : dark).get('surface-0')!;
		const doc: ThemeDoc = {
			mila: 'theme',
			version: VERSION,
			id: presetId(slug),
			name: nameOf(slug),
			base: 'standard',
			target: 'AA',
			accent: { manual: true },
			tokens: {
				'neutral-hue': { hue: round(page[2], 1) },
				'neutral-chroma': { n: round(clamp(page[1], 0, 0.03), 3) }
			},
			light: values(light),
			dark: values(dark)
		};
		// What the app reads back must be what was written.
		const { theme } = parseTheme(doc, 'stored');
		if (JSON.stringify(theme) !== JSON.stringify(doc))
			throw new Error(`${slug}: does not read back`);
		out.push({ slug, mode, doc });
	}
	return out;
}
