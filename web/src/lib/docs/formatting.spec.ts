import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { FONTS } from '../theme/document';
import {
	COLUMN_LAYOUTS,
	FONT_NAMES,
	FONT_STACKS,
	HIGHLIGHT_COLORS,
	IMAGE_SIDES,
	IMAGE_SIZES,
	imageMarkers,
	PAGE_FONTS,
	PAGE_SIZES,
	WORD_SIZES
} from './formatting';

describe('the formatting beyond Markdown', () => {
	it("offers the fonts themes pick from, as they're written there", () => {
		expect(Object.fromEntries(PAGE_FONTS.map((f) => [f, FONT_STACKS[f]]))).toEqual(FONTS);
		for (const f of PAGE_FONTS) expect(FONT_NAMES[f]).toBeTruthy();
	});

	it("names what the server's Markdown names (ydoc/formatting.go)", () => {
		const go = readFileSync(
			new URL('../../../../server/internal/ydoc/formatting.go', import.meta.url),
			'utf8'
		);
		const list = (name: string) => {
			const m = new RegExp(`${name}\\s*=\\s*\\[\\][a-z0-9]+\\{([^}]*)\\}`).exec(go);
			return m ? [...m[1].matchAll(/"([^"]+)"|(\d+)/g)].map((x) => x[1] ?? Number(x[2])) : null;
		};
		expect(list('HighlightColors')).toEqual([...HIGHLIGHT_COLORS]);
		expect(list('PageFonts')).toEqual([...PAGE_FONTS]);
		expect(list('WordSizes')).toEqual([...WORD_SIZES]);
		expect(list('PageSizes')).toEqual([...PAGE_SIZES]);
		expect(list('ImageSizes')).toEqual([...IMAGE_SIZES]);
		expect(list('ImageSides')).toEqual([...IMAGE_SIDES]);
		expect(go).toContain(
			`2: {"${COLUMN_LAYOUTS[2].join('", "')}"}, 3: {"${COLUMN_LAYOUTS[3].join('", "')}"}`
		);
	});

	it('takes up an image marker only right after an image, and a caption only under one alone', () => {
		const img = { type: 'image', attrs: { src: '/a.png' } };
		const marker = { type: 'text', text: '<!-- mila:image 50 right -->' };
		expect(imageMarkers([img, marker])).toEqual([
			{ ...img, attrs: { src: '/a.png', size: 50, side: 'right' } }
		]);
		expect(imageMarkers([marker])).toEqual([marker]);
		const caption = [
			img,
			{ type: 'text', text: '<!-- mila:image 25 left -->\n' },
			{ type: 'text', text: 'Beds', marks: [{ type: 'italic' }] }
		];
		expect(imageMarkers(caption)).toEqual([
			{ ...img, attrs: { src: '/a.png', size: 25, side: 'left', caption: 'Beds' } }
		]);
		const noMarker = [
			img,
			{ type: 'text', text: '\n' },
			{ type: 'text', text: 'Beds', marks: [{ type: 'italic' }] }
		];
		expect(imageMarkers(noMarker)).toEqual(noMarker);
	});
});
