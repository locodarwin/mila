import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// The user guide (docs/guide), published to the wiki by scripts/guide-to-wiki.py,
// and its pictures, made by `make shots` (web/e2e/shots/guide.shots.ts).
const GUIDE = join(import.meta.dirname, '..', '..', '..', '..', 'docs', 'guide');
const IMAGES = join(GUIDE, 'images');
const pages = readdirSync(GUIDE).filter((f) => f.endsWith('.md'));
const text = (page: string) => readFileSync(join(GUIDE, page), 'utf8');

describe("The guide's pictures", () => {
	it('are each named alone on a line, light first, saying what they show', () => {
		for (const page of pages) {
			const named = [...text(page).matchAll(/images\//g)].length;
			const whole = [
				...text(page).matchAll(/^!\[([^\]]*)\]\(images\/([a-z0-9-]+)-light\.webp\)$/gm)
			];
			// The wiki publisher shows the dark one in dark mode only for this form.
			expect(whole, page).toHaveLength(named);
			for (const m of whole) expect(m[1].length, `${page}: ${m[2]}`).toBeGreaterThan(30);
		}
	});

	it('exist in both schemes, and none is left unused', () => {
		const used = new Set(
			pages.flatMap((page) =>
				[...text(page).matchAll(/\(images\/([a-z0-9-]+)-light\.webp\)/g)].map((m) => m[1])
			)
		);
		const files = readdirSync(IMAGES).toSorted();
		expect(files).toEqual(
			[...used].flatMap((name) => [`${name}-dark.webp`, `${name}-light.webp`]).toSorted()
		);
	});
});
