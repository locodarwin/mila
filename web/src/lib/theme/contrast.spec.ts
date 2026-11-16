import { describe, expect, it } from 'vitest';
import { contrastRatio, oklchToLinearSrgb, type Lch } from './color';
import { check, clearsFloors, fix, needed, PAIRS, pairTokens, ratio, type Pair } from './contrast';
import type { Color, Resolved } from './resolve';
import { byName } from './tokens';

const c = (l: number, ch = 0, h = 0, alpha = 1): Color => ({ lch: [l, ch, h], alpha });
const map = (entries: Record<string, Color>) => new Map<string, Resolved>(Object.entries(entries));

describe('PAIRS', () => {
	it('names only registry tokens', () => {
		expect(pairTokens().filter((n) => !byName(n))).toEqual([]);
	});
});

describe('check', () => {
	const pairs: Pair[] = [
		{ fg: 'text-muted', bg: ['surface-0', 'surface-3'], min: 'text', label: 'Muted' }
	];

	it('finds a pair below its target, with its ratio', () => {
		const resolved = map({ 'text-muted': c(0.62), 'surface-0': c(1), 'surface-3': c(0.9) });
		const failures = check(resolved, 'AA', pairs);
		expect(failures.map((f) => f.bg)).toEqual(['surface-0', 'surface-3']);
		expect(failures[0].ratio).toBeLessThan(4.5);
		expect(failures[0].need).toBe(4.5);
	});

	it('asks more of AAA', () => {
		const resolved = map({ 'text-muted': c(0.5), 'surface-0': c(1), 'surface-3': c(1) });
		expect(check(resolved, 'AA', pairs)).toEqual([]);
		expect(check(resolved, 'AAA', pairs).length).toBe(2);
		expect(needed('ui', 'AAA')).toBe(3);
		expect(needed('large', 'AAA')).toBe(4.5);
	});

	it('skips a pair missing a color', () => {
		expect(check(map({ 'surface-0': c(1) }), 'AA', pairs)).toEqual([]);
	});

	it("works out FieldInput's mixed chip", () => {
		const resolved = map({
			'text-primary': c(0.2),
			'palette-red-soft': c(0.94, 0.045, 25),
			'palette-red': c(0.6, 0.15, 25),
			'surface-0': c(0.995)
		});
		const red = PAIRS.find((p) => p.label === 'Text on red chips')!;
		expect(check(resolved, 'AA', [red])).toEqual([]);
		// Dark text on a dark mix fails.
		resolved.set('palette-red', c(0.1, 0.1, 25));
		resolved.set('surface-0', c(0.15));
		expect(check(resolved, 'AA', [red]).length).toBe(1);
	});

	it('lays a see-through background over the page', () => {
		const half = ratio(c(0), c(0, 0, 0, 0.5), c(1));
		expect(half).toBeGreaterThan(1);
		expect(half).toBeLessThan(ratio(c(0), c(1)));
	});
});

describe('the floors', () => {
	it('hold at 3:1 for text and the focus ring on the page', () => {
		const good = map({
			'text-primary': c(0.2),
			'focus-ring-color': c(0.5, 0.15, 262),
			'surface-0': c(1)
		});
		expect(clearsFloors(good)).toBe(true);
		good.set('text-primary', c(0.85));
		expect(clearsFloors(good)).toBe(false);
	});
});

describe('fix', () => {
	const lin = (x: Lch) => oklchToLinearSrgb(...x);

	it('moves a failing text color until every pair it is in passes', () => {
		const resolved = map({
			'text-muted': c(0.7, 0.02, 260),
			'surface-0': c(0.99),
			'surface-1': c(0.97),
			'surface-2': c(0.95),
			'surface-3': c(0.92),
			'panel-bg': c(0.99),
			'card-bg': c(0.99),
			'menu-bg': c(0.99),
			'sidebar-bg': c(0.97),
			'dialog-bg': c(0.99)
		});
		const fixed = fix(resolved, 'text-muted', 'AA')!;
		expect(fixed[0]).toBeLessThan(0.7);
		resolved.set('text-muted', c(...fixed));
		expect(check(resolved, 'AA').filter((f) => f.pair.fg === 'text-muted')).toEqual([]);
	});

	it('moves a background away from what is read on it', () => {
		const resolved = map({ 'toast-text': c(0.99), 'toast-bg': c(0.8) });
		const fixed = fix(resolved, 'toast-bg', 'AA')!;
		expect(contrastRatio(lin(fixed), lin([0.99, 0, 0]))).toBeGreaterThanOrEqual(4.5);
	});

	it('reaches AAA when asked', () => {
		const resolved = map({ 'toast-text': c(0.99), 'toast-bg': c(0.5) });
		const fixed = fix(resolved, 'toast-bg', 'AAA')!;
		expect(contrastRatio(lin(fixed), lin([0.99, 0, 0]))).toBeGreaterThanOrEqual(7);
	});

	it('moves a token that is one side of a mixed background, mixing again for each try', () => {
		const pairs: Pair[] = [
			{
				fg: 'text-primary',
				bg: [{ mix: ['palette-red', 'surface-0', 0.18], label: 'red option' }],
				min: 'text',
				label: 'Text on red chips'
			}
		];
		const resolved = map({
			'text-primary': c(0.4),
			'surface-0': c(0.85),
			'palette-red': c(0.05, 0.05, 25)
		});
		expect(check(resolved, 'AA', pairs)).toHaveLength(1);
		const fixed = fix(resolved, 'palette-red', 'AA', pairs)!;
		expect(fixed[0]).toBeGreaterThan(0.05);
		resolved.set('palette-red', c(...fixed));
		expect(check(resolved, 'AA', pairs)).toEqual([]);
	});

	it('leaves a color that passes as it is', () => {
		const resolved = map({ 'toast-text': c(0.99), 'toast-bg': c(0.2) });
		expect(fix(resolved, 'toast-bg', 'AA')).toEqual([0.2, 0, 0]);
	});

	it('has nothing to fix for a token in no pair', () => {
		expect(fix(map({ 'radius-md': c(0.5) }), 'radius-md', 'AA')).toBeNull();
	});
});
