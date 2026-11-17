import { expect, test, type Page } from '@playwright/test';
import { apiCall, newPerson, OWNER, SIGNED_OUT } from './helpers';

test.use({ storageState: OWNER });

// Settings in three sections, with a workspace picker in the Workspace
// heading that chooses what the workspace pages are about. On a phone
// the rail, and so the picker, is the list's alone.

const rail = (page: Page) => page.getByRole('navigation', { name: 'Settings' });
const switcher = (page: Page) => page.getByRole('button', { name: /^Switch workspace/ });

test('the picker chooses the workspace Settings is about, and leaves the app where it was', async ({
	browser,
	isMobile
}) => {
	const { page, workspace: team } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Pat');
	const all = (await apiCall(page, 'GET', '/workspaces')).body as {
		id: string;
		name: string;
		isPersonal: boolean;
	}[];
	const personal = all.find((w) => w.isPersonal)!;

	// An old address, naming its workspace, goes to that workspace's page.
	await page.goto(`/settings/labels?ws=${team.id}`);
	await expect(page).toHaveURL(new RegExp(`/settings/w/${team.id}/labels$`));
	await expect(page.getByRole('heading', { name: `Labels in ${team.name}` })).toBeVisible();
	const picker = page.getByLabel('Workspace', { exact: true });
	if (isMobile) {
		// The page names its workspace under Back; the list has the picker.
		await expect(page.getByText(`Workspace: ${team.name}`)).toBeVisible();
		await page.locator('.content').getByRole('link', { name: 'Settings', exact: true }).click();
	}
	await expect(picker).toHaveValue(team.id);
	await expect(
		rail(page).getByRole('list', { name: `For everyone in ${team.name}` })
	).toBeVisible();
	await expect(rail(page).getByRole('link', { name: 'People' })).toBeVisible();

	// Picking another keeps to the same page, there (on a phone, the list's
	// pages are the new workspace's).
	await picker.selectOption(personal.id);
	if (isMobile) await rail(page).getByRole('link', { name: 'Labels and fields' }).click();
	await expect(page).toHaveURL(new RegExp(`/settings/w/${personal.id}/labels$`));
	await expect(page.getByRole('heading', { name: `Labels in ${personal.name}` })).toBeVisible();
	if (!isMobile) {
		// A personal workspace has nobody to list.
		await expect(rail(page).getByRole('link', { name: 'People' })).toHaveCount(0);
		await expect(rail(page).getByRole('link', { name: 'Profile' })).toBeVisible();
	}

	// The app is still in the workspace it was in.
	await page.goto('/');
	if (!isMobile) await expect(switcher(page)).toContainText(team.name);

	// Fields lead to their half of the page.
	await page.goto('/settings/fields');
	await expect(page).toHaveURL(new RegExp(`/settings/w/${team.id}/labels#fields$`));

	// A workspace this person is not in says so.
	await page.goto('/settings/w/00000000-0000-4000-8000-000000000000/general');
	await expect(page.getByRole('heading', { name: 'Not one of your workspaces' })).toBeVisible();
	await page.context().close();
});

test('on a phone Settings opens on its list, and each page goes back to it', async ({
	page,
	isMobile
}) => {
	test.skip(!isMobile, 'a phone layout');
	await page.goto('/settings');
	const list = rail(page);
	await expect(list.getByRole('link', { name: 'Profile' })).toBeVisible();
	await expect(page.getByLabel('Name', { exact: true })).toHaveCount(0);
	await list.getByRole('link', { name: 'Profile' }).click();
	await expect(page).toHaveURL(/\/settings\/profile$/);
	await expect(page.getByLabel('Name', { exact: true })).toBeVisible();
	await expect(rail(page)).toHaveCount(0);
	await page.locator('.content').getByRole('link', { name: 'Settings', exact: true }).click();
	await expect(page).toHaveURL(/\/settings$/);
	await expect(rail(page).getByRole('link', { name: 'Notifications' })).toBeVisible();
});

test('on a wide screen Settings opens on Profile', async ({ page, isMobile }) => {
	test.skip(isMobile, 'a wide layout');
	await page.goto('/settings');
	await expect(page).toHaveURL(/\/settings\/profile$/);
	await expect(rail(page).getByRole('link', { name: 'Profile' })).toHaveAttribute(
		'aria-current',
		'page'
	);
});

test('on a big screen the room beside the list grows, and paired sections sit side by side', async ({
	page,
	isMobile
}) => {
	test.skip(isMobile, 'a phone shows the list and a page apart');
	const heading = (name: string) => page.getByRole('heading', { name, exact: true });
	const box = async (name: string) => (await heading(name).boundingBox())!;
	const gap = async () => {
		const list = (await rail(page).boundingBox())!;
		return (await box('Dates and times')).x - (list.x + list.width);
	};

	await page.setViewportSize({ width: 1920, height: 1080 });
	await page.goto('/settings/preferences');
	await expect(heading('Dates and times')).toBeVisible();
	// Two sections a row: Dates and times beside Lists, Task panel beside Sidebar.
	expect(Math.abs((await box('Dates and times')).y - (await box('Lists')).y)).toBeLessThan(2);
	expect((await box('Lists')).x).toBeGreaterThan((await box('Dates and times')).x + 300);
	expect(Math.abs((await box('Task panel')).y - (await box('Sidebar')).y)).toBeLessThan(2);
	const wide = await gap();

	// A laptop: one column, and less room beside the list.
	await page.setViewportSize({ width: 1280, height: 800 });
	await expect
		.poll(async () => (await box('Lists')).y)
		.toBeGreaterThan((await box('Dates and times')).y + 20);
	expect((await box('Lists')).x).toBe((await box('Dates and times')).x);
	expect(await gap()).toBeLessThan(wide);
	expect(await gap()).toBeGreaterThanOrEqual(40);
});
