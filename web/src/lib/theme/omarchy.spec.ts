import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import shipped from './presets/omarchy.json' with { type: 'json' };
import { hexToOklch } from './color';
import { LIMITS, parseTheme } from './document';
import { buildPresets, presetId, readBase, readColors, shortfalls } from './omarchy';
import { isPreset } from './presetId';
import type { Preset } from './presets.svelte';

const base = readBase(readFileSync(new URL('../styles/tokens.css', import.meta.url), 'utf8'));
const list = shipped.themes as unknown as Preset[];

const hueGap = (a: number, b: number) => Math.min(Math.abs(a - b), 360 - Math.abs(a - b));

describe('the themes the app ships', () => {
	it('are every Omarchy theme, Catppuccin and its Latte as one', () => {
		expect(list.length).toBeGreaterThanOrEqual(20);
		expect(list.map((p) => p.slug)).not.toContain('catppuccin-latte');
		expect(list.find((p) => p.slug === 'catppuccin')?.mode).toBe('both');
		expect(list.find((p) => p.slug === 'tokyo-night')?.mode).toBe('dark');
		expect(list.find((p) => p.slug === 'flexoki-light')?.mode).toBe('light');
	});

	it('each reads back as stored, under an id of its own that marks it as one', () => {
		const ids = new Set<string>();
		const names = new Set<string>();
		for (const p of list) {
			expect(parseTheme(p.doc, 'stored').theme, p.slug).toEqual(p.doc);
			expect(p.doc.id).toBe(presetId(p.slug));
			expect(isPreset(p.doc.id)).toBe(true);
			expect(p.doc.base).toBe('standard');
			expect(p.doc.accent).toEqual({ manual: true });
			expect(new TextEncoder().encode(JSON.stringify(p.doc)).length).toBeLessThanOrEqual(
				LIMITS.bytes
			);
			ids.add(p.doc.id);
			names.add(p.doc.name.toLowerCase());
		}
		expect(ids.size).toBe(list.length);
		expect(names.size).toBe(list.length);
	});

	it('read at AA in both schemes', () => {
		for (const p of list)
			for (const scheme of ['light', 'dark'] as const)
				expect(shortfalls(p.doc, scheme), `${p.slug} ${scheme}`).toEqual([]);
	});

	it("keep urgent red where the palette's red is another hue", () => {
		// Lumon's colors are all blues.
		const lumon = list.find((p) => p.slug === 'lumon')!;
		for (const scheme of ['light', 'dark'] as const) {
			const red = lumon.doc[scheme]['palette-red'] as { oklch: [number, number, number] };
			expect(hueGap(red.oklch[2], base.hues.red)).toBeLessThan(1);
		}
	});
});

// Made-up palettes in Omarchy's shape.
const night = `mode = "dark"

accent = "#6d9df0"
selection = "#2a2f45"
muted = "#3f4566"

background = "#181a24"
lighter_background = "#22263a"

foreground = "#a8b0d4"
dark_foreground = "#525a82"

red = "#f07088"
yellow = "#dcaa60"
green = "#98c868"
blue = "#6d9df0"
magenta = "#a888e0"
`;
const paper = `mode = "light"
accent = "#2050a0"
selection = "#d0d4dc"
muted = "#a8acba"
background = "#f0f2f6"
lighter_background = "#dde1e9"
foreground = "#484c66"
dark_foreground = "#9a9eae"
red = "#d02040"
green = "#40a020"
blue = "#2050a0"
`;
// Every named color a shade of blue, as in Lumon.
const ice = night.replace(/^(red|yellow|green|magenta) = "#[0-9a-f]+"$/gm, '$1 = "#4d86b0"');

describe('making them', () => {
	it('reads colors.toml lines', () => {
		expect(readColors(night)).toMatchObject({ mode: 'dark', accent: '#6d9df0', red: '#f07088' });
	});

	it('keeps the page and text, lifts faint text to AA, and makes the scheme Omarchy lacks', () => {
		const [p] = buildPresets({ night: readColors(night) }, base);
		expect(p.mode).toBe('dark');
		const bg = hexToOklch('#181a24')!;
		const page = p.doc.dark['surface-0'] as { oklch: number[] };
		expect(page.oklch[0]).toBeCloseTo(bg[0], 2);
		const muted = p.doc.dark['text-muted'] as { oklch: number[] };
		expect(muted.oklch[0]).toBeGreaterThan(hexToOklch('#525a82')![0]);
		// The light one is light, in the same hue family.
		const light = p.doc.light['surface-0'] as { oklch: number[] };
		expect(light.oklch[0]).toBeGreaterThan(0.95);
		expect(hueGap(light.oklch[2], bg[2])).toBeLessThan(1);
		for (const scheme of ['light', 'dark'] as const) expect(shortfalls(p.doc, scheme)).toEqual([]);
	});

	it('makes a pair Omarchy keeps as two folders into one theme', () => {
		const out = buildPresets(
			{ catppuccin: readColors(night), 'catppuccin-latte': readColors(paper) },
			base
		);
		expect(out.map((p) => [p.slug, p.mode, p.doc.name])).toEqual([
			['catppuccin', 'both', 'Catppuccin']
		]);
		const light = out[0].doc.light['surface-0'] as { oklch: number[] };
		expect(light.oklch[0]).toBeCloseTo(hexToOklch('#f0f2f6')![0], 2);
	});

	it("uses the palette's own label colors where it has the hue, and Mila's where not", () => {
		const [warm] = buildPresets({ night: readColors(night) }, base);
		const [cold] = buildPresets({ ice: readColors(ice) }, base);
		const red = (p: Preset) => (p.doc.dark['palette-red'] as { oklch: number[] }).oklch[2];
		expect(hueGap(red(warm), hexToOklch('#f07088')![2])).toBeLessThan(1);
		expect(hueGap(red(cold), base.hues.red)).toBeLessThan(1);
	});

	it('gives each theme the same id every time', () => {
		expect(presetId('tokyo-night')).toBe(presetId('tokyo-night'));
		expect(presetId('tokyo-night')).not.toBe(presetId('nord'));
		expect(isPreset(presetId('nord'))).toBe(true);
		expect(isPreset('0b0e9f39-2a43-4d9c-9a6e-31f5c0a1d001')).toBe(false);
	});
});
