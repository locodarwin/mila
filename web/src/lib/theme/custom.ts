// Building a custom theme for the page: its stylesheet, the accent worked
// out against its own surfaces, its page colors for the browser's bars,
// and whether it clears the floors. Loaded only when a custom theme has
// no CSS cached for it yet (appearance.svelte.ts), or by the editor.
//
// Values come from the browser, never from a copy of tokens.css: a hidden
// probe carries the theme's base, a scheme, the page's density and font
// size, and the theme's own values inline, so tokens.css works out every
// token on it exactly as it would on <html>.
import {
	accentColors,
	oklchToLinearSrgb,
	pageColor,
	targetContrast,
	toHex,
	type AccentColors,
	type Lch,
	type Scheme
} from './color';
import { check, clearsFloors, pairTokens, type Failure } from './contrast';
import { appCss, applyInline, fmt, isMotion } from './css';
import { parseTheme, type ThemeDoc } from './document';
import { linear, resolveTokens, type Color, type Resolved } from './resolve';
import { byName } from './tokens';

export const SCHEMES: readonly Scheme[] = ['light', 'dark'];

/** What the page needs to show a custom theme, and what the pre-paint script keeps of it. */
export interface Built {
	id: string;
	css: string;
	/** surface-0 in each scheme, as hex, for the theme-color metas. */
	page: Record<Scheme, string>;
	accent: Record<Scheme, AccentColors>;
	/** Whether text and the focus ring reach 3:1 on the page, per scheme. */
	floors: Record<Scheme, boolean>;
}

export interface Options {
	/** The person's accent hue, which a generated accent follows; a hand-set one ignores it. */
	accentHue: number | null;
	density?: string;
	fontSize?: string;
}

/**
 * A hidden element on which tokens.css and the theme's values work out
 * every token for one scheme. Remove it when done.
 */
export function probe(theme: ThemeDoc, scheme: Scheme, options: Options): HTMLElement {
	const el = document.createElement('div');
	el.className = 'theme-probe';
	el.setAttribute('aria-hidden', 'true');
	el.dataset.theme = theme.base;
	el.dataset.scheme = scheme;
	el.dataset.density = options.density ?? document.documentElement.dataset.density ?? 'comfortable';
	el.dataset.fontSize = options.fontSize ?? document.documentElement.dataset.fontSize ?? 'm';
	el.style.cssText =
		'position:fixed;top:0;left:0;width:0;height:0;overflow:hidden;visibility:hidden;pointer-events:none;';
	document.body.append(el);
	applyInline(el, theme, scheme);
	return el;
}

const color = (v: Resolved | undefined): Color | null =>
	v && typeof v === 'object' && 'lch' in v ? v : null;

/**
 * Works out the accent for a theme in a scheme on an element that carries
 * it (a probe or a preview container), and writes it there inline as
 * apply() writes it on <html>. Returns the accent.
 */
export function applyAccent(
	el: HTMLElement,
	theme: ThemeDoc,
	scheme: Scheme,
	accentHue: number | null
): AccentColors {
	const manual = 'manual' in theme.accent;
	if (!manual && accentHue !== null) el.style.setProperty('--accent-hue', fmt(accentHue));
	const hue = Number(getComputedStyle(el).getPropertyValue('--accent-hue')) || 262;
	const names = ['surface-0', 'surface-1', 'surface-2', 'surface-3', 'accent-contrast'];
	const r = resolveTokens(el, names);
	const surfaces = names
		.slice(0, 4)
		.map((n) => color(r.get(n))?.lch)
		.filter((x): x is Lch => !!x);
	const onAccent = color(r.get('accent-contrast'));
	const accent =
		surfaces.length === 4
			? accentColors(
					hue,
					scheme,
					targetContrast(theme.target),
					surfaces,
					onAccent ? linear(onAccent) : undefined
				)
			: accentColors(hue, scheme, theme.base);
	el.style.setProperty('--accent-l', String(accent.l));
	el.style.setProperty('--accent-c', String(accent.c));
	el.style.setProperty('--accent-text-l', String(accent.textL));
	el.style.setProperty('--accent-text-c', String(accent.textC));
	return accent;
}

/**
 * Every token a theme gives in a scheme, as the browser draws them, with
 * its accent worked out: for contrast checks, Fix, and showing a token's
 * value from the base.
 */
export function resolveTheme(
	theme: ThemeDoc,
	scheme: Scheme,
	names: Iterable<string>,
	options: Options
): { values: Map<string, Resolved>; accent: AccentColors } {
	const all = [...names];
	const moving = all.filter((n) => {
		const meta = byName(n);
		return !!meta && isMotion(meta);
	});
	const el = probe(theme, scheme, options);
	try {
		const accent = applyAccent(el, theme, scheme, options.accentHue);
		const values = resolveTokens(
			el,
			all.filter((n) => !moving.includes(n))
		);
		if (moving.length) for (const [n, v] of resolveMotion(theme, moving, options)) values.set(n, v);
		return { values, accent };
	} finally {
		el.remove();
	}
}

/**
 * Motion tokens as the theme sets them, whatever the OS asks for: on a
 * probe without data-scheme, which the reduced-motion rule in tokens.css
 * (keyed on both attributes) does not reach, with the theme's motion
 * written inline even under reduced motion. Motion is the same in both
 * schemes. The editor reads and scales these; the page itself still
 * keeps still under reduced motion.
 */
function resolveMotion(theme: ThemeDoc, names: string[], options: Options): Map<string, Resolved> {
	const el = probe(theme, 'light', options);
	delete el.dataset.scheme;
	applyInline(el, theme, 'light', { motion: true });
	try {
		return resolveTokens(el, names);
	} finally {
		el.remove();
	}
}

/** Every contrast problem of a theme, per scheme. */
export function problems(theme: ThemeDoc, options: Options): Record<Scheme, Failure[]> {
	const names = pairTokens();
	const out = {} as Record<Scheme, Failure[]>;
	for (const scheme of SCHEMES)
		out[scheme] = check(resolveTheme(theme, scheme, names, options).values, theme.target);
	return out;
}

/** Builds a stored theme for the page. Throws ThemeError if it cannot be read. */
export function build(raw: ThemeDoc, options: Options): Built {
	const { theme } = parseTheme(raw, 'stored');
	const out: Built = {
		id: theme.id,
		css: appCss(theme),
		page: { light: '', dark: '' },
		accent: {} as Record<Scheme, AccentColors>,
		floors: { light: false, dark: false }
	};
	for (const scheme of SCHEMES) {
		const { values, accent } = resolveTheme(
			theme,
			scheme,
			['surface-0', 'text-primary', 'focus-ring-color'],
			options
		);
		out.accent[scheme] = accent;
		out.floors[scheme] = clearsFloors(values);
		const page = color(values.get('surface-0'));
		out.page[scheme] = page ? toHex(oklchToLinearSrgb(...page.lch)) : pageColor(theme.base, scheme);
	}
	return out;
}

/** Why a theme cannot be used (below the floors, or unreadable), or null when it can. */
export function floorsOf(raw: ThemeDoc, options?: Partial<Options>): string | null {
	let built: Built;
	try {
		built = build(raw, { accentHue: null, ...options });
	} catch {
		return 'This theme cannot be read.';
	}
	if (built.floors.light && built.floors.dark) return null;
	const which =
		!built.floors.light && !built.floors.dark
			? 'either scheme'
			: built.floors.light
				? 'dark'
				: 'light';
	return `Text or the focus ring would be unreadable in ${which}: each needs 3:1 on the page.`;
}
