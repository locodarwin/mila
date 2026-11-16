// Draws the app's PNG icons from the favicon art (static/favicon.svg) with
// the Playwright browser: `npx tsx scripts/app-icons.ts` from web/. Run it
// again after changing the favicon, and commit the PNGs.
//
//   icon-<size>.png           the favicon as it is, rounded corners and all
//   icon-maskable-<size>.png  full bleed, the mark inside the middle 60% so
//                             any launcher's mask (circle, squircle) keeps it
//   apple-touch-icon.png      full bleed at 180px; iOS rounds the corners
import { chromium } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const STATIC = join(import.meta.dirname, '../static');
const favicon = readFileSync(join(STATIC, 'favicon.svg'), 'utf8');
const fill = /<rect[^>]*fill="([^"]+)"/.exec(favicon)?.[1] ?? '#5b5bd6';
const mark = /<path[^>]*\/>/.exec(favicon)?.[0] ?? '';

/** The mark alone on a square of the icon's color, scaled about the middle. */
function fullBleed(scale: number): string {
	const t = 16 - 16 * scale;
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" fill="${fill}"/><g transform="translate(${t} ${t}) scale(${scale})">${mark}</g></svg>`;
}

const icons: { file: string; size: number; svg: string }[] = [
	{ file: 'icon-192.png', size: 192, svg: favicon },
	{ file: 'icon-512.png', size: 512, svg: favicon },
	{ file: 'icon-maskable-192.png', size: 192, svg: fullBleed(0.6) },
	{ file: 'icon-maskable-512.png', size: 512, svg: fullBleed(0.6) },
	{ file: 'apple-touch-icon.png', size: 180, svg: fullBleed(0.75) }
];

const browser = await chromium.launch();
try {
	for (const { file, size, svg } of icons) {
		const page = await browser.newPage({ viewport: { width: size, height: size } });
		await page.setContent(
			`<style>html,body{margin:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`
		);
		await page.screenshot({ path: join(STATIC, file), omitBackground: true });
		await page.close();
	}
} finally {
	await browser.close();
}
