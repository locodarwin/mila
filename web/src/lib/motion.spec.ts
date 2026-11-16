import { afterEach, describe, expect, it, vi } from 'vitest';
import { collapse, cubicBezier, duration, parseDuration, parseEasing, sheet } from './motion';

describe('parseDuration', () => {
	it('reads milliseconds and seconds', () => {
		expect(parseDuration('200ms')).toBe(200);
		expect(parseDuration(' 0.26s ')).toBe(260);
	});
	it('refuses anything else', () => {
		expect(parseDuration('')).toBeNull();
		expect(parseDuration('fast')).toBeNull();
		expect(parseDuration('var(--dur-fade)')).toBeNull();
		expect(parseDuration('-5ms')).toBeNull();
	});
});

describe('parseEasing', () => {
	it('draws the cubic-bezier CSS would', () => {
		const ease = parseEasing('cubic-bezier(0.2, 0.8, 0.2, 1)')!;
		expect(ease(0)).toBe(0);
		expect(ease(1)).toBe(1);
		// An ease-out: well past halfway at the halfway point, and never back.
		expect(ease(0.5)).toBeGreaterThan(0.8);
		for (let t = 0.05; t < 1; t += 0.05) expect(ease(t + 0.05)).toBeGreaterThanOrEqual(ease(t));
	});
	it('agrees with a known point of ease-out', () => {
		// cubic-bezier(0, 0, 0.58, 1) at x = 0.5 is about 0.684 (checked against Chromium).
		expect(parseEasing('ease-out')!(0.5)).toBeCloseTo(0.684, 2);
	});
	it('follows linear() stops, overshoot and all', () => {
		const spring = parseEasing(
			'linear(0, 0.18 6%, 0.6 18%, 0.98 32%, 1.08 42%, 1.06 52%, 1.01 66%, 0.995 80%, 1)'
		)!;
		expect(spring(0)).toBe(0);
		expect(spring(0.42)).toBeCloseTo(1.08);
		expect(spring(0.37)).toBeCloseTo(1.03);
		expect(spring(1)).toBe(1);
	});
	it('spaces stops without a position evenly', () => {
		const e = parseEasing('linear(0, 0.5, 1)')!;
		expect(e(0.25)).toBeCloseTo(0.25);
		expect(e(0.75)).toBeCloseTo(0.75);
	});
	it('refuses what it cannot draw', () => {
		expect(parseEasing('steps(4)')).toBeNull();
		expect(parseEasing('cubic-bezier(0, 1)')).toBeNull();
		expect(parseEasing('linear(x)')).toBeNull();
	});
	it('is the identity for linear', () => {
		expect(parseEasing('linear')!(0.3)).toBe(0.3);
		expect(cubicBezier(0, 0, 1, 1)(0.3)).toBeCloseTo(0.3);
	});
});

describe('duration', () => {
	it('falls back to the default tokens without a page', () => {
		expect(duration('micro')).toBe(120);
		expect(duration('panel')).toBe(200);
		expect(duration('view')).toBe(320);
		expect(duration('fade')).toBe(150);
	});
});

describe('sheet', () => {
	const node = {} as Element;
	afterEach(() => vi.unstubAllGlobals());
	// What fly() reads from the element it moves.
	const stub = () => vi.stubGlobal('getComputedStyle', () => ({ transform: 'none', opacity: '1' }));

	it('leaves at once beside the list, so the list takes its room back', () => {
		expect(sheet(node, { phone: false }, { direction: 'out' }).duration).toBe(0);
	});

	it('slides in beside the list, and up and down on a phone', () => {
		stub();
		expect(sheet(node, { phone: false }, { direction: 'in' }).duration).toBe(200);
		expect(sheet(node, { phone: true }, { direction: 'in' }).duration).toBe(320);
		expect(sheet(node, { phone: true }, { direction: 'out' }).duration).toBe(320);
	});
});

describe('collapse', () => {
	afterEach(() => vi.unstubAllGlobals());
	// What slide() reads from the row it folds.
	const stub = () =>
		vi.stubGlobal('getComputedStyle', () => ({
			opacity: '1',
			height: '40px',
			paddingTop: '0px',
			paddingBottom: '0px',
			marginTop: '0px',
			marginBottom: '0px',
			borderTopWidth: '0px',
			borderBottomWidth: '0px'
		}));
	const rows = async (n: number) => {
		const configs = Array.from({ length: n }, () => collapse({} as Element));
		await Promise.resolve();
		return configs.map((c) => c().duration);
	};

	it('folds a row or a few away', async () => {
		stub();
		expect(await rows(1)).toEqual([200]);
		expect(await rows(8)).toEqual(Array(8).fill(200));
	});

	it('takes a crowd away at once: a filter, a long list growing', async () => {
		stub();
		expect(await rows(9)).toEqual(Array(9).fill(0));
		// The next change counts afresh.
		expect(await rows(1)).toEqual([200]);
	});
});
