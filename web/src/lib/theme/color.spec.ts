import { describe, expect, it } from 'vitest';
import {
	AAA_CONTRAST,
	accentColors,
	accentFor,
	accentTextColor,
	contrastRatio,
	contrastTarget,
	inGamut,
	MIN_CONTRAST,
	oklchToLinearSrgb,
	SURFACES,
	type Character,
	type Scheme
} from './color';

describe('oklchToLinearSrgb', () => {
	it('maps white and black to the ends of the range', () => {
		const white = oklchToLinearSrgb(1, 0, 0);
		const black = oklchToLinearSrgb(0, 0, 0);
		white.forEach((v) => expect(v).toBeCloseTo(1, 3));
		black.forEach((v) => expect(v).toBeCloseTo(0, 6));
	});

	it('knows a vivid green is outside sRGB', () => {
		expect(inGamut(oklchToLinearSrgb(0.6, 0.4, 145))).toBe(false);
	});
});

describe('contrastRatio', () => {
	it('is 21 for black on white', () => {
		expect(contrastRatio([1, 1, 1], [0, 0, 0])).toBeCloseTo(21, 5);
	});
});

describe('accentFor', () => {
	for (const scheme of ['light', 'dark'] as Scheme[]) {
		it(`gives every hue an in-gamut AA accent in the ${scheme} scheme`, () => {
			for (let hue = 0; hue < 360; hue += 3) {
				const { l, c } = accentFor(hue, scheme);
				const color = oklchToLinearSrgb(l, c, hue);
				expect(inGamut(color), `hue ${hue}`).toBe(true);
				expect(
					contrastRatio(color, accentTextColor(scheme, hue)),
					`hue ${hue}`
				).toBeGreaterThanOrEqual(MIN_CONTRAST);
			}
		});
	}

	it('keeps a colorful accent where it can', () => {
		expect(accentFor(262, 'light').c).toBeGreaterThan(0.12);
	});
});

describe('accentColors', () => {
	const characters: Character[] = ['standard', 'calm', 'playful', 'contrast'];
	for (const character of characters) {
		for (const scheme of ['light', 'dark'] as Scheme[]) {
			it(`keeps accent text readable on every ${character} ${scheme} surface`, () => {
				const target = contrastTarget(character);
				const surfaces = SURFACES[character][scheme].map(([l, c, h]) => oklchToLinearSrgb(l, c, h));
				for (let hue = 0; hue < 360; hue += 5) {
					const a = accentColors(hue, scheme, character);
					const accent = oklchToLinearSrgb(a.l, a.c, hue);
					const text = oklchToLinearSrgb(a.textL, a.textC, hue);
					expect(inGamut(accent), `hue ${hue}`).toBe(true);
					expect(inGamut(text), `hue ${hue} text`).toBe(true);
					expect(
						contrastRatio(accent, accentTextColor(scheme, hue)),
						`hue ${hue}`
					).toBeGreaterThanOrEqual(target);
					for (const [i, surface] of surfaces.entries()) {
						expect(
							contrastRatio(text, surface),
							`hue ${hue} on surface-${i}`
						).toBeGreaterThanOrEqual(target);
					}
				}
			});
		}
	}

	it('aims for AAA in High contrast only', () => {
		expect(contrastTarget('contrast')).toBe(AAA_CONTRAST);
		expect(contrastTarget('calm')).toBe(MIN_CONTRAST);
		expect(accentFor(262, 'light', 'contrast').l).toBeLessThan(accentFor(262, 'light').l);
	});
});
