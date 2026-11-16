import { describe, expect, it } from 'vitest';
import vectors from '../../../../shared/theme-vectors.json';
import {
	blank,
	exportFileName,
	exportTheme,
	parseTheme,
	ThemeError,
	uniqueName,
	validName
} from './document';

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

describe('the shared theme vectors', () => {
	for (const v of vectors.valid) {
		it(`reads ${v.name} as the server does`, () => {
			expect(parseTheme(v.raw, 'stored')).toEqual({ theme: v.normalized, notes: [] });
			const imported = parseTheme(v.raw, 'import');
			expect(imported.notes).toEqual([]);
			expect(imported.theme.id).toMatch(UUID);
			expect(imported.theme.id).not.toBe(v.normalized.id);
			expect({ ...imported.theme, id: v.normalized.id }).toEqual(v.normalized);
		});
	}

	for (const v of vectors.invalid) {
		it(`imports ${v.name}: ${v.import}`, () => {
			if (v.import === 'reject') {
				expect(() => parseTheme(v.raw, 'import')).toThrow(ThemeError);
				return;
			}
			const { theme, notes } = parseTheme(v.raw, 'import');
			if (v.import === 'clean') expect(notes).toEqual([]);
			else expect(notes.length).toBeGreaterThan(0);
			expect(theme.id).toMatch(UUID);
			expect(validName(theme.name)).toBe(true);
			// What import keeps is a theme the server takes.
			expect(JSON.stringify(theme).length).toBeLessThanOrEqual(16 * 1024);
		});
	}
});

describe('parseTheme', () => {
	const stored = (extra: object) =>
		JSON.stringify({
			mila: 'theme',
			version: 1,
			id: '5d0c6b8e-2f4a-4c1e-9b7d-3a2f1e0c9d8b',
			name: 'Night shift',
			base: 'calm',
			...extra
		});

	it('keeps tokens it does not know in a stored theme, for the newer server', () => {
		const { theme } = parseTheme(
			stored({ light: { 'sparkle-bg': { oklch: [0.5, 0.1, 20] }, 'surface-0': { px: 3 } } }),
			'stored'
		);
		expect(theme.light).toEqual({
			'sparkle-bg': { oklch: [0.5, 0.1, 20] },
			'surface-0': { px: 3 }
		});
	});

	it('never reads __proto__, even when stored', () => {
		const raw =
			'{"mila":"theme","version":1,"id":"5d0c6b8e-2f4a-4c1e-9b7d-3a2f1e0c9d8b","name":"x","base":"calm","light":{"__proto__":{"polluted":true}},"__proto__":{"polluted":true}}';
		const { theme } = parseTheme(raw, 'stored');
		expect(Object.keys(theme.light)).toEqual([]);
		expect(({} as Record<string, unknown>).polluted).toBeUndefined();
		expect(Object.getPrototypeOf(theme.light)).toBe(Object.prototype);
	});

	it('reads a theme held behind a proxy, as the app holds its state', () => {
		const doc = JSON.parse(stored({ light: { 'surface-0': { oklch: [0.5, 0.1, 20] } } }));
		const proxied = (v: unknown): unknown =>
			v && typeof v === 'object'
				? new Proxy(v as object, { get: (t, k) => proxied(Reflect.get(t, k)) })
				: v;
		expect(parseTheme(proxied(doc), 'stored').theme.light).toEqual({
			'surface-0': { oklch: [0.5, 0.1, 20] }
		});
	});

	it('refuses a stored theme with no id', () => {
		expect(() => parseTheme(stored({ id: 'x' }), 'stored')).toThrow(ThemeError);
	});

	it('names an import apart from the themes there already', () => {
		const { theme } = parseTheme(stored({}), 'import', {
			taken: ['Night shift', 'night shift (2)']
		});
		expect(theme.name).toBe('Night shift (3)');
	});

	it('refuses a file over 64 KiB', () => {
		expect(() => parseTheme(' '.repeat(64 * 1024) + stored({}), 'import')).toThrow(ThemeError);
	});

	it('copies values rather than keeping what it was given', () => {
		const color = { oklch: [0.5, 0.1, 20] };
		const { theme } = parseTheme(
			{ ...JSON.parse(stored({})), light: { 'surface-0': color } },
			'import'
		);
		color.oklch[0] = 2;
		expect(theme.light['surface-0']).toEqual({ oklch: [0.5, 0.1, 20] });
	});
});

describe('names and files', () => {
	it('keeps a unique name within 60 characters', () => {
		const long = 'x'.repeat(60);
		const name = uniqueName(long, [long]);
		expect(name.endsWith(' (2)')).toBe(true);
		expect([...name].length).toBe(60);
	});

	it('writes a file name without what file systems refuse', () => {
		const theme = { ...blank('calm'), name: 'a/b\\c:d*e?"f<g>h|i\u0001' };
		expect(exportFileName(theme)).toBe('abcdefghi.mila-theme.json');
		expect(exportFileName({ ...theme, name: '???' })).toBe('theme.mila-theme.json');
	});

	it('exports what it imports', () => {
		const theme = { ...blank('playful'), name: 'Round trip' };
		theme.tokens['radius-md'] = { px: 9 };
		const back = parseTheme(exportTheme(theme), 'import').theme;
		expect({ ...back, id: theme.id }).toEqual(theme);
	});

	it('starts High contrast copies at AAA', () => {
		expect(blank('contrast').target).toBe('AAA');
		expect(blank('calm').target).toBe('AA');
	});
});
