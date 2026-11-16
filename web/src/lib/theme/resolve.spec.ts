import { describe, expect, it } from 'vitest';
import { hexToOklch, linearSrgbToOklch, oklchToLinearSrgb, srgbToOklch, toHex } from './color';
import { linear, parseColor } from './resolve';

const close = (a: number[], b: number[], digits = 3) =>
	a.forEach((x, i) => expect(x, `channel ${i}`).toBeCloseTo(b[i], digits));

describe('parseColor', () => {
	// The same color as each browser may write it from getComputedStyle.
	const teal = [0.6, 0.1, 185];
	const rgb = oklchToLinearSrgb(0.6, 0.1, 185);
	const hex = toHex(rgb);
	const bytes = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));

	it('reads oklch()', () => {
		close(parseColor('oklch(0.6 0.1 185)')!.lch, teal, 6);
		close(parseColor('oklch(60% 25% 185deg)')!.lch, teal, 6);
		expect(parseColor('oklch(0.6 0.1 185 / 0.25)')!.alpha).toBe(0.25);
		expect(parseColor('oklch(0.5 0 none)')!.lch).toEqual([0.5, 0, 0]);
	});

	it('reads rgb() and rgba(), with commas or without', () => {
		for (const s of [
			`rgb(${bytes.join(', ')})`,
			`rgb(${bytes.join(' ')})`,
			`rgba(${bytes.join(', ')}, 1)`
		])
			close(parseColor(s)!.lch.slice(0, 2), teal.slice(0, 2), 2);
		expect(parseColor('rgba(0, 0, 0, 0.25)')!.alpha).toBe(0.25);
		expect(parseColor('rgb(0 0 0 / 30%)')!.alpha).toBeCloseTo(0.3);
	});

	it('reads color(srgb) and color(srgb-linear)', () => {
		const srgb = bytes.map((b) => (b / 255).toFixed(5)).join(' ');
		close(parseColor(`color(srgb ${srgb})`)!.lch.slice(0, 2), teal.slice(0, 2), 2);
		close(parseColor(`color(srgb-linear ${rgb.join(' ')})`)!.lch, teal, 4);
	});

	it('reads oklab()', () => {
		const a = 0.1 * Math.cos((185 * Math.PI) / 180);
		const b = 0.1 * Math.sin((185 * Math.PI) / 180);
		close(parseColor(`oklab(0.6 ${a} ${b})`)!.lch, teal, 6);
	});

	it('refuses what is not a color it knows', () => {
		for (const s of [
			'red',
			'hsl(0 50% 50%)',
			'oklch(0.5 0.1)',
			'rgb(1, 2, 3, 4, 5)',
			'color(display-p3 1 0 0)',
			''
		])
			expect(parseColor(s), s).toBeNull();
		expect(parseColor('transparent')!.alpha).toBe(0);
	});
});

describe('conversions', () => {
	it('round-trips OKLCH through linear sRGB', () => {
		for (const lch of [
			[0.6, 0.1, 185],
			[0.3, 0.05, 20],
			[0.95, 0.02, 300]
		] as [number, number, number][])
			close(linearSrgbToOklch(oklchToLinearSrgb(...lch)), lch, 3);
	});

	it('reads hex as the color it names', () => {
		close(hexToOklch('#ffffff')!, [1, 0, 0], 3);
		close(hexToOklch('#000')!, [0, 0, 0], 3);
		close(hexToOklch('#808080')!, srgbToOklch(128 / 255, 128 / 255, 128 / 255), 6);
		expect(hexToOklch('#ggg')).toBeNull();
	});

	it('lays a see-through color over its background', () => {
		const black = { lch: [0, 0, 0] as [number, number, number], alpha: 0.5 };
		const white = { lch: [1, 0, 0] as [number, number, number], alpha: 1 };
		// Half black over white is mid gray in encoded values: 127.5 of 255.
		expect(['#7f7f7f', '#808080']).toContain(toHex(linear(black, white)));
	});
});
