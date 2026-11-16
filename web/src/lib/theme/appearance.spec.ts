import { readFileSync } from 'node:fs';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { preferences } from '../preferences.svelte';
import {
	appearance,
	CACHE_VERSION,
	DEFAULTS,
	parse,
	readCustom,
	themeKey
} from './appearance.svelte';
import { build } from './custom';
import { blank } from './document';

// Building needs a real page (custom.ts resolves tokens in the browser);
// here it is stood in for, to check what apply() does with what it builds.
vi.mock('./custom', () => ({ build: vi.fn() }));

describe('parse', () => {
	it('returns the defaults for nothing stored', () => {
		expect(parse(null)).toEqual(DEFAULTS);
	});

	it('returns the defaults for malformed JSON', () => {
		expect(parse('{not json')).toEqual(DEFAULTS);
	});

	it('keeps valid values and replaces invalid ones', () => {
		expect(
			parse(
				JSON.stringify({ character: 'calm', scheme: 'sepia', accentHue: 400, density: 'dense' })
			)
		).toEqual({ ...DEFAULTS, character: 'calm', density: 'dense' });
	});

	it('keeps every font size, XL and XXL included', () => {
		for (const fontSize of ['s', 'm', 'l', 'xl', 'xxl'])
			expect(parse(JSON.stringify({ fontSize })).fontSize).toBe(fontSize);
		expect(parse(JSON.stringify({ fontSize: 'xxxl' })).fontSize).toBe('m');
	});

	it('keeps a custom accent hue', () => {
		expect(parse(JSON.stringify({ accentHue: 120 })).accentHue).toBe(120);
	});
});

describe('appearance.preview', () => {
	it('shows a look without touching the preferences', () => {
		const theme = preferences.current.theme;
		appearance.preview({ character: 'playful' });
		appearance.preview({ density: 'dense' });
		expect(appearance.current.character).toBe('playful');
		expect(appearance.current.density).toBe('dense');
		expect(preferences.current.theme).toBe(theme);
		appearance.preview(null);
		expect(appearance.current.character).toBe(theme);
		expect(appearance.current.density).toBe(preferences.current.density);
	});
});

describe('a custom theme in the preferences', () => {
	const id = '5d0c6b8e-2f4a-4c1e-9b7d-3a2f1e0c9d8b';

	it('parses a custom theme id, and nothing else', () => {
		expect(parse(JSON.stringify({ customTheme: id })).customTheme).toBe(id);
		expect(parse(JSON.stringify({ customTheme: 'night' })).customTheme).toBeNull();
	});

	it('leaves the custom theme when a character is chosen', () => {
		preferences.set({ customTheme: id, theme: 'calm' });
		expect(appearance.current.customTheme).toBe(id);
		appearance.set({ character: 'standard' });
		expect(preferences.current.customTheme).toBeNull();
		expect(preferences.current.theme).toBe('standard');
		appearance.set({ density: 'dense' });
		appearance.set({ density: 'comfortable' });
		preferences.disconnect();
	});
});

describe('the built theme kept for the pre-paint script', () => {
	const kept = (v: number) =>
		JSON.stringify({ character: 'calm', custom: { v, id: 'x', key: 'k', css: 'html{}' } });

	it("is read by app.html's pre-paint at the same cache version", () => {
		const html = readFileSync(new URL('../../app.html', import.meta.url), 'utf8');
		const versions = [...html.matchAll(/c\.v === (\d+)/g)].map((m) => Number(m[1]));
		expect(versions).toEqual([CACHE_VERSION]);
		expect(html).toContain(`CACHE_VERSION ${CACHE_VERSION}`);
	});

	it('is used only at this cache version', () => {
		expect(readCustom(kept(CACHE_VERSION))?.css).toBe('html{}');
		expect(readCustom(kept(CACHE_VERSION + 1))).toBeNull();
		expect(readCustom('{oops')).toBeNull();
		expect(readCustom(null)).toBeNull();
	});

	it('is built again when the theme or the accent hue changes', () => {
		const doc = blank('calm');
		const key = themeKey(doc, null);
		expect(themeKey(structuredClone(doc), null)).toBe(key);
		expect(themeKey({ ...doc, name: 'Other' }, null)).not.toBe(key);
		expect(themeKey(doc, 120)).not.toBe(key);
	});
});

describe('apply() with a custom theme', () => {
	// Just enough of a page for apply().
	function fakePage() {
		const elements = new Map<string, { id: string; textContent: string; remove(): void }>();
		const root = {
			dataset: {} as Record<string, string>,
			style: { setProperty() {}, removeProperty() {} }
		};
		vi.stubGlobal('document', {
			documentElement: root,
			head: { append: (el: { id: string }) => elements.set(el.id, el as never) },
			getElementById: (id: string) => elements.get(id) ?? null,
			createElement: () => ({
				id: '',
				textContent: '',
				remove() {
					elements.delete(this.id);
				}
			}),
			querySelectorAll: () => []
		});
		vi.stubGlobal('getComputedStyle', () => ({ getPropertyValue: () => '' }));
		return { root, style: () => elements.get('mila-theme')?.textContent };
	}

	const built = (id: string, css: string, ok = true) => ({
		id,
		css,
		page: { light: '#ffffff', dark: '#000000' },
		accent: {
			light: { l: 0.5, c: 0.1, textL: 0.45, textC: 0.1 },
			dark: { l: 0.7, c: 0.1, textL: 0.75, textC: 0.1 }
		},
		floors: { light: ok, dark: ok }
	});

	afterEach(() => {
		appearance.preview(null);
		vi.unstubAllGlobals();
	});

	it('installs a theme, and keeps the last good look when a change fails the floors', async () => {
		const page = fakePage();
		const doc = { ...blank('playful'), name: 'Bright' };
		vi.mocked(build).mockReturnValueOnce(built(doc.id, 'html[data-custom-theme] { --a: 1; }'));
		appearance.preview({ doc });
		appearance.apply();
		await vi.dynamicImportSettled();
		expect(page.style()).toBe('html[data-custom-theme] { --a: 1; }');
		expect(page.root.dataset.customTheme).toBe(doc.id);
		expect(page.root.dataset.theme).toBe('playful');

		// An edit that makes text unreadable is not shown.
		const worse = { ...doc, light: { 'surface-0': { oklch: [0.2, 0, 0] } } } as typeof doc;
		vi.mocked(build).mockReturnValueOnce(
			built(doc.id, 'html[data-custom-theme] { --a: 2; }', false)
		);
		appearance.preview({ doc: worse });
		appearance.apply();
		await vi.dynamicImportSettled();
		expect(page.style()).toBe('html[data-custom-theme] { --a: 1; }');
		expect(appearance.notApplied).toBe(doc.id);

		// Undoing the edit shows the good version again, and clears the refusal.
		appearance.preview({ doc });
		appearance.apply();
		expect(page.style()).toBe('html[data-custom-theme] { --a: 1; }');
		expect(appearance.notApplied).toBeNull();

		// Refused again, then back to a built-in: the refusal goes with it.
		vi.mocked(build).mockReturnValueOnce(
			built(doc.id, 'html[data-custom-theme] { --a: 2; }', false)
		);
		appearance.preview({ doc: worse });
		appearance.apply();
		await vi.dynamicImportSettled();
		expect(appearance.notApplied).toBe(doc.id);
		appearance.preview({ doc: null, character: 'calm' });
		appearance.apply();
		expect(page.style()).toBeUndefined();
		expect(page.root.dataset.customTheme).toBeUndefined();
		expect(page.root.dataset.theme).toBe('calm');
		expect(appearance.notApplied).toBeNull();
	});

	it('shows the base of an unreadable theme when no good version of it is showing', async () => {
		const page = fakePage();
		const other = { ...blank('calm'), name: 'Other' };
		vi.mocked(build).mockReturnValueOnce(built(other.id, 'html[data-custom-theme] { --b: 1; }'));
		appearance.preview({ doc: other });
		appearance.apply();
		await vi.dynamicImportSettled();
		expect(page.root.dataset.customTheme).toBe(other.id);

		const unreadable = { ...blank('contrast'), name: 'Unreadable' };
		vi.mocked(build).mockReturnValueOnce(built(unreadable.id, 'html { --c: 1; }', false));
		appearance.preview({ doc: unreadable });
		appearance.apply();
		await vi.dynamicImportSettled();
		expect(appearance.notApplied).toBe(unreadable.id);
		expect(page.style()).toBeUndefined();
		expect(page.root.dataset.customTheme).toBeUndefined();
		expect(page.root.dataset.theme).toBe('contrast');

		// Another theme in use: the refusal of this one is over.
		appearance.preview({ doc: other });
		appearance.apply();
		expect(appearance.notApplied).toBeNull();
	});
});
