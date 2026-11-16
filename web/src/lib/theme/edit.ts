// Changes to a theme document, for the theme editor: each returns a new
// document and leaves the one given alone, so the editor's history keeps
// every step as it was. A token is changed in the map it belongs in: the
// scheme being edited for one set apart per scheme, or the map both
// schemes share.
import type { Scheme } from './color';
import {
	EASES,
	FONTS,
	type EaseId,
	type FontId,
	type Map3,
	type ShadowLayer,
	type ThemeAccent,
	type ThemeDoc,
	type ThemeValue
} from './document';
import { byName, GROUPS, TOKENS, type TokenMeta } from './tokens';

/** The map a token's value lives in when editing a scheme. */
export function mapOf(meta: TokenMeta, scheme: Scheme): Map3 {
	return meta.perScheme ? scheme : 'tokens';
}

/** Whether the theme sets a token itself, in the scheme being edited. */
export function overridden(doc: ThemeDoc, name: string, scheme: Scheme): boolean {
	const meta = byName(name);
	return !!meta && Object.hasOwn(doc[mapOf(meta, scheme)], meta.name);
}

/** The theme with a token set to a value, or back to its base's (null). */
export function withValue(
	doc: ThemeDoc,
	name: string,
	scheme: Scheme,
	value: ThemeValue | null
): ThemeDoc {
	const meta = byName(name);
	if (!meta) return doc;
	const map = mapOf(meta, scheme);
	const next = { ...doc[map] };
	if (value === null) delete next[meta.name];
	else next[meta.name] = value;
	return { ...doc, [map]: next };
}

/** The theme with several tokens set (null for back to the base) at once: one step. */
export function withValues(
	doc: ThemeDoc,
	scheme: Scheme,
	changes: Iterable<[string, ThemeValue | null]>
): ThemeDoc {
	let out = doc;
	for (const [name, value] of changes) out = withValue(out, name, scheme, value);
	return out;
}

/** The theme with a group back to its base: the group's tokens in light, in dark and in both. */
export function withoutGroup(doc: ThemeDoc, group: string): ThemeDoc {
	const changes = groupTokens(group).map((t): [string, null] => [t.name, null]);
	return withValues(withValues(doc, 'light', changes), 'dark', changes);
}

/** The theme back to its base: every token, and the accent. Name, base and goal stay. */
export function resetAll(doc: ThemeDoc): ThemeDoc {
	return { ...doc, accent: {}, tokens: {}, light: {}, dark: {} };
}

/** The accent's tokens, which a hand-set accent sets per scheme. */
export const ACCENT_TOKENS = ['accent', 'accent-hover', 'accent-text'] as const;

/** Whether a theme's accent is set by hand rather than worked out from a hue. */
export const manual = (doc: ThemeDoc) => 'manual' in doc.accent;

/**
 * The theme with its accent generated from a hue (null for the base's),
 * or set by hand. Setting it by hand starts from the colors given (what
 * the generated accent was in each scheme), so nothing jumps; going back
 * to generated drops the hand-set colors.
 */
export function withAccent(
	doc: ThemeDoc,
	accent: ThemeAccent,
	start?: Record<Scheme, Partial<Record<(typeof ACCENT_TOKENS)[number], ThemeValue>>>
): ThemeDoc {
	let out: ThemeDoc = { ...doc, accent };
	for (const scheme of ['light', 'dark'] as const) {
		out = withValues(
			out,
			scheme,
			ACCENT_TOKENS.map((name) => [
				name,
				'manual' in accent ? (start?.[scheme][name] ?? null) : null
			])
		);
	}
	return out;
}

/** How many tokens of a group the theme sets, in the scheme being edited or both. */
export function changedIn(doc: ThemeDoc, group: string, scheme: Scheme): number {
	return groupTokens(group).filter((t) => overridden(doc, t.name, scheme)).length;
}

/** Whether a theme changes anything at all. */
export function changedAny(doc: ThemeDoc): boolean {
	return (
		Object.keys(doc.accent).length > 0 ||
		Object.keys(doc.tokens).length > 0 ||
		Object.keys(doc.light).length > 0 ||
		Object.keys(doc.dark).length > 0
	);
}

/**
 * The tokens that shape a whole group at once, shown first: the neutral
 * hue and tint behind surfaces, text and borders; the label colors'
 * lightness and chroma; the text size and row height.
 */
const KNOBS: Record<string, readonly string[]> = {
	surfaces: ['neutral-hue', 'neutral-chroma'],
	palette: ['palette-l', 'palette-c', 'palette-soft-l', 'palette-soft-c'],
	type: ['type-base', 'type-base-dense', 'font-ui', 'font-heading'],
	spacing: ['row-base', 'dense-factor']
};

/** A group's tokens: its knobs first, then the rest in the registry's order. */
export function groupTokens(group: string): TokenMeta[] {
	const knobs = KNOBS[group] ?? [];
	const all = TOKENS.filter((t) => t.group === group);
	return [
		...knobs.map((n) => all.find((t) => t.name === n)).filter((t): t is TokenMeta => !!t),
		...all.filter((t) => !knobs.includes(t.name))
	];
}

/** Whether a token is one of its group's knobs. */
export const isKnob = (meta: TokenMeta) => (KNOBS[meta.group] ?? []).includes(meta.name);

/** The groups, in the registry's order. */
export { GROUPS };

/**
 * What a token can be "the same as": a token of the same kind, set the
 * same way for both schemes, and of a lower layer (which keeps refs from
 * ever going round in a circle). A see-through one only for a token that
 * may be see-through too.
 */
export function candidates(meta: TokenMeta): TokenMeta[] {
	return TOKENS.filter(
		(t) =>
			t.type === meta.type &&
			t.perScheme === meta.perScheme &&
			t.rank < meta.rank &&
			(!t.alpha || !!meta.alpha)
	);
}

/**
 * Tokens scaled together from their base values: a whole group's spacing,
 * text sizes or durations by one factor, each rounded to its step and held
 * to its bounds. At 1 they go back to the base.
 */
export function scaled(
	doc: ThemeDoc,
	scheme: Scheme,
	base: Map<string, number>,
	factor: number
): ThemeDoc {
	const changes: [string, ThemeValue | null][] = [];
	for (const [name, value] of base) {
		const meta = byName(name);
		if (!meta || (meta.type !== 'px' && meta.type !== 'ms')) continue;
		if (Math.abs(factor - 1) < 1e-6) {
			changes.push([name, null]);
			continue;
		}
		const step = meta.step ?? 1;
		const n = Math.min(meta.max!, Math.max(meta.min!, Math.round((value * factor) / step) * step));
		const v = Number(n.toFixed(4));
		changes.push([name, meta.type === 'px' ? { px: v } : { ms: v }]);
	}
	return withValues(doc, scheme, changes);
}

/** The scales the editor offers per group, as names of the tokens they move. */
export const SCALES: Record<string, { label: string; names: readonly string[] }> = {
	spacing: {
		label: 'All spacing',
		names: ['space-1', 'space-2', 'space-3', 'space-4', 'space-5', 'space-6', 'space-7']
	},
	type: {
		label: 'All text sizes',
		names: [
			'type-xs',
			'type-sm',
			'type-base',
			'type-base-dense',
			'type-lg',
			'type-xl',
			'type-2xl',
			'type-3xl'
		]
	}
};

/** The motion speeds: a factor on every duration. */
export const SPEEDS = [
	{ id: 'slower', label: 'Slower', factor: 1.5 },
	{ id: 'base', label: 'As the base', factor: 1 },
	{ id: 'faster', label: 'Faster', factor: 0.6 },
	{ id: 'still', label: 'Almost still', factor: 0.25 }
] as const;

export const DURATIONS = ['dur-micro', 'dur-panel', 'dur-view'] as const;

/** Names for the faces a theme may use. */
export const FONT_NAMES: Record<string, string> = {
	inter: 'Inter',
	nunito: 'Nunito',
	'source-serif': 'Source Serif 4',
	'jetbrains-mono': 'JetBrains Mono',
	'system-sans': "This device's sans serif",
	'system-serif': "This device's serif",
	'system-mono': "This device's monospace"
};

/** Names for the curves a theme may pick. */
export const EASE_NAMES: Record<string, string> = {
	standard: 'Standard',
	gentle: 'Gentle',
	spring: 'Spring',
	snappy: 'Snappy',
	linear: 'Even'
};

/** The face a font stack is, by the stacks a theme may use; null for another. */
export function fontOf(raw: string): FontId | null {
	const norm = (s: string) =>
		s
			.replace(/["']/g, '')
			.replace(/\s*,\s*/g, ',')
			.trim()
			.toLowerCase();
	const n = norm(raw);
	for (const [id, stack] of Object.entries(FONTS)) if (norm(stack) === n) return id as FontId;
	return null;
}

/** A curve as a theme writes it: a named one, or a cubic bezier. Null for another (Playful's spring is named). */
export function easeOf(raw: string): ThemeValue | null {
	const norm = (s: string) => s.replace(/\s+/g, '').toLowerCase();
	const n = norm(raw);
	for (const [id, css] of Object.entries(EASES)) if (norm(css) === n) return { ease: id as EaseId };
	const m = /^cubic-bezier\(([^)]*)\)$/.exec(n);
	if (!m) return null;
	const b = m[1].split(',').map(Number);
	return b.length === 4 && b.every(Number.isFinite)
		? { bezier: b as [number, number, number, number] }
		: null;
}

/** A shadow's layers as the browser has the token: offsets before each color. */
export function layersOf(raw: string): ShadowLayer[] {
	if (!raw.trim() || raw.trim() === 'none') return [];
	const layers: string[] = [];
	let depth = 0;
	let start = 0;
	for (let i = 0; i < raw.length; i++) {
		if (raw[i] === '(') depth++;
		else if (raw[i] === ')') depth--;
		else if (raw[i] === ',' && depth === 0) {
			layers.push(raw.slice(start, i));
			start = i + 1;
		}
	}
	layers.push(raw.slice(start));
	const out: ShadowLayer[] = [];
	for (const layer of layers.slice(0, 3)) {
		const lengths = [...layer.replace(/[a-z-]+\([^)]*\)/gi, ' ').matchAll(/(-?[\d.]+)(px)?/g)].map(
			(m) => Number(m[1])
		);
		const [, y = 0, blur = 0, spread = 0] = lengths;
		out.push({ y, blur, spread });
	}
	return out;
}
