// The browser's own bars and the installed app's splash screen use plain
// hex colors that cannot read the tokens: they must match the page color
// the tokens give (pageColor), in app.html and the manifest alike.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { pageColor, toHex } from './color';

const web = join(import.meta.dirname, '../../..');

describe('the page color outside the page', () => {
	it('converts to hex as CSS does', () => {
		expect(toHex([1, 1, 1])).toBe('#ffffff');
		expect(toHex([0, 0, 0])).toBe('#000000');
		expect(toHex([0.2159, 0.2159, 0.2159])).toBe('#808080');
		expect(pageColor('contrast', 'dark')).toBe('#000000');
	});
	it('matches the theme-color metas in app.html', () => {
		const html = readFileSync(join(web, 'src/app.html'), 'utf8');
		const meta = (scheme: string) =>
			new RegExp(
				`<meta name="theme-color" content="(#[0-9a-f]{6})" media="\\(prefers-color-scheme: ${scheme}\\)"`
			).exec(html)?.[1];
		expect(meta('light')).toBe(pageColor('standard', 'light'));
		expect(meta('dark')).toBe(pageColor('standard', 'dark'));
	});
	it('matches the manifest', () => {
		const manifest = JSON.parse(readFileSync(join(web, 'static/manifest.json'), 'utf8'));
		expect(manifest.background_color).toBe(pageColor('standard', 'light'));
		expect(manifest.theme_color).toBe(pageColor('standard', 'light'));
	});
});
