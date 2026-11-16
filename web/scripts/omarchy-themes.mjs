// Writes src/lib/theme/presets/omarchy.json, the themes made from
// Omarchy's (src/lib/theme/omarchy.ts), from a folder of Omarchy themes:
// its themes/ folder, each theme a folder holding colors.toml. Run it when
// Omarchy adds or changes a theme, look at them in the app, and commit
// the file.
//
//   git clone --depth 1 https://github.com/basecamp/omarchy /tmp/omarchy
//   node scripts/omarchy-themes.mjs /tmp/omarchy/themes
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { runnerImport } from 'vite';

const dir = process.argv[2];
if (!dir || !existsSync(dir)) {
	console.error('usage: node scripts/omarchy-themes.mjs <omarchy>/themes');
	process.exit(2);
}

const web = new URL('..', import.meta.url).pathname;
const { module: omarchy } = await runnerImport(join(web, 'src/lib/theme/omarchy.ts'), {
	configFile: false,
	root: web,
	logLevel: 'error'
});

const palettes = {};
for (const e of readdirSync(dir, { withFileTypes: true })) {
	const file = join(dir, e.name, 'colors.toml');
	if (e.isDirectory() && existsSync(file))
		palettes[e.name] = omarchy.readColors(readFileSync(file, 'utf8'));
}

let commit = null;
try {
	commit = execFileSync('git', ['-C', dir, 'rev-parse', '--short', 'HEAD'], {
		encoding: 'utf8'
	}).trim();
} catch {
	// not a clone: the file says so
}

const base = omarchy.readBase(readFileSync(join(web, 'src/lib/styles/tokens.css'), 'utf8'));
const themes = omarchy.buildPresets(palettes, base);
const out = join(web, 'src/lib/theme/presets');
mkdirSync(out, { recursive: true });
// A theme a line: small for the app to load, and a change to one theme is a change to its line.
const lines = themes.map((t) => '\t\t' + JSON.stringify(t)).join(',\n');
writeFileSync(
	join(out, 'omarchy.json'),
	`{\n\t"source": "github.com/basecamp/omarchy",\n\t"commit": ${JSON.stringify(commit)},\n\t"themes": [\n${lines}\n\t]\n}\n`
);
console.log(`${themes.length} themes from ${Object.keys(palettes).length} folders`);
