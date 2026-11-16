import { describe, expect, it } from 'vitest';
import { blank, parseValue, type ThemeDoc } from './document';
import {
	candidates,
	changedAny,
	changedIn,
	easeOf,
	fontOf,
	groupTokens,
	layersOf,
	resetAll,
	scaled,
	withAccent,
	withoutGroup,
	withValue
} from './edit';
import { byName, GROUPS, TOKENS } from './tokens';

const doc = (): ThemeDoc => blank('calm', 'Test');

describe('changing a theme document', () => {
	it('puts a token in the map it belongs in, and leaves the old document alone', () => {
		const before = doc();
		const a = withValue(before, 'surface-0', 'dark', { oklch: [0.2, 0.01, 90] });
		expect(a.dark).toEqual({ 'surface-0': { oklch: [0.2, 0.01, 90] } });
		expect(a.light).toEqual({});
		const b = withValue(a, 'radius-md', 'dark', { px: 8 });
		expect(b.tokens).toEqual({ 'radius-md': { px: 8 } });
		expect(before.dark).toEqual({});
		expect(withValue(b, 'radius-md', 'light', null).tokens).toEqual({});
	});

	it('resets a whole group, in light, in dark and in both, and nothing else', () => {
		let d = withValue(doc(), 'sidebar-bg', 'light', { oklch: [0.9, 0.02, 90] });
		d = withValue(d, 'sidebar-bg', 'dark', { oklch: [0.2, 0.02, 90] });
		d = withValue(d, 'card-radius', 'light', { px: 4 });
		d = withValue(d, 'card-bg', 'dark', { ref: 'surface-1' });
		expect(changedIn(d, 'sidebar', 'light')).toBe(1);
		const r = withoutGroup(d, 'sidebar');
		expect(r.light).toEqual({});
		expect(r.dark).toEqual({ 'card-bg': { ref: 'surface-1' } });
		expect(r.tokens).toEqual({ 'card-radius': { px: 4 } });
		const cards = withoutGroup(d, 'cards');
		expect(cards.tokens).toEqual({});
		expect(cards.dark).toEqual({ 'sidebar-bg': { oklch: [0.2, 0.02, 90] } });
		expect(changedAny(resetAll(r))).toBe(false);
	});

	it('sets the accent by hand from the colors given, and drops them going back', () => {
		const start = {
			light: { accent: { oklch: [0.5, 0.1, 200] as [number, number, number] } },
			dark: { accent: { oklch: [0.7, 0.1, 200] as [number, number, number] } }
		};
		const hand = withAccent(doc(), { manual: true }, start);
		expect(hand.accent).toEqual({ manual: true });
		expect(hand.light.accent).toEqual({ oklch: [0.5, 0.1, 200] });
		const back = withAccent(hand, { hue: 30 });
		expect(back.light).toEqual({});
		expect(back.dark).toEqual({});
	});

	it('scales tokens from their base values, rounded to their steps, and 1 resets them', () => {
		const base = new Map([
			['space-1', 4],
			['space-4', 16]
		]);
		const big = scaled(doc(), 'light', base, 1.5);
		expect(big.tokens).toEqual({ 'space-1': { px: 6 }, 'space-4': { px: 24 } });
		expect(scaled(big, 'light', base, 1).tokens).toEqual({});
		const durations = scaled(doc(), 'light', new Map([['dur-panel', 200]]), 100);
		expect(durations.tokens['dur-panel']).toEqual({ ms: byName('dur-panel')!.max });
	});

	it('offers as "Same as" only tokens a ref may name', () => {
		for (const t of TOKENS) {
			for (const c of candidates(t))
				expect(parseValue(t, { ref: c.name }), `${t.name} → ${c.name}`).toEqual({ ref: c.name });
		}
		expect(candidates(byName('surface-0')!)).toEqual(
			TOKENS.filter((t) => t.type === 'color' && t.perScheme && t.rank === 0 && !t.alpha)
		);
	});

	it('lists every token once across the groups, knobs first', () => {
		const all = GROUPS.flatMap((g) => groupTokens(g.id).map((t) => t.name));
		expect(all.sort()).toEqual(TOKENS.map((t) => t.name).sort());
		expect(groupTokens('surfaces')[0].name).toBe('neutral-hue');
	});
});

describe('reading values the browser drew', () => {
	it('knows the fonts and curves a theme may use', () => {
		expect(fontOf('"Nunito Variable", system-ui, sans-serif')).toBe('nunito');
		expect(fontOf('Comic Sans')).toBeNull();
		expect(easeOf('cubic-bezier(0.25, 0.6, 0.3, 1)')).toEqual({ ease: 'gentle' });
		expect(easeOf('cubic-bezier(0.1, 0.2, 0.3, 0.4)')).toEqual({ bezier: [0.1, 0.2, 0.3, 0.4] });
		expect(easeOf('steps(4)')).toBeNull();
	});

	it("reads a shadow's layers, whatever color they are in", () => {
		expect(layersOf('0 2px 6px oklch(0.3 0.02 75 / 0.12), 0 1px 2px rgba(0, 0, 0, 0.45)')).toEqual([
			{ y: 2, blur: 6, spread: 0 },
			{ y: 1, blur: 2, spread: 0 }
		]);
		expect(layersOf('none')).toEqual([]);
	});
});
