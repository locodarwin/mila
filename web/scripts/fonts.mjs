// Fills in the font files index.html's inline script preloads for the
// saved theme (src/app.html), so a page's text does not reflow when its
// font arrives after it was drawn. Run by `npm run build`, after vite
// build, since the files' names carry a hash of their contents.

import { readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const build = fileURLToPath(new URL('../build', import.meta.url));

// The Latin files of each family, which the app's text uses.
const assets = readdirSync(join(build, '_app/immutable/assets'));
const font = (family) => {
	const file = assets.find(
		(f) => f.startsWith(`${family}-latin-wght-normal.`) && f.endsWith('.woff2')
	);
	if (!file) throw new Error(`no ${family} font in the build`);
	return `/_app/immutable/assets/${file}`;
};
const fonts = { inter: font('inter'), serif: font('source-serif-4'), nunito: font('nunito') };

const indexPath = join(build, 'index.html');
const marker = '/* mila:fonts */ {}';
const index = readFileSync(indexPath, 'utf8');
if (!index.includes(marker)) throw new Error(`index.html has no ${marker} to fill in`);
writeFileSync(indexPath, index.replace(marker, JSON.stringify(fonts)));
// The precompressed copies are of the page before; the server sends
// index.html itself.
rmSync(indexPath + '.br', { force: true });
rmSync(indexPath + '.gz', { force: true });
console.log('fonts.mjs: index.html preloads', Object.keys(fonts).join(', '));
