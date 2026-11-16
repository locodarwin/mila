// What the documentation's pictures share: the server they're taken on, the
// workspace docs.setup.ts seeds there, and taking a picture in light and dark.
// `make shots` runs them all, in the pinned Playwright image.
import { expect, type Locator, type Page } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

export const SHOTS_SERVER = 'http://localhost:4177';
export const DATA = join(import.meta.dirname, '../../../tmp/shots');
export const SEED = join(DATA, 'seed.json');
export const STATE = join(DATA, 'state.json');
/** The project the views show, and the task whose panel is shown. */
export const PROJECT = 'Roastery';
export const TASK = 'Service the afterburner';

/** The sample workspace's JSON export, written by scripts/sample-data.py when the server starts. */
export const SAMPLE = join(DATA, 'sample/mila-sample-small-business.zip');

/** What the setup made, for the pictures to find their way. */
export interface Seed {
	ws: string;
	/** When the pictures are taken: the app's clock stays here, so every picture tells the same day. */
	now: string;
	/** Projects, tasks and pages by name. */
	projects: Record<string, string>;
	tasks: Record<string, string>;
	pages: Record<string, string>;
	views: Record<string, string>;
}

export const seed = () => JSON.parse(readFileSync(SEED, 'utf8')) as Seed;

/** Calls the API from inside the page, so cookies and Origin are real. */
export async function api(page: Page, method: string, path: string, body?: unknown) {
	const out = await page.evaluate(
		async ([method, path, body]) => {
			const res = await fetch('/api/v1' + path, {
				method,
				headers: { 'Content-Type': 'application/json' },
				body: body === undefined ? undefined : JSON.stringify(body)
			});
			const text = await res.text();
			return { status: res.status, body: text ? JSON.parse(text) : null };
		},
		[method, path, body] as const
	);
	expect(out.status, `${method} ${path}: ${JSON.stringify(out.body)}`).toBeLessThan(300);
	return out.body;
}

export async function prefer(page: Page, preferences: Record<string, unknown>) {
	if (page.url() === 'about:blank') await page.goto('/');
	await api(page, 'PATCH', '/me/settings/preferences', preferences);
}

/** One picture: how to get the page there, and what part of it to keep. */
export interface Shot {
	name: string;
	/** Gets the page ready and says what to take. */
	take: (page: Page, s: Seed) => Promise<Locator>;
	preferences?: Record<string, unknown>;
	/** For a layout that needs more or less room than most. */
	viewport?: { width: number; height: number };
	/** The part of what's taken to keep, in CSS pixels, where its end is empty. */
	top?: number;
	height?: number;
	/** CSS for the picture, to leave something out. */
	style?: string;
}

/** A PNG as WebP, by the browser, so the pictures need no other tool. */
async function webp(page: Page, png: Buffer, quality: number) {
	const out = await page.evaluate(
		async ([data, quality]) => {
			const image = new Image();
			image.src = data as string;
			await image.decode();
			const canvas = document.createElement('canvas');
			canvas.width = image.naturalWidth;
			canvas.height = image.naturalHeight;
			canvas.getContext('2d')!.drawImage(image, 0, 0);
			return {
				data: canvas.toDataURL('image/webp', quality as number),
				size: [image.naturalWidth, image.naturalHeight] as [number, number]
			};
		},
		[`data:image/png;base64,${png.toString('base64')}`, quality] as const
	);
	expect(out.data).toMatch(/^data:image\/webp/);
	return { bytes: Buffer.from(out.data.split(',')[1], 'base64'), size: out.size };
}

/**
 * Takes each shot in light and dark, as `<name>-<scheme>.webp` in `out`, and
 * returns each one's size in pixels.
 */
export async function takeAll(
	page: Page,
	shots: Shot[],
	out: string,
	{ viewport, quality = 0.9 }: { viewport: { width: number; height: number }; quality?: number }
) {
	const s = seed();
	await page.clock.setFixedTime(new Date(s.now));
	const sizes: Record<string, [number, number]> = {};
	for (const scheme of ['light', 'dark']) {
		await prefer(page, { theme: 'standard', scheme });
		for (const shot of shots) {
			await prefer(page, { taskPanel: 'boxes', ...shot.preferences });
			await page.setViewportSize(shot.viewport ?? viewport);
			const target = await shot.take(page, s);
			await expect(page.locator('html')).toHaveAttribute('data-scheme', scheme);
			await page.waitForLoadState('networkidle');
			await page.evaluate(() => document.fonts.ready);
			const options = { animations: 'disabled', caret: 'hide', style: shot.style } as const;
			const box = await target.boundingBox();
			if (!box) throw new Error(`${shot.name} has nothing to show`);
			const top = shot.top ?? 0;
			const png =
				shot.top === undefined && shot.height === undefined
					? await target.screenshot(options)
					: await page.screenshot({
							...options,
							clip: {
								x: box.x,
								y: box.y + top,
								width: box.width,
								height: Math.min(shot.height ?? box.height - top, box.height - top)
							}
						});
			const { bytes, size } = await webp(page, png, quality);
			writeFileSync(join(out, `${shot.name}-${scheme}.webp`), bytes);
			sizes[shot.name] = size;
			// Whatever a shot opened (a menu, a dialog) goes before the next.
			await page.keyboard.press('Escape');
			await page.mouse.move(0, 0);
		}
	}
	return sizes;
}

/** Sizes as JSON, as the formatter would write it, so that make lint stays happy. */
export function sizesJson(sizes: Record<string, [number, number]>) {
	const lines = Object.entries(sizes)
		.toSorted(([a], [b]) => a.localeCompare(b))
		.map(([name, [w, h]]) => `\t"${name}": [${w}, ${h}]`);
	return `{\n${lines.join(',\n')}\n}\n`;
}
