// Custom themes: making one, the editor's preview, using it, the first
// paint, the floors, export and import (hostile files included), Fix,
// undo, deleting, another device following, the built-ins and the themes
// made from Omarchy's against every contrast pair, and reduced motion. A theme is a person's own, so each
// test signs up someone new.
import { readFileSync } from 'node:fs';
import { expect, test, type Browser, type Page } from '@playwright/test';
import { oklchToLinearSrgb, toHex } from '../src/lib/theme/color';
import { check, pairTokens } from '../src/lib/theme/contrast';
import { parseColor, type Resolved } from '../src/lib/theme/resolve';
import { axe } from './axe';
import { apiCall, OWNER, SIGNED_OUT, newPerson } from './helpers';

const SAMPLE = JSON.parse(
	readFileSync(new URL('./visual/fixtures/sample.mila-theme.json', import.meta.url), 'utf8')
);
const OMARCHY: { slug: string; mode: string; doc: { id: string; name: string } }[] = JSON.parse(
	readFileSync(new URL('../src/lib/theme/presets/omarchy.json', import.meta.url), 'utf8')
).themes;
const omarchy = (slug: string) => OMARCHY.find((p) => p.slug === slug)!;

test.use({ storageState: OWNER });
test.skip(({ isMobile }) => isMobile, 'the phone layout is checked in phone.e2e.ts');

async function person(browser: Browser, name: string) {
	const { page: own } = await newPerson(() => browser.newContext(SIGNED_OUT), name);
	await own.emulateMedia({ colorScheme: 'light' });
	return own;
}

/** Makes a theme on a built-in from the manager and opens it in the editor; returns its id. */
async function newTheme(page: Page, base = 'Calm'): Promise<string> {
	await page.goto('/settings/themes');
	await page.getByLabel('Start from').selectOption({ label: base });
	await page.getByRole('button', { name: 'New theme', exact: true }).click();
	await expect(page).toHaveURL(/\/settings\/themes\/[0-9a-f-]{36}$/);
	await expect(page.getByLabel('Theme name')).toBeVisible();
	return page.url().split('/').pop()!;
}

const group = (page: Page, name: string) =>
	page
		.getByRole('navigation', { name: 'Parts of the theme' })
		.getByRole('button', { name: new RegExp(`^${name}`) });

/** Opens a setting's row, which shows its editor, Reset, Fix and contrast in place. */
async function openRow(page: Page, label: string) {
	const row = page.getByRole('button', { name: new RegExp(`^${label}:`) });
	if ((await row.getAttribute('aria-expanded')) !== 'true') await row.click();
	await expect(row).toHaveAttribute('aria-expanded', 'true');
}

/** Sets a color token in the scheme being edited, by hex: opens its row, and closes it after. */
async function setColor(page: Page, label: string, hex: string) {
	await openRow(page, label);
	const hexField = page.getByLabel('Hex', { exact: true });
	await hexField.fill(hex);
	await hexField.press('Enter');
	await page.getByRole('button', { name: 'Done' }).click();
	await expect(page.getByLabel('Hex', { exact: true })).toBeHidden();
}

/** An element's background as #rrggbb, however the browser writes it. */
async function background(page: Page, selector: string) {
	const css = await page
		.locator(selector)
		.first()
		.evaluate((el) => getComputedStyle(el).backgroundColor);
	const c = parseColor(css);
	return c ? toHex(oklchToLinearSrgb(...c.lch)) : css;
}

/** An element's text color as #rrggbb, however the browser writes it. */
async function color(page: Page, selector: string) {
	const css = await page
		.locator(selector)
		.first()
		.evaluate((el) => getComputedStyle(el).color);
	const c = parseColor(css);
	return c ? toHex(oklchToLinearSrgb(...c.lch)) : css;
}

/** A duration custom property in milliseconds, however the browser writes it. */
function ms(value: string) {
	const m = /^([\d.]+)(m?s)$/.exec(value.trim());
	return m ? Math.round(Number(m[1]) * (m[2] === 's' ? 1000 : 1)) : NaN;
}

/** Waits until the server has the theme as the editor last saved it. */
async function savedTheme(page: Page, id: string, test: (t: Record<string, unknown>) => boolean) {
	await expect
		.poll(async () =>
			page
				.evaluate(async (id) => {
					const res = await fetch(`/api/v1/me/themes/${id}`);
					return res.ok ? res.json() : null;
				}, id)
				.then((t: Record<string, unknown> | null) => !!t && test(t))
		)
		.toBe(true);
}

test('a new theme changes the preview, not the app, until it is used, and paints first', async ({
	browser
}) => {
	const own = await person(browser, 'Thea');
	const id = await newTheme(own);
	await expect(own.getByText('Based on Calm')).toBeVisible();
	const appBefore = await background(own, 'body');
	await setColor(own, 'Page', '#f0e6d2');
	// The preview draws it at once; the app does not wear it yet.
	await expect.poll(() => background(own, '.theme-preview[data-scheme="light"]')).toBe('#f0e6d2');
	expect(await background(own, 'body')).toBe(appBefore);
	await expect(own.getByText('Changed', { exact: true })).toBeVisible();
	await savedTheme(own, id, (t) => JSON.stringify(t).includes('surface-0'));

	// Used, the app wears it.
	await own.getByRole('button', { name: 'Use this theme' }).click();
	await expect(own.locator('html')).toHaveAttribute('data-custom-theme', id);
	await expect.poll(() => background(own, 'body')).toBe('#f0e6d2');
	await expect(own.getByText('In use', { exact: true })).toBeVisible();

	// The next load paints it before any script runs: only the pre-paint
	// script, with the scripts blocked (not the stylesheets), can.
	await own.route(/\/_app\/immutable\/.*\.js$/, (r) => r.abort());
	await own.reload();
	await expect(own.locator('html')).toHaveAttribute('data-custom-theme', id);
	expect(await background(own, 'body')).toBe('#f0e6d2');
	await own.unroute(/\/_app\/immutable\/.*\.js$/);
	await own.context().close();
});

test('a theme below the floors cannot be used, and the theme in use keeps its last good look', async ({
	browser
}) => {
	const own = await person(browser, 'Flora');
	const id = await newTheme(own, 'Standard');
	await own.getByRole('button', { name: 'Use this theme' }).click();
	await expect(own.locator('html')).toHaveAttribute('data-custom-theme', id);
	const css = await own.locator('#mila-theme').evaluate((el) => el.textContent);
	const textBefore = await own.locator('body').evaluate((el) => getComputedStyle(el).color);

	// Text the color of the page: never installed, and said so.
	await group(own, 'Text').click();
	await setColor(own, 'Text', '#fdfdfd');
	await expect(
		own.getByText(/Not applied: text or the focus ring would be unreadable/)
	).toBeVisible();
	expect(await own.locator('#mila-theme').evaluate((el) => el.textContent)).toBe(css);
	expect(await own.locator('body').evaluate((el) => getComputedStyle(el).color)).toBe(textBefore);

	// Another theme like it cannot be put in use at all.
	await newTheme(own, 'Standard');
	await group(own, 'Text').click();
	await setColor(own, 'Text', '#fdfdfd');
	await expect(own.getByRole('button', { name: 'Use this theme' })).toBeDisabled();
	await expect(own.getByText(/Fix that to use this theme/)).toBeVisible();
	await own.context().close();
});

test('a theme exports to a file and imports back, and a hostile one is cleaned', async ({
	browser
}) => {
	const own = await person(browser, 'Ezra');
	await newTheme(own, 'Playful');
	await own.getByLabel('Theme name').fill('Night shift');
	await own.getByLabel('Theme name').press('Tab');
	await setColor(own, 'Page', '#fff8f0');
	await own.getByRole('link', { name: 'Back to themes' }).click();
	const rows = own.getByRole('list', { name: 'Your themes' });
	await expect(rows.getByText('Night shift')).toBeVisible();

	await rows.getByRole('button', { name: 'More for Night shift' }).click();
	const [file] = await Promise.all([
		own.waitForEvent('download'),
		own.getByRole('button', { name: 'Export file' }).click()
	]);
	expect(file.suggestedFilename()).toBe('Night shift.mila-theme.json');
	const text = readFileSync(await file.path(), 'utf8');
	expect(JSON.parse(text)).toMatchObject({ mila: 'theme', name: 'Night shift', base: 'playful' });

	await own.locator('#theme-file').setInputFiles({
		name: 'Night shift.mila-theme.json',
		mimeType: 'application/json',
		buffer: Buffer.from(text)
	});
	const dialog = own.getByRole('dialog', { name: 'Import Night shift (2)' });
	await expect(dialog).toBeVisible();
	await expect(dialog.getByText('Everything in the file can be used.')).toBeVisible();
	await dialog.getByRole('button', { name: 'Add theme' }).click();
	await expect(rows.getByText('Night shift (2)')).toBeVisible();

	// A hostile file: what Mila cannot use is left out, and said.
	const hostile = {
		mila: 'theme',
		version: 1,
		id: 'not-an-id',
		name: 'Bad </style><script>alert(1)</script>',
		base: 'standard',
		target: 'AA',
		accent: { hue: 30 },
		tokens: {
			'radius-md': { px: 9 },
			'font-ui': { font: "x; } body { background: url('https://evil.example/x') }" },
			'dur-panel': { ms: '1e999' },
			'made-up': { px: 3 }
		},
		light: {
			'surface-1': { oklch: [0.96, 0.02, 90] },
			'surface-0': { ref: 'text-primary; }' }
		},
		dark: {}
	};
	// A key no object literal can hold as its own.
	const pasted = JSON.stringify(hostile).replace(
		'"surface-1"',
		'"__proto__":{"polluted":true},"surface-1"'
	);
	await own.getByLabel('Or paste a theme').fill(pasted);
	await own.getByRole('button', { name: 'Check the pasted theme' }).click();
	const bad = own.getByRole('dialog', { name: /^Import Bad/ });
	await expect(bad).toBeVisible();
	await expect(bad.getByText(/settings? Mila cannot use (was|were) left out/)).toBeVisible();
	await expect(bad.getByText(/from a newer Mila (was|were) left out/)).toBeVisible();
	await bad.getByRole('button', { name: 'Add theme' }).click();
	const badRow = rows.getByRole('listitem').filter({ hasText: 'Bad' });
	await expect(badRow).toBeVisible();
	await badRow.getByRole('button', { name: /^Use / }).click();
	await expect(own.locator('html')).toHaveAttribute('data-custom-theme', /.+/);
	const css = (await own.locator('#mila-theme').textContent()) ?? '';
	expect(css).toContain('--radius-md: 9px;');
	expect(css).not.toMatch(/url\(|<|script|evil|1e999|made-up|polluted/);
	for (const [, value] of css.matchAll(/--[a-z0-9-]+: ([^;]*);/g))
		expect(value).toMatch(/^[0-9a-z.\-% (),/'"]+$/i);
	await own.context().close();
});

test('Fix clears a contrast warning', async ({ browser }) => {
	const own = await person(browser, 'Felix');
	await newTheme(own, 'Standard');
	await group(own, 'Text').click();
	await setColor(own, 'Muted text', '#c8c8c8');
	const row = own.locator('#token-text-muted');
	// The closed row marks it; open, it says why and offers Fix.
	await expect(row.getByRole('button', { name: /^Muted text:/ })).toContainText('contrast problem');
	await openRow(own, 'Muted text');
	await expect(row.getByText(/AA needs 4.5/).first()).toBeVisible();
	await expect(own.getByRole('button', { name: /^[1-9]\d* problems? at AA/ })).toBeVisible();
	await row.getByRole('button', { name: 'Fix Muted text' }).click();
	await expect(row.getByText(/AA needs/)).toHaveCount(0);
	await expect(row.getByRole('button', { name: 'Fix Muted text' })).toHaveCount(0);
	await own.context().close();
});

test('undo, redo and resets in the editor leave the app undo alone', async ({ browser }) => {
	const own = await person(browser, 'Uma');
	await own.getByLabel('New task').fill('Undo canary');
	await own.getByLabel('New task').press('Enter');
	await expect(
		own.locator('#main [data-task-id]').filter({ hasText: 'Undo canary' })
	).toBeVisible();

	await newTheme(own, 'Standard');
	const preview = '.theme-preview[data-scheme="light"]';
	const plain = await background(own, preview);
	await setColor(own, 'Page', '#e0f0ff');
	await expect.poll(() => background(own, preview)).toBe('#e0f0ff');

	// Focus is back on the swatch: the editor's undo, not the app's.
	await own.keyboard.press('Control+z');
	await expect.poll(() => background(own, preview)).toBe(plain);
	await own.keyboard.press('Control+Shift+z');
	await expect.poll(() => background(own, preview)).toBe('#e0f0ff');
	const history = own.getByRole('group', { name: 'History' });
	await history.getByRole('button', { name: 'Undo' }).click();
	await expect.poll(() => background(own, preview)).toBe(plain);
	await history.getByRole('button', { name: 'Redo' }).click();
	await expect.poll(() => background(own, preview)).toBe('#e0f0ff');

	// One token back to its base, then a whole group.
	await openRow(own, 'Page');
	await own.getByRole('button', { name: 'Reset Page' }).click();
	await expect.poll(() => background(own, preview)).toBe(plain);
	await setColor(own, 'Page', '#e0f0ff');
	await setColor(own, 'Raised surface', '#d0e0f0');
	await expect(group(own, 'Surfaces')).toContainText('2 changed');
	await own.getByRole('button', { name: 'Reset Surfaces' }).click();
	await expect(group(own, 'Surfaces')).not.toContainText('changed');
	await expect.poll(() => background(own, preview)).toBe(plain);

	// Revert to when it opened.
	await setColor(own, 'Page', '#e0f0ff');
	await own.getByRole('button', { name: 'More', exact: true }).click();
	await own.getByRole('button', { name: 'Revert to when I opened it' }).click();
	await expect.poll(() => background(own, preview)).toBe(plain);

	// The task made before is still there: no Ctrl+Z reached the app.
	await own.goto('/');
	await expect(
		own.locator('#main [data-task-id]').filter({ hasText: 'Undo canary' })
	).toBeVisible();
	await own.context().close();
});

test('a deleted theme comes back with undo', async ({ browser }) => {
	const own = await person(browser, 'Dot');
	await newTheme(own, 'Calm');
	await own.getByRole('link', { name: 'Back to themes' }).click();
	const rows = own.getByRole('list', { name: 'Your themes' });
	await rows.getByRole('button', { name: 'More for New theme' }).click();
	await own.getByRole('button', { name: 'Delete' }).click();
	await expect(rows).toHaveCount(0);
	await own.getByRole('button', { name: 'Undo' }).click();
	await expect(own.getByRole('list', { name: 'Your themes' }).getByText('New theme')).toBeVisible();
	await own.context().close();
});

test("another device of the same person follows a theme's changes", async ({ browser }) => {
	const own = await person(browser, 'Otto');
	const id = await newTheme(own, 'Standard');
	const { cookies } = await own.context().storageState();
	const other = await (
		await browser.newContext({ storageState: { cookies, origins: [] } })
	).newPage();
	await other.goto('/settings/themes');
	const rows = other.getByRole('list', { name: 'Your themes' });
	await expect(rows.getByText('New theme')).toBeVisible();
	await own.getByLabel('Theme name').fill('Evening');
	await own.getByLabel('Theme name').press('Tab');
	await savedTheme(own, id, (t) => t.name === 'Evening');
	await expect(rows.getByText('Evening')).toBeVisible();
	await other.context().close();
	await own.context().close();
});

test('every built-in theme passes every contrast pair', async ({ browser }) => {
	// Every theme in both schemes, one page load each: longer than most.
	test.slow();
	const names = pairTokens();
	for (const character of ['standard', 'calm', 'playful', 'contrast']) {
		for (const scheme of ['light', 'dark']) {
			const context = await browser.newContext(SIGNED_OUT);
			const page = await context.newPage();
			await page.addInitScript((a) => localStorage.setItem('mila.appearance', JSON.stringify(a)), {
				character,
				scheme
			});
			await page.goto('/dev/ui');
			await expect(page.getByRole('heading', { name: 'Style guide' })).toBeVisible();
			await expect(page.locator('html')).toHaveAttribute('data-theme', character);
			await expect(page.locator('html')).toHaveAttribute('data-scheme', scheme);
			// The accent as the app works it out, not the stylesheet's fallback.
			await expect(page.locator('html')).toHaveAttribute('style', /--accent-l/);
			const raw = await page.evaluate((names) => {
				const probe = document.createElement('span');
				document.documentElement.append(probe);
				const out: Record<string, string> = {};
				for (const n of names) {
					probe.style.color = `var(--${n})`;
					out[n] = getComputedStyle(probe).color;
				}
				probe.remove();
				return out;
			}, names);
			const resolved = new Map<string, Resolved>();
			for (const [n, v] of Object.entries(raw)) {
				const c = parseColor(v);
				if (c) resolved.set(n, c);
			}
			expect(resolved.size, `${character} ${scheme} colors`).toBe(names.length);
			const target = character === 'contrast' ? 'AAA' : 'AA';
			const failures = check(resolved, target).map(
				(f) =>
					`${f.pair.fg} on ${typeof f.bg === 'string' ? f.bg : f.bg.label}: ${f.ratio.toFixed(2)}`
			);
			expect(failures, `${character} ${scheme}`).toEqual([]);
			await context.close();
		}
	}
});

test('every theme from Omarchy passes every contrast pair as drawn', async ({ browser }) => {
	// 22 themes in both schemes: longer than most.
	test.setTimeout(240_000);
	const names = pairTokens();
	const context = await browser.newContext(SIGNED_OUT);
	const page = await context.newPage();
	for (const preset of OMARCHY) {
		for (const scheme of ['light', 'dark']) {
			// In use as this device keeps it, as after picking it.
			await page.addInitScript(
				({ a, p }) => {
					localStorage.setItem('mila.appearance', JSON.stringify(a));
					localStorage.setItem('mila.presetTheme', JSON.stringify(p));
				},
				{ a: { character: 'standard', scheme, customTheme: preset.doc.id }, p: preset }
			);
			await page.goto('/dev/ui');
			await expect(page.getByRole('heading', { name: 'Style guide' })).toBeVisible();
			await expect(page.locator('html')).toHaveAttribute('data-custom-theme', preset.doc.id);
			await expect(page.locator('html')).toHaveAttribute('data-scheme', scheme);
			const raw = await page.evaluate((names) => {
				const probe = document.createElement('span');
				document.documentElement.append(probe);
				const out: Record<string, string> = {};
				for (const n of names) {
					probe.style.color = `var(--${n})`;
					out[n] = getComputedStyle(probe).color;
				}
				probe.remove();
				return out;
			}, names);
			const resolved = new Map<string, Resolved>();
			for (const [n, v] of Object.entries(raw)) {
				const c = parseColor(v);
				if (c) resolved.set(n, c);
			}
			expect(resolved.size, `${preset.slug} ${scheme} colors`).toBe(names.length);
			const failures = check(resolved, 'AA').map(
				(f) =>
					`${f.pair.fg} on ${typeof f.bg === 'string' ? f.bg : f.bg.label}: ${f.ratio.toFixed(2)}`
			);
			expect(failures, `${preset.slug} ${scheme}`).toEqual([]);
		}
	}
	await context.close();
});

test('a theme from Omarchy is picked like a built-in, takes its scheme, paints first and can be copied', async ({
	browser
}) => {
	const own = await person(browser, 'Omar');
	const tokyo = omarchy('tokyo-night');
	await own.goto('/settings/appearance');
	const picker = own.getByRole('group', { name: 'Theme' });
	await expect(picker.getByText('From Omarchy')).toBeVisible();
	await picker.getByText('Tokyo Night', { exact: true }).click();
	// Omarchy has it in dark only, so the scheme follows.
	await expect(own.locator('html')).toHaveAttribute('data-custom-theme', tokyo.doc.id);
	await expect(own.locator('html')).toHaveAttribute('data-scheme', 'dark');
	await expect(own.getByRole('radio', { name: 'Dark', exact: true })).toBeChecked();
	// Omarchy's own page color, exactly.
	const page = '#1a1b26';
	await expect.poll(() => background(own, 'body')).toBe(page);
	// Light still works, from the same hues.
	const schemes = own.getByRole('group', { name: 'Scheme' });
	await schemes.getByText('Light', { exact: true }).click();
	await expect(own.locator('html')).toHaveAttribute('data-scheme', 'light');
	await expect(own.locator('html')).toHaveAttribute('data-custom-theme', tokyo.doc.id);
	await schemes.getByText('Dark', { exact: true }).click();
	await expect.poll(() => background(own, 'body')).toBe(page);

	// The next load paints it before any script runs.
	await own.route(/\/_app\/immutable\/.*\.js$/, (r) => r.abort());
	await own.reload();
	await expect(own.locator('html')).toHaveAttribute('data-custom-theme', tokyo.doc.id);
	expect(await background(own, 'body')).toBe(page);
	await own.unroute(/\/_app\/immutable\/.*\.js$/);

	// Another device, which never kept it, gets it from the preference.
	const { cookies } = await own.context().storageState();
	const other = await (
		await browser.newContext({ storageState: { cookies, origins: [] } })
	).newPage();
	await other.goto('/');
	await expect(other.locator('html')).toHaveAttribute('data-custom-theme', tokyo.doc.id);
	await expect.poll(() => background(other, 'body')).toBe(page);
	await other.context().close();

	// A copy is the way to change one: it is the person's own, under the same name.
	await own.reload();
	await own.goto('/settings/themes');
	await own.getByLabel('Start from').selectOption({ label: 'Tokyo Night' });
	await own.getByRole('button', { name: 'New theme', exact: true }).click();
	await expect(own).toHaveURL(/\/settings\/themes\/[0-9a-f-]{36}$/);
	await expect(own.getByLabel('Theme name')).toHaveValue('Tokyo Night');
	expect(own.url()).not.toContain(tokyo.doc.id);
	await own.context().close();
});

test("reduced motion keeps a custom theme's durations out", async ({ browser }) => {
	const own = await person(browser, 'Rhea');
	const id = await newTheme(own, 'Standard');
	await group(own, 'Motion').click();
	await openRow(own, 'Panels');
	const panels = own.getByLabel('Panels, exactly in ms');
	await panels.fill('900');
	await panels.press('Tab');
	await own.getByRole('button', { name: 'Use this theme' }).click();
	await expect(own.locator('html')).toHaveAttribute('data-custom-theme', id);
	const panel = async () =>
		ms(
			await own.evaluate(() =>
				getComputedStyle(document.documentElement).getPropertyValue('--dur-panel')
			)
		);
	await expect.poll(panel).toBe(900);
	await own.emulateMedia({ reducedMotion: 'reduce' });
	await expect.poll(panel).toBe(150);
	await own.context().close();
});

test('the sample theme the visual tests wear is valid and has no contrast problems', async ({
	browser
}) => {
	const own = await person(browser, 'Sami');
	const put = await apiCall(own, 'PUT', `/me/themes/${SAMPLE.id}`, SAMPLE);
	expect(put.status, JSON.stringify(put.body)).toBe(200);
	await own.goto(`/settings/themes/${SAMPLE.id}`);
	await expect(own.getByLabel('Theme name')).toHaveValue('Harbor');
	await own.getByRole('button', { name: 'Reads at AA, light and dark' }).click();
	await expect(own.getByText('Everything reads at AA, in light and dark.')).toBeVisible();
	await expect(own.getByRole('button', { name: 'Use this theme' })).toBeEnabled();
	await own.context().close();
});

test("under reduced motion the editor reads and scales the theme's own durations", async ({
	browser
}) => {
	const own = await person(browser, 'Mona');
	await own.emulateMedia({ reducedMotion: 'reduce' });
	const id = await newTheme(own, 'Calm');
	await group(own, 'Motion').click();
	await openRow(own, 'Panels');
	// Calm's own, not the 150ms everything fades in under reduced motion.
	await expect(own.getByLabel('Panels, exactly in ms')).toHaveValue('260');
	await expect(own.getByRole('radio', { name: 'As the base' })).toBeChecked();
	await own.getByRole('radio', { name: 'Faster' }).check();
	await expect(own.getByLabel('Panels, exactly in ms')).toHaveValue('160');
	await savedTheme(own, id, (t) => {
		const tokens = t.tokens as Record<string, { ms?: number }>;
		return (
			tokens['dur-micro']?.ms === 90 &&
			tokens['dur-panel']?.ms === 160 &&
			tokens['dur-view']?.ms === 240
		);
	});
	await own.context().close();
});

test("Ctrl+Z in a task opened over the editor is the app's undo, not the editor's", async ({
	browser
}) => {
	const own = await person(browser, 'Tess');
	await own.getByLabel('New task').fill('Panel canary');
	await own.getByLabel('New task').press('Enter');
	const row = own.locator('#main [data-task-id]').filter({ hasText: 'Panel canary' });
	const taskId = await row.getAttribute('data-task-id');
	const id = await newTheme(own, 'Standard');
	const preview = '.theme-preview[data-scheme="light"]';
	await setColor(own, 'Page', '#e0f0ff');
	await own.goto(`/settings/themes/${id}?task=${taskId}`);
	const panel = own.getByRole('complementary', { name: 'Task details' });
	await panel.getByLabel('Priority', { exact: true }).selectOption('high');
	await panel.getByRole('button', { name: 'Close' }).focus();
	await own.keyboard.press('Control+z');
	await expect(panel.getByLabel('Priority', { exact: true })).toHaveValue('none');
	expect(await background(own, preview)).toBe('#e0f0ff');
	await own.context().close();
});

test('a copy of a theme that is deleted meanwhile starts from Standard', async ({ browser }) => {
	const own = await person(browser, 'Cleo');
	await newTheme(own, 'Calm');
	await own.getByRole('link', { name: 'Back to themes' }).click();
	const start = own.getByLabel('Start from');
	await start.selectOption({ label: 'New theme' });
	await own
		.getByRole('list', { name: 'Your themes' })
		.getByRole('button', { name: 'More for New theme' })
		.click();
	await own.getByRole('button', { name: 'Delete' }).click();
	await expect(start).toHaveValue('standard');
	await own.getByRole('button', { name: 'New theme', exact: true }).click();
	await expect(own.getByText('Based on Standard')).toBeVisible();
	await own.context().close();
});

test("a part's reset puts it back in light and dark", async ({ browser }) => {
	const own = await person(browser, 'Rex');
	await newTheme(own, 'Standard');
	const editing = own.getByRole('group', { name: 'Editing' });
	await setColor(own, 'Page', '#e0f0ff');
	await editing.getByText('Dark', { exact: true }).click();
	await setColor(own, 'Page', '#203040');
	await expect(group(own, 'Surfaces')).toContainText('1 changed');
	await editing.getByText('Light', { exact: true }).click();
	await own.getByRole('button', { name: 'Reset Surfaces' }).click();
	await expect(group(own, 'Surfaces')).not.toContainText('changed');
	await editing.getByText('Dark', { exact: true }).click();
	await expect(group(own, 'Surfaces')).not.toContainText('changed');
	await expect(own.getByRole('button', { name: 'Reset Surfaces' })).toBeDisabled();
	await own.context().close();
});

test("the accent's own hue comes back after setting it by hand", async ({ browser }) => {
	const own = await person(browser, 'Hugo');
	await newTheme(own, 'Standard');
	await group(own, 'Accent').click();
	await own.getByRole('slider', { name: 'Accent hue' }).fill('30');
	await own.getByRole('radio', { name: /^Set by hand/ }).check();
	await expect(own.getByRole('slider', { name: 'Accent hue' })).toBeHidden();
	await own.getByRole('radio', { name: /^Worked out from a hue/ }).check();
	await expect(own.getByRole('slider', { name: 'Accent hue' })).toHaveValue('30');
	await own.context().close();
});

test("the Pages colors reach the preview's notes, a heading level at a time", async ({
	browser
}) => {
	const own = await person(browser, 'Paige');
	await newTheme(own, 'Standard');
	const notes = '.theme-preview[data-scheme="light"] .doc-text';
	// The preview shows the part being edited: the notes come with Pages.
	await expect(own.locator(notes)).toHaveCount(0);
	await group(own, 'Pages').click();
	await expect(own.locator(`${notes} h1`).first()).toHaveText('Garden plan');
	const h2Before = await color(own, `${notes} h2`);
	await setColor(own, 'Heading 1', '#9b2c6b');
	await expect.poll(() => color(own, `${notes} h1`)).toBe('#9b2c6b');
	// Only that level: the next one keeps its color.
	expect(await color(own, `${notes} h2`)).toBe(h2Before);
	await setColor(own, 'Table header', '#e8f0e0');
	await expect.poll(() => background(own, `${notes} th`)).toBe('#e8f0e0');
	await axe(own);
});

test('the editor finds a setting by search, by Changed only, and from a click on the preview', async ({
	browser
}) => {
	const own = await person(browser, 'Finn');
	await newTheme(own, 'Standard');

	// A search goes across every part, and says where each setting is.
	const find = own.getByLabel('Find a setting');
	await find.fill('table');
	await expect(own.getByRole('heading', { name: 'Settings matching table' })).toBeVisible();
	const header = own.getByRole('button', { name: /^Table header:/ });
	await expect(header).toContainText('Pages');
	await expect(own.getByRole('button', { name: /^Page:/ })).toHaveCount(0);
	await find.press('Escape');
	await expect(find).toHaveValue('');
	await expect(own.getByRole('heading', { name: 'Surfaces' })).toBeVisible();

	// Changed only: what this theme sets, in the part.
	await setColor(own, 'Raised surface', '#d0e0f0');
	await own.getByText('Changed only', { exact: true }).click();
	await expect(own.getByRole('button', { name: /^Raised surface:/ })).toBeVisible();
	await expect(own.getByRole('button', { name: /^Page:/ })).toHaveCount(0);
	await own.getByText('Changed only', { exact: true }).click();

	// A row opens in place, and Escape closes it, back on the row.
	const page = own.getByRole('button', { name: /^Page:/ });
	await page.click();
	await expect(page).toHaveAttribute('aria-expanded', 'true');
	await own.keyboard.press('Escape');
	await expect(page).toHaveAttribute('aria-expanded', 'false');
	await expect(page).toBeFocused();

	// A click on the preview opens what draws it, in its part.
	await group(own, 'Pages').click();
	const heading = own.locator('.theme-preview[data-scheme="light"] h1');
	const box = (await heading.boundingBox())!;
	await own.mouse.click(box.x + 10, box.y + box.height / 2);
	const h1 = own.getByRole('button', { name: /^Heading 1:/ });
	await expect(h1).toHaveAttribute('aria-expanded', 'true');
	await expect(h1).toBeFocused();
	// Whole app shows every part, the board among them.
	await expect(own.locator('.theme-preview[data-scheme="light"] .board')).toHaveCount(0);
	await own.getByText('Whole app', { exact: true }).click();
	await expect(own.locator('.theme-preview[data-scheme="light"] .board')).toBeVisible();
	await own.context().close();
});
