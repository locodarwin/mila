// Workspace themes: an admin publishes one of their own themes and
// suggests it; a member is asked once, uses it, and follows the admin's
// update; when it is unpublished the member keeps it until they save a
// copy. A theme below the contrast floors cannot be published. Fresh
// people throughout, since the shared owner's themes are other tests'.
import { readFileSync } from 'node:fs';
import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, linkFromEmail, newMember, OWNER, SIGNED_OUT, uniqueEmail } from './helpers';

const SAMPLE = JSON.parse(
	readFileSync(new URL('./visual/fixtures/sample.mila-theme.json', import.meta.url), 'utf8')
);

test.use({ storageState: OWNER });
test.skip(({ isMobile }) => isMobile, 'the phone layout is checked in phone.e2e.ts');

const switcher = (page: Page) => page.getByRole('button', { name: /^Switch workspace/ });

async function switchTo(page: Page, name: string) {
	await switcher(page).click();
	await page.getByRole('button', { name, exact: true }).click();
	await expect(switcher(page)).toContainText(name);
}

/** The theme kept on a device as the workspace theme in use. */
const kept = (page: Page) =>
	page.evaluate(() => JSON.parse(localStorage.getItem('mila.workspaceTheme') ?? 'null'));

test('an admin publishes and suggests a theme; a member uses it, follows it and keeps it', async ({
	page,
	browser
}) => {
	// A long one: CI's slower machines have run out its time near the end.
	test.slow();
	const { page: ada, workspace } = await newMember(
		page,
		() => browser.newContext(SIGNED_OUT),
		'Ada',
		'admin'
	);
	// Dana joins the same workspace as a member.
	const email = uniqueEmail('dana');
	const inv = await apiCall(page, 'POST', `/workspaces/${workspace.id}/invites`, {
		email,
		role: 'member'
	});
	expect(inv.status).toBe(201);
	const danaContext = await browser.newContext(SIGNED_OUT);
	const dana = await danaContext.newPage();
	await dana.goto(await linkFromEmail(email));
	await dana.getByLabel('Your name').fill('Dana');
	await dana.getByRole('button', { name: /^Join/ }).click();
	await expect(dana.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
	for (const p of [ada, dana]) await p.emulateMedia({ colorScheme: 'light' });

	// Ada has two themes of her own: Harbor, and one nobody could read.
	const harbor = { ...SAMPLE, id: crypto.randomUUID() };
	expect((await apiCall(ada, 'PUT', `/me/themes/${harbor.id}`, harbor)).status).toBe(200);
	const pale = {
		mila: 'theme',
		version: 1,
		id: crypto.randomUUID(),
		name: 'Pale experiment',
		base: 'standard',
		// Unreadable in dark only: its swatch, shown in light here, still reads.
		dark: { 'text-primary': { oklch: [0.25, 0, 0] } }
	};
	expect((await apiCall(ada, 'PUT', `/me/themes/${pale.id}`, pale)).status).toBe(200);

	await switchTo(ada, workspace.name);
	await ada.goto(`/settings/w/${workspace.id}/themes`);
	const section = ada.getByRole('region', { name: `Workspace themes in ${workspace.name}` });
	await expect(section.getByText('No workspace themes yet.', { exact: false })).toBeVisible();
	await section.getByRole('button', { name: 'Publish one of my themes' }).click();
	const dialog = ada.getByRole('dialog', { name: `Publish a theme to ${workspace.name}` });
	await expect(dialog.getByRole('radio', { name: /Pale experiment/ })).toBeDisabled();
	await expect(dialog.getByText(/unreadable in dark/)).toBeVisible();
	await dialog.getByRole('radio', { name: /Harbor/ }).check();
	await expect(dialog.getByLabel('Name in the workspace')).toHaveValue('Harbor');
	await dialog.getByLabel('Name in the workspace').fill('Studio harbor');
	await dialog.getByRole('checkbox', { name: /Suggest it to everyone/ }).check();
	await axe(ada);
	await dialog.getByRole('button', { name: 'Publish', exact: true }).click();
	await expect(dialog).toBeHidden();
	const rows = section.getByRole('list', { name: 'Workspace themes' });
	await expect(rows.getByText('Studio harbor')).toBeVisible();
	await expect(rows.getByText('Suggested')).toBeVisible();
	await expect(section.getByText('1 of 10')).toBeVisible();
	await axe(ada);
	// Ada suggested it, so she is not asked herself.
	await expect(ada.getByRole('region', { name: 'Theme suggestion' })).toBeHidden();

	// Dana is asked when she opens the workspace, once.
	await switchTo(dana, workspace.name);
	const asked = dana.getByRole('region', { name: 'Theme suggestion' });
	await expect(asked).toContainText(`${workspace.name} suggests the Studio harbor theme`);
	await axe(dana);
	// A workspace's theme is not hers to fetch from her own themes.
	const missing: string[] = [];
	dana.on('response', (r) => {
		if (r.status() === 404 && r.url().includes('/me/themes/')) missing.push(r.url());
	});
	await asked.getByRole('button', { name: 'Use it' }).click();
	await expect(asked).toBeHidden();
	const id = (await kept(dana)).doc.id as string;
	await expect(dana.locator('html')).toHaveAttribute('data-custom-theme', id);
	await expect
		.poll(async () => (await apiCall(dana, 'GET', '/me')).body.preferences)
		.toMatchObject({ customTheme: id, themeAnswers: [id] });
	await dana.reload();
	await expect(dana.locator('html')).toHaveAttribute('data-custom-theme', id);
	await expect(dana.getByRole('region', { name: 'Theme suggestion' })).toBeHidden();
	// The settings message that follows a change has been and gone.
	await apiCall(dana, 'GET', '/me');
	await dana.waitForTimeout(500);
	expect(missing).toEqual([]);

	// Members use and duplicate; only admins do the rest.
	await dana.goto(`/settings/w/${workspace.id}/themes`);
	const danaRows = dana.getByRole('list', { name: 'Workspace themes' });
	await expect(danaRows.getByRole('button', { name: 'Studio harbor is in use' })).toBeDisabled();
	await expect(danaRows.getByRole('button', { name: 'Duplicate Studio harbor' })).toBeVisible();
	await expect(danaRows.getByRole('button', { name: /Suggest|More/ })).toHaveCount(0);
	await axe(dana);
	// The picker shows it under the workspace's name.
	await dana.goto('/settings/appearance');
	await expect(dana.getByText(`From ${workspace.name}`)).toBeVisible();
	await expect(dana.getByRole('radio', { name: /Studio harbor/ })).toBeChecked();

	// Ada changes her Harbor and updates the workspace's copy from it.
	const changed = {
		...harbor,
		light: { ...harbor.light, 'surface-0': { oklch: [0.95, 0.03, 200] } }
	};
	expect((await apiCall(ada, 'PUT', `/me/themes/${harbor.id}`, changed)).status).toBe(200);
	await ada.reload();
	await expect(section.getByText('1 person using it', { exact: false })).toBeVisible();
	await rows.getByRole('button', { name: 'More for Studio harbor' }).click();
	await ada.getByRole('button', { name: 'Update from Harbor' }).click();
	await expect
		.poll(async () => (await kept(dana))?.doc.light['surface-0'])
		.toEqual({ oklch: [0.95, 0.03, 200] });

	// Unpublished: Dana keeps it on screen, and is told, until she chooses.
	await rows.getByRole('button', { name: 'More for Studio harbor' }).click();
	await ada.getByRole('button', { name: 'Unpublish' }).click();
	await expect(section.getByText('No workspace themes yet.', { exact: false })).toBeVisible();
	const gone = dana.getByRole('region', { name: 'Theme no longer offered' });
	await expect(gone).toContainText(`${workspace.name} no longer offers Studio harbor`);
	await expect(dana.locator('html')).toHaveAttribute('data-custom-theme', id);
	await gone.getByRole('button', { name: 'Save a copy to my themes' }).click();
	await expect(gone).toBeHidden();
	await expect
		.poll(async () => {
			const me = (await apiCall(dana, 'GET', '/me')).body.preferences.customTheme;
			const mine = (await apiCall(dana, 'GET', '/me/themes')).body.themes as { id: string }[];
			return me !== id && mine.some((t) => t.id === me);
		})
		.toBe(true);
	await expect(dana.locator('html')).not.toHaveAttribute('data-custom-theme', id);

	await danaContext.close();
	await ada.context().close();
});

test('members cannot publish, and a workspace offers ten themes at most', async ({
	page,
	browser
}) => {
	const { page: mel, workspace } = await newMember(
		page,
		() => browser.newContext(SIGNED_OUT),
		'Mel'
	);
	const body = { ...SAMPLE, id: crypto.randomUUID() };
	const made = await apiCall(mel, 'POST', `/workspaces/${workspace.id}/workspace-themes`, {
		name: 'Mine',
		body
	});
	expect(made.status).toBe(403);
	await switchTo(mel, workspace.name);
	await mel.goto(`/settings/w/${workspace.id}/themes`);
	await expect(
		mel.getByRole('region', { name: `Workspace themes in ${workspace.name}` })
	).toBeVisible();
	await expect(mel.getByRole('button', { name: 'Publish one of my themes' })).toHaveCount(0);

	// The owner fills the workspace through the API; the eleventh is refused.
	for (let i = 0; i < 10; i++) {
		const res = await apiCall(page, 'POST', `/workspaces/${workspace.id}/workspace-themes`, {
			name: `Theme ${i}`,
			body: { ...SAMPLE, id: crypto.randomUUID() }
		});
		expect(res.status, JSON.stringify(res.body)).toBe(201);
	}
	const eleventh = await apiCall(page, 'POST', `/workspaces/${workspace.id}/workspace-themes`, {
		name: 'One more',
		body: { ...SAMPLE, id: crypto.randomUUID() }
	});
	expect(eleventh.status).toBe(400);
	expect(eleventh.body.message).toContain('10 themes');
	await expect(
		mel.getByRole('list', { name: 'Workspace themes' }).getByRole('listitem')
	).toHaveCount(10);
	await mel.context().close();
});
