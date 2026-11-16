// A custom theme as a document: what it is based on, and only the tokens
// it changes. The server keeps the same rules (accounts/themes.go), and
// shared/theme-vectors.json holds the two to the same answers.
//
// parseTheme is the one way in. It never trusts what it reads: it reads
// only an object's own keys, never __proto__, constructor or prototype,
// and every value it keeps is a fresh copy. A theme that is drawn goes
// through it again (css.ts), so even a stored one cannot reach the CSS
// unchecked.
import { uuid } from '../uuid';
import type { Character } from './color';
import { byName, RENAMED, type TokenMeta } from './tokens';

export const VERSION = 1;

export const LIMITS = {
	/** Characters in a name, after control characters are taken out. */
	name: 60,
	/** Bytes of a theme as compact JSON. */
	bytes: 16 * 1024,
	/** Themes per person. */
	themes: 20,
	/** Bytes of a file or text to import. */
	importBytes: 64 * 1024
};

export const BASES: readonly Character[] = ['standard', 'calm', 'playful', 'contrast'];
export const TARGETS = ['AA', 'AAA'] as const;
export type Target = (typeof TARGETS)[number];

/** The faces a theme may use: the four the app bundles, and three system stacks. */
export const FONTS = {
	inter: "'Inter Variable', system-ui, sans-serif",
	nunito: "'Nunito Variable', system-ui, sans-serif",
	'source-serif': "'Source Serif 4 Variable', Georgia, serif",
	'jetbrains-mono': "'JetBrains Mono Variable', ui-monospace, monospace",
	'system-sans': 'system-ui, sans-serif',
	'system-serif': 'ui-serif, Georgia, serif',
	'system-mono': 'ui-monospace, monospace'
} as const;
export type FontId = keyof typeof FONTS;

/** The curves a theme may pick by name; any other is a cubic bezier. */
export const EASES = {
	standard: 'cubic-bezier(0.2, 0.8, 0.2, 1)',
	gentle: 'cubic-bezier(0.25, 0.6, 0.3, 1)',
	spring: 'linear(0, 0.18 6%, 0.6 18%, 0.98 32%, 1.08 42%, 1.06 52%, 1.01 66%, 0.995 80%, 1)',
	snappy: 'cubic-bezier(0.3, 0.9, 0.1, 1)',
	linear: 'linear'
} as const;
export type EaseId = keyof typeof EASES;

export interface ShadowLayer {
	y: number;
	blur: number;
	spread: number;
}

export type ThemeValue =
	| { oklch: [number, number, number]; alpha?: number }
	| { ref: string }
	| { px: number }
	| { ms: number }
	| { n: number }
	| { hue: number }
	| { font: FontId }
	| { ease: EaseId }
	| { bezier: [number, number, number, number] }
	| { layers: ShadowLayer[] };

/** The accent: generated from a hue (the base's when none), or set by hand per scheme. */
export type ThemeAccent = { hue?: number } | { manual: true };

export interface ThemeDoc {
	mila: 'theme';
	version: number;
	id: string;
	name: string;
	base: Character;
	target: Target;
	accent: ThemeAccent;
	/** Tokens the same in both schemes. */
	tokens: Record<string, ThemeValue>;
	light: Record<string, ThemeValue>;
	dark: Record<string, ThemeValue>;
}

export type Map3 = 'tokens' | 'light' | 'dark';
export const MAPS: readonly Map3[] = ['tokens', 'light', 'dark'];

/** A theme that cannot be read at all; the message is for people. */
export class ThemeError extends Error {}

/**
 * UPGRADES[n] rewrites a theme of version n into version n + 1 (the
 * sync/outbox.ts pattern). A change of shape raises VERSION and adds the
 * step, here and in accounts/themes.go.
 */
export const UPGRADES: Record<number, (doc: Record<string, unknown>) => Record<string, unknown>> =
	{};

const FIELDS = ['mila', 'version', 'id', 'name', 'base', 'target', 'accent', ...MAPS];
// Keys that are never read, whatever object they are in.
const FORBIDDEN = new Set(['__proto__', 'constructor', 'prototype']);
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => v !== null && typeof v === 'object' && !Array.isArray(v);
const own = (o: Obj, k: string) => Object.hasOwn(o, k);
const keys = (o: Obj) => Object.keys(o).filter((k) => !FORBIDDEN.has(k));
const num = (v: unknown, lo: number, hi: number): v is number =>
	typeof v === 'number' && Number.isFinite(v) && v >= lo && v <= hi;
const only = (o: Obj, allowed: string[]) => Object.keys(o).every((k) => allowed.includes(k));

/** A name with control characters taken out and the ends trimmed. */
export function cleanName(name: string): string {
	// eslint-disable-next-line no-control-regex
	return name.replace(/[\u0000-\u001f\u007f-\u009f]/g, '').trim();
}

const length = (s: string) => [...s].length;

/** A name cut to the limit, by characters. */
function clip(name: string): string {
	return [...name].slice(0, LIMITS.name).join('').trim();
}

/** The name, made unique among the ones given: "Night shift (2)". */
export function uniqueName(name: string, taken: readonly string[]): string {
	const used = new Set(taken.map((n) => n.toLowerCase()));
	if (!used.has(name.toLowerCase())) return name;
	for (let i = 2; ; i++) {
		const suffix = ` (${i})`;
		const candidate = clip([...name].slice(0, LIMITS.name - suffix.length).join('')) + suffix;
		if (!used.has(candidate.toLowerCase())) return candidate;
	}
}

/**
 * The value for a token, as a fresh copy, or null when it is not one the
 * token can take.
 */
export function parseValue(meta: TokenMeta, v: unknown): ThemeValue | null {
	if (!isObj(v)) return null;
	const k = Object.keys(v);
	if (k.length === 1 && k[0] === 'ref') {
		const name = v.ref;
		if (typeof name !== 'string') return null;
		const target = byName(name);
		if (
			!target ||
			target.type !== meta.type ||
			target.perScheme !== meta.perScheme ||
			target.rank >= meta.rank ||
			(target.alpha && !meta.alpha)
		)
			return null;
		return { ref: target.name };
	}
	const one = (key: string) => (k.length === 1 && k[0] === key ? v[key] : undefined);
	switch (meta.type) {
		case 'color': {
			if (!own(v, 'oklch') || !only(v, ['oklch', 'alpha'])) return null;
			const c = v.oklch;
			if (!Array.isArray(c) || c.length !== 3) return null;
			const [l, ch, h] = c;
			if (!num(l, 0, 1) || !num(ch, 0, 0.4) || !num(h, 0, 360)) return null;
			if (!own(v, 'alpha')) return { oklch: [l, ch, h] };
			if (!meta.alpha || !num(v.alpha, 0, 1)) return null;
			return { oklch: [l, ch, h], alpha: v.alpha };
		}
		case 'px':
		case 'ms':
		case 'number': {
			const key = meta.type === 'number' ? 'n' : meta.type;
			const n = one(key);
			if (!num(n, meta.min!, meta.max!)) return null;
			return { [key]: n } as ThemeValue;
		}
		case 'hue': {
			const h = one('hue');
			return num(h, 0, 360) ? { hue: h } : null;
		}
		case 'font': {
			const f = one('font');
			return typeof f === 'string' && Object.hasOwn(FONTS, f) ? { font: f as FontId } : null;
		}
		case 'ease': {
			const e = one('ease');
			if (typeof e === 'string') return Object.hasOwn(EASES, e) ? { ease: e as EaseId } : null;
			const b = one('bezier');
			if (!Array.isArray(b) || b.length !== 4) return null;
			const [x1, y1, x2, y2] = b;
			if (!num(x1, 0, 1) || !num(y1, -1, 2) || !num(x2, 0, 1) || !num(y2, -1, 2)) return null;
			return { bezier: [x1, y1, x2, y2] };
		}
		case 'shadow': {
			const layers = one('layers');
			if (!Array.isArray(layers) || layers.length > 3) return null;
			const out: ShadowLayer[] = [];
			for (const l of layers) {
				if (!isObj(l) || Object.keys(l).length !== 3) return null;
				const { y, blur, spread } = l;
				if (!num(y, -32, 64) || !num(blur, 0, 128) || !num(spread, -32, 32)) return null;
				out.push({ y, blur, spread });
			}
			return { layers: out };
		}
	}
	return null;
}

/** Whether a token belongs in a map: per-scheme ones in light and dark, the rest in tokens. */
export function belongs(meta: TokenMeta, map: Map3): boolean {
	return meta.perScheme === (map !== 'tokens');
}

const bytes = (s: string) => new TextEncoder().encode(s).length;

function plural(n: number, one: string, many: string) {
	return n === 1 ? one : many;
}

export interface ParseOptions {
	/** Import only: names already in use, so the new one differs. */
	taken?: readonly string[];
}

/**
 * Reads a theme.
 *
 * import: a file or pasted text. Whatever cannot be used is left out
 * and said in the notes; it gets a new id and a name of its own.
 *
 * stored: from the server or this device's cache. Tokens it does not
 * know, or values it cannot use, are kept as they are, so a tab older than
 * the theme sends them back untouched when it saves (the server checks
 * them); css.ts leaves them out when drawing.
 *
 * Throws ThemeError when there is no theme to read.
 */
export function parseTheme(
	raw: unknown,
	mode: 'import' | 'stored',
	options: ParseOptions = {}
): { theme: ThemeDoc; notes: string[] } {
	const importing = mode === 'import';
	let v = raw;
	if (typeof raw === 'string') {
		if (importing && bytes(raw) > LIMITS.importBytes)
			throw new ThemeError('This file is too large to be a Mila theme (64 KiB at most).');
		try {
			v = JSON.parse(raw);
		} catch {
			throw new ThemeError('This is not a Mila theme: it is not JSON.');
		}
	}
	if (!isObj(v) || v.mila !== 'theme') throw new ThemeError('This is not a Mila theme.');
	const notes: string[] = [];
	let version = v.version;
	if (typeof version !== 'number' || !Number.isInteger(version) || version < 1)
		throw new ThemeError('This theme has no version Mila can read.');
	let doc: Obj = v;
	for (; version < VERSION; version++) {
		const step = UPGRADES[version];
		if (!step) throw new ThemeError(`Mila cannot read themes of version ${version}.`);
		doc = step(doc);
	}
	if (version > VERSION && importing)
		notes.push('This theme comes from a newer Mila; anything this one cannot use was left out.');
	if (!BASES.includes(doc.base as Character))
		throw new ThemeError('This theme is not based on a theme Mila has.');

	let id = doc.id;
	if (importing) id = uuid();
	else if (typeof id !== 'string' || !UUID.test(id)) throw new ThemeError('This theme has no id.');

	let name = typeof doc.name === 'string' ? cleanName(doc.name) : '';
	if (!name) name = importing ? 'Imported theme' : 'Untitled theme';
	name = clip(name);
	if (importing) name = uniqueName(name, options.taken ?? []);

	let unknown = 0;
	let unusable = 0;
	if (importing) unknown += Object.keys(doc).filter((k) => !FIELDS.includes(k)).length;

	let target: Target = 'AA';
	if (own(doc, 'target')) {
		if (TARGETS.includes(doc.target as Target)) target = doc.target as Target;
		else unusable++;
	}

	let accent: ThemeAccent = {};
	if (own(doc, 'accent')) {
		const a = doc.accent;
		if (isObj(a) && Object.keys(a).length === 0) accent = {};
		else if (isObj(a) && Object.keys(a).length === 1 && num(a.hue, 0, 360)) accent = { hue: a.hue };
		else if (isObj(a) && Object.keys(a).length === 1 && a.manual === true)
			accent = { manual: true };
		else unusable++;
	}

	const maps = { tokens: {}, light: {}, dark: {} } as Record<Map3, Record<string, ThemeValue>>;
	for (const map of MAPS) {
		if (!own(doc, map)) continue;
		const m = doc[map];
		if (!isObj(m)) {
			unusable++;
			continue;
		}
		if (importing) unknown += Object.keys(m).filter((k) => FORBIDDEN.has(k)).length;
		for (const key of keys(m)) {
			const name = Object.hasOwn(RENAMED, key) ? RENAMED[key] : key;
			// A new name given too wins over the old one.
			if (name !== key && own(m, name)) continue;
			if (!importing) {
				// A copy through JSON, which also takes the app's reactive
				// copies (structuredClone refuses those).
				maps[map][name] = JSON.parse(JSON.stringify(m[key])) as ThemeValue;
				continue;
			}
			const meta = byName(name);
			if (!meta) {
				unknown++;
				continue;
			}
			const value = belongs(meta, map) ? parseValue(meta, m[key]) : null;
			if (value) maps[map][meta.name] = value;
			else unusable++;
		}
	}
	if (unknown)
		notes.push(
			`${unknown} ${plural(unknown, 'setting', 'settings')} from a newer Mila ${plural(unknown, 'was', 'were')} left out.`
		);
	if (unusable)
		notes.push(
			`${unusable} ${plural(unusable, 'setting', 'settings')} Mila cannot use ${plural(unusable, 'was', 'were')} left out.`
		);

	const theme: ThemeDoc = {
		mila: 'theme',
		version: importing ? VERSION : version,
		id: id as string,
		name,
		base: doc.base as Character,
		target,
		accent,
		...maps
	};
	if (importing && bytes(JSON.stringify(theme)) > LIMITS.bytes)
		throw new ThemeError('This theme is larger than Mila allows (16 KiB).');
	return { theme, notes };
}

/** The value a theme gives a token in a scheme, if it is one Mila can use. */
export function valueOf(
	theme: ThemeDoc,
	name: string,
	scheme: 'light' | 'dark'
): ThemeValue | null {
	const meta = byName(name);
	if (!meta) return null;
	const map: Map3 = meta.perScheme ? scheme : 'tokens';
	const raw = Object.hasOwn(theme[map], meta.name) ? theme[map][meta.name] : undefined;
	return raw === undefined ? null : parseValue(meta, raw);
}

/** A new theme on a built-in, changing nothing yet. High contrast's copies aim for AAA. */
export function blank(base: Character, name = 'New theme'): ThemeDoc {
	return {
		mila: 'theme',
		version: VERSION,
		id: uuid(),
		name,
		base,
		target: base === 'contrast' ? 'AAA' : 'AA',
		accent: {},
		tokens: {},
		light: {},
		dark: {}
	};
}

/** A theme as the text of a file to share. */
export function exportTheme(theme: ThemeDoc): string {
	return JSON.stringify(theme, null, '\t') + '\n';
}

/** A file name for a theme: its name without what file systems refuse. */
export function exportFileName(theme: ThemeDoc): string {
	// eslint-disable-next-line no-control-regex
	const safe = theme.name.replace(/[/\\:*?"<>|\u0000-\u001f\u007f-\u009f]/g, '').trim();
	return `${safe || 'theme'}.mila-theme.json`;
}

/** Whether a name is one the server takes. */
export function validName(name: string): boolean {
	const n = cleanName(name);
	return n.length > 0 && length(n) <= LIMITS.name;
}
