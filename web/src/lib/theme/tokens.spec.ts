// The theme token registry (shared/theme-tokens.json) against the code:
// every custom property a component reads is a token a theme can set or
// one of the few INTERNAL ones, no component draws a raw color, and every
// token is declared in tokens.css and used. Together these keep every
// element of the app reachable from a theme.
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { describe, expect, it } from 'vitest';
import { byName, FAMILIES, GROUPS, INTERNAL, RENAMED, TOKENS } from './tokens';

const src = join(import.meta.dirname, '../..');
const TOKENS_CSS = 'lib/styles/tokens.css';

function walk(dir: string): string[] {
	return readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
		e.isDirectory() ? walk(join(dir, e.name)) : [join(dir, e.name)]
	);
}

const files = walk(src)
	.filter((f) => /\.(svelte|css|ts|html)$/.test(f) && !/\.(spec|test)\.ts$/.test(f))
	.filter((f) => !f.endsWith('.d.ts'))
	.map((f) => ({
		path: relative(src, f),
		text: readFileSync(f, 'utf8').replace(/\/\*[\s\S]*?\*\//g, '')
	}));

const css = files.find((f) => f.path === TOKENS_CSS)!.text;

/** CSS's named colors that could be mistaken for a theme's. */
const NAMED = [
	'white',
	'black',
	'red',
	'green',
	'blue',
	'gray',
	'grey',
	'silver',
	'yellow',
	'orange',
	'purple',
	'pink',
	'navy',
	'teal',
	'maroon',
	'olive',
	'lime',
	'aqua',
	'fuchsia',
	'cyan',
	'magenta'
];
const known = (name: string) => !!byName(name) || INTERNAL.has(name);

/** The CSS a file holds: a stylesheet whole, a component's <style> block. */
function styles(file: { path: string; text: string }): string {
	if (file.path.endsWith('.css')) return file.text;
	if (file.path.endsWith('.svelte') || file.path.endsWith('.html'))
		return [...file.text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');
	return '';
}

/** Every declaration of each token in tokens.css, in the file's order. */
function declared(): Map<string, string[]> {
	const out = new Map<string, string[]>();
	for (const [, name, value] of css.matchAll(/--([a-z0-9-]+):\s*([^;]+);/g)) {
		out.set(name, [...(out.get(name) ?? []), value.replace(/\s+/g, ' ').trim()]);
	}
	return out;
}

describe('the token registry', () => {
	it('names every custom property the code reads', () => {
		const unknown: string[] = [];
		for (const f of files) {
			for (const [, name] of f.text.matchAll(/var\(--([a-z0-9-]+)\s*[,)]/g))
				if (!known(name)) unknown.push(`${f.path}: --${name}`);
		}
		expect(unknown).toEqual([]);
	});

	it('names every member of a family built in code', () => {
		const problems: string[] = [];
		for (const f of files) {
			for (const [, prefix, suffix] of f.text.matchAll(
				/var\(--([a-z0-9-]*)\$\{[^}]*\}([a-z0-9-]*)\)/g
			)) {
				// Any token by name: the style guide showing them, the serializer
				// writing a ref it checked against the registry, and resolve.ts
				// reading the tokens it is asked for.
				const anyToken =
					f.path.startsWith('routes/dev/ui/') ||
					['lib/theme/css.ts', 'lib/theme/resolve.ts'].includes(f.path);
				if (prefix === '' && suffix === '' && anyToken) continue;
				const family = `${prefix}*${suffix}`;
				const members = FAMILIES[family];
				if (!members) {
					problems.push(`${f.path}: var(--${family}) is not a family in tokens.ts`);
					continue;
				}
				for (const m of members)
					if (!known(`${prefix}${m}${suffix}`))
						problems.push(`${f.path}: --${prefix}${m}${suffix}`);
			}
		}
		expect(problems).toEqual([]);
	});

	it('leaves theme tokens to tokens.css: components set only internal properties', () => {
		const problems: string[] = [];
		for (const f of files) {
			if (f.path === TOKENS_CSS) continue;
			const set = [
				...[...styles(f).matchAll(/(?:^|[;{\s])--([a-z0-9-]+)\s*:/g)].map((m) => m[1]),
				...[...f.text.matchAll(/style:--([a-z0-9-]+)/g)].map((m) => m[1]),
				...[...f.text.matchAll(/setProperty\(\s*'--([a-z0-9-]+)'/g)].map((m) => m[1])
			];
			for (const name of set) if (!INTERNAL.has(name)) problems.push(`${f.path}: --${name}`);
		}
		expect(problems).toEqual([]);
	});

	it('finds no raw color outside tokens.css', () => {
		const problems: string[] = [];
		// A color function of numbers, or of numbers worked out in code.
		const literal = /\b(?:oklch|oklab|rgba?|hsla?|hwb|lab|lch)\(\s*(?:[\d.]|\$\{)/;
		// A hex color, alone or inside a longer value.
		const hex = /(?:^|[\s:(,'"`])#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})(?![\w-])/;
		// A named color in a value (color-mix's too), not in a property's
		// name (white-space) or a token's (--palette-red).
		const named = new RegExp(`:[^;{}]*(?<![\\w-])(?:${NAMED.join('|')})(?![\\w-])`, 'i');
		// css.ts, the one serializer, writes a theme's colors, which come
		// from the registry; app.html's theme-color metas, which CSS cannot
		// set, are held to the Standard page color by install.spec.ts.
		const engine = (path: string) => path === 'lib/theme/css.ts';
		const metas = /<meta name="theme-color" content="#[0-9a-f]{6}"/g;
		const decl = declared();
		// A var() may fall back to the token's own value, as tokens.css
		// writes it (a ::backdrop in a browser that does not inherit custom
		// properties into it); that is the token, not a color of its own.
		const fallback = /var\(--([a-z0-9-]+), ((?:[^()]|\([^()]*\))+)\)/g;
		const withoutFallbacks = (line: string) =>
			line.replace(fallback, (all, name: string, value: string) =>
				decl.get(name)?.[0] === value.trim() ? `var(--${name})` : all
			);
		for (const f of files) {
			if (f.path === TOKENS_CSS) continue;
			const sheet = styles(f);
			for (const raw of sheet.split('\n')) {
				const line = withoutFallbacks(raw);
				if (literal.test(line) || hex.test(line) || named.test(line))
					problems.push(`${f.path}: ${line.trim()}`);
			}
			// Colors written into markup or code as strings, and inline styles.
			if (!f.path.endsWith('.css') && !engine(f.path)) {
				const code = (
					f.path.endsWith('.ts') ? f.text : f.text.replace(/<style[^>]*>[\s\S]*?<\/style>/g, '')
				)
					.replace(metas, '')
					.replace(/<!--[\s\S]*?-->/g, '');
				for (const [s] of code.matchAll(/(['"`])(?:(?!\1)[^\\\n]|\\.)*\1/g))
					if (literal.test(s) || hex.test(s)) problems.push(`${f.path}: ${s}`);
				for (const [, style] of code.matchAll(/\bstyle(?::[a-z-]+)?="([^"]*)"/g))
					if (named.test(`:${style}`)) problems.push(`${f.path}: style="${style}"`);
			}
		}
		expect(problems).toEqual([]);
	});

	it('is declared in tokens.css, token by token', () => {
		const decl = declared();
		expect(TOKENS.filter((t) => !decl.has(t.name)).map((t) => t.name)).toEqual([]);
	});

	it('has every token used somewhere', () => {
		const all = files.map((f) => f.text).join('\n');
		const used = new Set([
			...[...all.matchAll(/var\(--([a-z0-9-]+)\s*[,)]/g)].map((m) => m[1]),
			...[...all.matchAll(/'--([a-z0-9-]+)'/g)].map((m) => m[1])
		]);
		for (const [family, members] of Object.entries(FAMILIES)) {
			const [prefix, suffix] = family.split('*');
			if (all.includes(`--${prefix}\${`) && new RegExp(`\\}${suffix}[)\`]`).test(all))
				for (const m of members) used.add(`${prefix}${m}${suffix}`);
		}
		expect(TOKENS.filter((t) => !used.has(t.name)).map((t) => t.name)).toEqual([]);
	});

	it('keeps tokens.css keyed on attributes, never :root[...]', () => {
		expect(css).not.toContain(':root[');
	});

	it('gives every token a known group, type, label and use, and numbers their bounds', () => {
		const groups = new Set(GROUPS.map((g) => g.id));
		const types = ['color', 'px', 'ms', 'number', 'font', 'ease', 'shadow', 'hue'];
		const numeric = ['px', 'ms', 'number'];
		const names = new Set<string>();
		for (const t of TOKENS) {
			expect(names.has(t.name), `${t.name} twice`).toBe(false);
			names.add(t.name);
			expect(t.name).toMatch(/^[a-z][a-z0-9-]*[a-z0-9]$/);
			expect(groups.has(t.group), `${t.name} group`).toBe(true);
			expect(types, `${t.name} type`).toContain(t.type);
			expect([0, 1, 2, 3], `${t.name} rank`).toContain(t.rank);
			expect(t.label.length, `${t.name} label`).toBeGreaterThan(0);
			expect(t.used.length, `${t.name} used`).toBeGreaterThan(0);
			expect(INTERNAL.has(t.name), `${t.name} is internal`).toBe(false);
			if (t.type === 'color') expect(typeof t.alpha, `${t.name} alpha`).toBe('boolean');
			if (numeric.includes(t.type)) {
				expect(t.min! <= t.max!, `${t.name} bounds`).toBe(true);
				expect(t.step! > 0, `${t.name} step`).toBe(true);
			}
			// Only colors and the numbers tuned per scheme differ by scheme.
			if (t.perScheme) expect(['color', 'number'], `${t.name} per scheme`).toContain(t.type);
		}
		for (const g of GROUPS) expect(g.label.length).toBeGreaterThan(0);
		for (const [from, to] of Object.entries(RENAMED)) {
			expect(names.has(from), `renamed ${from} is still a token`).toBe(false);
			expect(names.has(to), `renamed to ${to}`).toBe(true);
		}
	});

	it('has every default ref point to a lower rank of the same kind, as tokens.css sets it', () => {
		const decl = declared();
		for (const t of TOKENS) {
			if (!t.default) continue;
			const target = byName(t.default.ref);
			expect(target, `${t.name} → ${t.default.ref}`).toBeDefined();
			expect(target!.type, `${t.name} type`).toBe(t.type);
			expect(target!.perScheme, `${t.name} scheme`).toBe(t.perScheme);
			expect(target!.rank, `${t.name} rank`).toBeLessThan(t.rank);
			if (target!.alpha) expect(t.alpha, `${t.name} alpha`).toBe(true);
			// Its first declaration, in the rule every theme starts from;
			// a character may set its own after.
			expect(decl.get(t.name)?.[0], `${t.name} in tokens.css`).toBe(`var(--${t.default.ref})`);
		}
	});
});
