// The serializer is the only way a theme reaches CSS: whatever a theme
// document holds, what comes out is known selectors and custom
// properties, with values made of numbers, token names and fixed strings.
import { describe, expect, it } from 'vitest';
import { appCss, fmt, values } from './css';
import { blank, EASES, FONTS, parseTheme, ThemeError, type ThemeDoc } from './document';
import { byName, TOKENS } from './tokens';

// A small seeded generator, so a failure can be run again.
function rng(seed: number) {
	return () => {
		seed = (seed * 1664525 + 1013904223) >>> 0;
		return seed / 2 ** 32;
	};
}

const HOSTILE = [
	'}',
	';',
	'</style><script>alert(1)</script>',
	'url(https://e.example/x)',
	'@import "x";',
	'\\',
	'red; } html { background: url(//e.example) }',
	'expression(alert(1))',
	'__proto__',
	'constructor',
	'"',
	"'",
	'\u0000',
	'1e999',
	'NaN',
	'-0'
];

function generator(seed: number) {
	const r = rng(seed);
	const pick = <T>(list: readonly T[]): T => list[Math.floor(r() * list.length)];
	const names = [...TOKENS.map((t) => t.name), ...HOSTILE, 'sparkle-bg'];
	const number = () =>
		pick([0, 1, -1, 0.5, 360, 999, -999, 1e-9, 1e21, Infinity, -Infinity, NaN, r() * 400 - 50]);
	function value(depth = 0): unknown {
		switch (Math.floor(r() * 12)) {
			case 0:
				return { oklch: [r(), r() * 0.4, r() * 360], ...(r() < 0.3 ? { alpha: r() } : {}) };
			case 1:
				return { oklch: [number(), number(), number()] };
			case 2:
				return { ref: pick(names) };
			case 3:
				return { [pick(['px', 'ms', 'n', 'hue'])]: r() < 0.7 ? number() : pick(HOSTILE) };
			case 4:
				return { font: pick([...Object.keys(FONTS), ...HOSTILE]) };
			case 5:
				return { ease: pick([...Object.keys(EASES), ...HOSTILE]) };
			case 6:
				return { bezier: [number(), number(), number(), number()] };
			case 7:
				return {
					layers: Array.from({ length: Math.floor(r() * 5) }, () => ({
						y: number(),
						blur: number(),
						spread: number(),
						...(r() < 0.2 ? { color: pick(HOSTILE) } : {})
					}))
				};
			case 8:
				return pick(HOSTILE);
			case 9:
				return depth < 2 ? { [pick(HOSTILE)]: value(depth + 1) } : null;
			case 10:
				return [value(depth + 1)];
			default:
				return { px: r() * 20 };
		}
	}
	const map = () =>
		Object.fromEntries(Array.from({ length: Math.floor(r() * 12) }, () => [pick(names), value()]));
	return () => ({
		mila: 'theme',
		version: r() < 0.9 ? 1 : pick([2, '1', 0]),
		id: '5d0c6b8e-2f4a-4c1e-9b7d-3a2f1e0c9d8b',
		name: pick(['Fuzz', ...HOSTILE]),
		base: r() < 0.95 ? pick(['standard', 'calm', 'playful', 'contrast']) : pick(HOSTILE),
		target: pick(['AA', 'AAA', ...HOSTILE]),
		accent: pick([{}, { hue: number() }, { manual: true }, { hue: pick(HOSTILE) }]),
		tokens: r() < 0.95 ? map() : pick(HOSTILE),
		light: map(),
		dark: map(),
		...(r() < 0.1 ? { [pick(HOSTILE)]: value() } : {})
	});
}

const FIXED = new Set<string>([...Object.values(FONTS), ...Object.values(EASES)]);
const SELECTORS = new Set([
	'html[data-custom-theme] {',
	"html[data-custom-theme][data-scheme='light'] {",
	"html[data-custom-theme][data-scheme='dark'] {",
	'@media (prefers-reduced-motion: no-preference) {',
	'}'
]);

/** Whether a value is one the serializer may write. */
function safeValue(v: string): boolean {
	if (FIXED.has(v)) return true;
	if (!/^[0-9a-z.\- %(),/]+$/.test(v)) return false;
	for (const [, fn] of v.matchAll(/([a-z-]+)\(/g))
		if (!['oklch', 'var', 'cubic-bezier'].includes(fn)) return false;
	for (const [, name] of v.matchAll(/var\(--([a-z0-9-]+)\)/g))
		if (!byName(name) && name !== 'shadow-color') return false;
	return true;
}

function checkCss(css: string) {
	for (const line of css.split('\n')) {
		if (!line) continue;
		if (SELECTORS.has(line.trim())) continue;
		const m = /^\t--([a-z0-9-]+): (.+);$/.exec(line);
		expect(m, line).not.toBeNull();
		expect(byName(m![1]) || m![1] === 'accent-hue', line).toBeTruthy();
		expect(safeValue(m![2]), line).toBe(true);
		for (const bad of ['url(', '@import', '\\', '<', ';', '}', '{', 'expression'])
			expect(m![2].includes(bad), line).toBe(false);
	}
}

describe('appCss under fuzz', () => {
	it('writes only known selectors, tokens and safe values', () => {
		const next = generator(20261029);
		let drawn = 0;
		for (let i = 0; i < 3000; i++) {
			const raw = next();
			for (const mode of ['stored', 'import'] as const) {
				let theme: ThemeDoc;
				try {
					theme = parseTheme(JSON.stringify(raw), mode).theme;
				} catch (err) {
					expect(err).toBeInstanceOf(ThemeError);
					continue;
				}
				const css = appCss(theme);
				checkCss(css);
				for (const scheme of ['light', 'dark'] as const)
					for (const [name, v] of values(theme, scheme)) {
						expect(byName(name) || name === 'accent-hue').toBeTruthy();
						expect(safeValue(v), `${name}: ${v}`).toBe(true);
					}
				if (css) drawn++;
			}
		}
		// The fuzz must reach the serializer often enough to mean something.
		expect(drawn).toBeGreaterThan(500);
	});

	it('leaves out what a stored theme keeps but cannot use', () => {
		const theme = blank('calm');
		theme.light = {
			'sparkle-bg': { oklch: [0.5, 0.1, 20] },
			'surface-0': { px: 3 } as never,
			'radius-md': { px: 3 }
		};
		expect(values(theme, 'light').size).toBe(0);
	});
});

describe('appCss', () => {
	it('splits values by scheme, and keeps motion for those who allow it', () => {
		const theme = blank('calm');
		theme.accent = { hue: 200 };
		theme.tokens = { 'radius-md': { px: 8 }, 'dur-panel': { ms: 240 }, ease: { ease: 'snappy' } };
		theme.light = {
			'surface-0': { oklch: [0.99, 0.01, 85] },
			scrim: { oklch: [0, 0, 0], alpha: 0.4 }
		};
		theme.dark = { 'card-bg': { ref: 'surface-2' } };
		expect(appCss(theme)).toBe(
			[
				'html[data-custom-theme] {',
				'\t--accent-hue: 200;',
				'\t--radius-md: 8px;',
				'}',
				'@media (prefers-reduced-motion: no-preference) {',
				'html[data-custom-theme] {',
				'\t--dur-panel: 240ms;',
				`\t--ease: ${EASES.snappy};`,
				'}',
				'}',
				"html[data-custom-theme][data-scheme='light'] {",
				'\t--surface-0: oklch(0.99 0.01 85);',
				'\t--scrim: oklch(0 0 0 / 0.4);',
				'}',
				"html[data-custom-theme][data-scheme='dark'] {",
				'\t--card-bg: var(--surface-2);',
				'}',
				''
			].join('\n')
		);
	});

	it('draws shadows in the shadow color, and none for no layers', () => {
		const theme = blank('standard');
		theme.tokens = {
			'shadow-sm': { layers: [{ y: 1, blur: 3, spread: -1 }] },
			'shadow-lg': { layers: [] }
		};
		const v = values(theme, 'light');
		expect(v.get('shadow-sm')).toBe('0 1px 3px -1px var(--shadow-color)');
		expect(v.get('shadow-lg')).toBe('none');
	});

	it('writes numbers plainly, never in exponent form', () => {
		expect(fmt(0.123456)).toBe('0.1235');
		expect(fmt(8)).toBe('8');
		expect(fmt(-0.00001)).toBe('0');
		expect(fmt(1e-9)).toBe('0');
		expect(() => fmt(Infinity)).toThrow();
	});
});
