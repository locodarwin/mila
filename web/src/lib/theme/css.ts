// A custom theme as CSS: the only way a theme reaches a style. Every
// value is built here from numbers (finite, fixed to four places), token
// names checked against the registry, and fixed strings picked by name;
// nothing the theme document holds is ever written as it is.
//
// The rules hold only what the theme changes; everything else, derived
// tokens included, comes from tokens.css for its base, worked out again on
// <html> from these values.
import { reducedMotion } from '../motion';
import type { Scheme } from './color';
import { EASES, FONTS, MAPS, parseValue, type ThemeDoc, type ThemeValue } from './document';
import { byName, type TokenMeta } from './tokens';

/** A number as CSS writes it: four places at most, no trailing zeros. */
export function fmt(n: number): string {
	if (!Number.isFinite(n)) throw new Error('not a finite number');
	const s = n.toFixed(4).replace(/\.?0+$/, '');
	return s === '-0' ? '0' : s;
}

/** A color as CSS, for a swatch drawn outside any theme preview. */
export function colorCss([l, c, h]: [number, number, number], alpha = 1): string {
	const color = `${fmt(l)} ${fmt(c)} ${fmt(h)}`;
	return alpha >= 1 ? `oklch(${color})` : `oklch(${color} / ${fmt(alpha)})`;
}

/** One value as CSS. */
export function cssValue(meta: TokenMeta, v: ThemeValue): string {
	if ('ref' in v) return `var(--${byName(v.ref)!.name})`;
	if ('oklch' in v) {
		const [l, c, h] = v.oklch;
		const color = `${fmt(l)} ${fmt(c)} ${fmt(h)}`;
		return v.alpha === undefined ? `oklch(${color})` : `oklch(${color} / ${fmt(v.alpha)})`;
	}
	if ('px' in v) return `${fmt(v.px)}px`;
	if ('ms' in v) return `${fmt(v.ms)}ms`;
	if ('n' in v) return fmt(v.n);
	if ('hue' in v) return fmt(v.hue);
	if ('font' in v) return FONTS[v.font];
	if ('ease' in v) return EASES[v.ease];
	if ('bezier' in v) return `cubic-bezier(${v.bezier.map(fmt).join(', ')})`;
	if ('layers' in v) {
		if (!v.layers.length) return 'none';
		return v.layers
			.map((l) => `0 ${fmt(l.y)}px ${fmt(l.blur)}px ${fmt(l.spread)}px var(--shadow-color)`)
			.join(', ');
	}
	throw new Error(`no CSS for a ${meta.type} value`);
}

/** Motion tokens follow the OS's reduced motion setting, never the theme. */
export const isMotion = (meta: TokenMeta) => meta.group === 'motion';

/**
 * The custom properties a theme sets in a scheme, by name: the ones the
 * same in both schemes, then that scheme's. Only tokens Mila knows, in the
 * map they belong in, with values they can take; the rest is left out.
 * The accent hue, when the theme chose one, is among them.
 */
export function values(theme: ThemeDoc, scheme: Scheme): Map<string, string> {
	const out = new Map<string, string>();
	if ('hue' in theme.accent && typeof theme.accent.hue === 'number')
		out.set('accent-hue', fmt(theme.accent.hue));
	for (const map of MAPS) {
		if (map !== 'tokens' && map !== scheme) continue;
		const m = theme[map];
		for (const key of Object.keys(m)) {
			const meta = byName(key);
			if (!meta || meta.perScheme !== (map !== 'tokens')) continue;
			const v = parseValue(meta, m[key]);
			if (v) out.set(meta.name, cssValue(meta, v));
		}
	}
	return out;
}

function rule(selector: string, decls: [string, string][]): string {
	if (!decls.length) return '';
	return `${selector} {\n${decls.map(([n, v]) => `\t--${n}: ${v};\n`).join('')}}\n`;
}

const motion = (name: string) => {
	const meta = byName(name);
	return !!meta && isMotion(meta);
};

/**
 * The stylesheet for the app: the scheme-free values on
 * html[data-custom-theme] (0,1,1), which beats the characters (0,1,0) and
 * loses to reduced motion (0,2,0), and each scheme's on
 * html[data-custom-theme][data-scheme] (0,2,1), which beats a character
 * in a scheme (0,2,0). Motion is only for those who allow it.
 */
export function appCss(theme: ThemeDoc): string {
	const both = [...values(theme, 'light')].filter(([n]) => !isScheme(n));
	const still = both.filter(([n]) => !motion(n));
	const moving = both.filter(([n]) => motion(n));
	const own = (scheme: Scheme) => [...values(theme, scheme)].filter(([n]) => isScheme(n));
	let css = rule('html[data-custom-theme]', still);
	const m = rule('html[data-custom-theme]', moving);
	if (m) css += `@media (prefers-reduced-motion: no-preference) {\n${m}}\n`;
	for (const scheme of ['light', 'dark'] as const)
		css += rule(`html[data-custom-theme][data-scheme='${scheme}']`, own(scheme));
	return css;
}

function isScheme(name: string): boolean {
	return byName(name)?.perScheme ?? false;
}

/**
 * Writes a theme's values for one scheme on an element, as inline custom
 * properties: a preview container, which carries data-theme (the base),
 * data-scheme, data-density and data-font-size, so tokens.css works out the
 * rest on it. Clears what an earlier call set. Motion is left out while
 * the OS asks for less of it, unless `motion` asks for it anyway.
 */
export function applyInline(
	el: HTMLElement,
	theme: ThemeDoc,
	scheme: Scheme,
	{ motion: always = false }: { motion?: boolean } = {}
) {
	const next = values(theme, scheme);
	const set = (el.dataset.themeProps ?? '').split(' ').filter(Boolean);
	for (const name of set) if (!next.has(name)) el.style.removeProperty(`--${name}`);
	// motion: the theme's own motion even so (the editor reading it).
	const quiet = reducedMotion() && !always;
	const written: string[] = [];
	for (const [name, value] of next) {
		if (quiet && motion(name)) {
			el.style.removeProperty(`--${name}`);
			continue;
		}
		el.style.setProperty(`--${name}`, value);
		written.push(name);
	}
	el.dataset.themeProps = written.join(' ');
}
