// The accent math needs each theme's surfaces, which live in tokens.css.
// This keeps the copy in color.ts in step with them.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { accentColors, contrastRatio, oklchToLinearSrgb, SURFACES, type Character } from './color';

const css = readFileSync(new URL('../styles/tokens.css', import.meta.url), 'utf8').replace(
	/\/\*[\s\S]*?\*\//g,
	''
);

/** The declarations of the first rule whose selector is exactly `selector`. */
function block(selector: string): string {
	const want = selector.replace(/\s+/g, ' ');
	for (const [, sel, body] of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
		if (sel.trim().replace(/\s+/g, ' ') === want) return body;
	}
	throw new Error(`no rule for ${selector}`);
}

function value(rule: string, name: string): string | undefined {
	return rule.match(new RegExp(`--${name}:\\s*([^;]+);`))?.[1].trim();
}

describe('SURFACES', () => {
	const characters: Character[] = ['standard', 'calm', 'playful', 'contrast'];
	for (const character of characters) {
		for (const scheme of ['light', 'dark'] as const) {
			it(`matches tokens.css for ${character} ${scheme}`, () => {
				const base = block(
					scheme === 'light' ? ":root, [data-scheme='light']" : "[data-scheme='dark']"
				);
				const own =
					character === 'standard' || character === 'playful'
						? ''
						: block(`[data-scheme='${scheme}'][data-theme='${character}']`);
				const traits = character === 'standard' ? '' : block(`[data-theme='${character}']`);
				const neutralHue =
					value(traits, 'neutral-hue') ?? value(block(':root, [data-theme]'), 'neutral-hue');
				const neutralChroma =
					value(traits, 'neutral-chroma') ?? value(block(':root, [data-theme]'), 'neutral-chroma');
				for (let i = 0; i < 4; i++) {
					const raw = (value(own, `surface-${i}`) ?? value(base, `surface-${i}`))!
						.replace('var(--neutral-chroma)', neutralChroma!)
						.replace('var(--neutral-hue)', neutralHue!);
					const [l, c, h] = raw
						.match(/oklch\(([^)]+)\)/)![1]
						.split(/\s+/)
						.map(Number);
					const [wl, wc, wh] = SURFACES[character][scheme][i];
					expect([l, c, c === 0 ? 0 : h], `surface-${i}`).toEqual([wl, wc, wc === 0 ? 0 : wh]);
				}
			});
		}
	}
});

// WCAG 1.4.11: what shows where a control is, or what state it is in,
// reaches 3:1 against what it sits on. Controls sit on surface-0 (pages,
// the task panel) and surface-1 (the sidebar).
describe('control contrast', () => {
	const characters: Character[] = ['standard', 'calm', 'playful', 'contrast'];
	const surfaces = (character: Character, scheme: 'light' | 'dark') =>
		SURFACES[character][scheme].slice(0, 2).map(([l, c, h]) => oklchToLinearSrgb(l, c, h));

	for (const character of characters) {
		for (const scheme of ['light', 'dark'] as const) {
			it(`--border-control reaches 3:1 in ${character} ${scheme}`, () => {
				const base = block(
					scheme === 'light' ? ":root, [data-scheme='light']" : "[data-scheme='dark']"
				);
				const own =
					character === 'contrast' ? block(`[data-scheme='${scheme}'][data-theme='contrast']`) : '';
				const traits = character === 'standard' ? '' : block(`[data-theme='${character}']`);
				const neutralHue =
					value(traits, 'neutral-hue') ?? value(block(':root, [data-theme]'), 'neutral-hue');
				const neutralChroma =
					value(traits, 'neutral-chroma') ?? value(block(':root, [data-theme]'), 'neutral-chroma');
				const raw = (value(own, 'border-control') ?? value(base, 'border-control'))!
					.replace('var(--neutral-chroma)', neutralChroma!)
					.replace('var(--neutral-hue)', neutralHue!);
				const [l, c, h] = raw
					.match(/oklch\(([^)]+)\)/)![1]
					.split(/\s+/)
					.map(Number);
				const border = oklchToLinearSrgb(l, c, h);
				for (const [i, surface] of surfaces(character, scheme).entries())
					expect(contrastRatio(border, surface), `on surface-${i}`).toBeGreaterThanOrEqual(3);
			});

			it(`the accent (focus rings, checked boxes) reaches 3:1 in ${character} ${scheme}, any hue`, () => {
				for (let hue = 0; hue < 360; hue += 5) {
					const { l, c } = accentColors(hue, scheme, character);
					const accent = oklchToLinearSrgb(l, c, hue);
					for (const [i, surface] of surfaces(character, scheme).entries())
						expect(
							contrastRatio(accent, surface),
							`hue ${hue} on surface-${i}`
						).toBeGreaterThanOrEqual(3);
				}
			});
		}
	}

	// Reports' chart series are marks too (components/reports/).
	for (const character of characters) {
		for (const scheme of ['light', 'dark'] as const) {
			it(`the chart series reach 3:1 in ${character} ${scheme}`, () => {
				const base = block(
					scheme === 'light' ? ":root, [data-scheme='light']" : "[data-scheme='dark']"
				);
				for (const n of [1, 2, 3, 4, 5, 6]) {
					const raw = value(base, `chart-series-${n}`)!;
					const [l, c, h] = raw
						.match(/oklch\(([^)]+)\)/)![1]
						.split(/\s+/)
						.map(Number);
					const series = oklchToLinearSrgb(l, c, h);
					for (const [i, surface] of surfaces(character, scheme).entries())
						expect(
							contrastRatio(series, surface),
							`series ${n} on surface-${i}`
						).toBeGreaterThanOrEqual(3);
				}
			});
		}
	}
});
