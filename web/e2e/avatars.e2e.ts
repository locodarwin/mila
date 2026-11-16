import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, newMember, OWNER, pngBuffer, SIGNED_OUT, newPerson } from './helpers';

// People's pictures. Each test signs up someone new for the picture it
// changes, so no other test sees it.
test.use({ storageState: OWNER });

const PICTURE = /^\/api\/v1\/avatars\/[0-9a-f]{32}$/;

/** The picture inside an avatar, once it has loaded. */
async function shows(avatar: ReturnType<Page['locator']>) {
	const img = avatar.locator('img');
	await expect(img).toHaveAttribute('src', PICTURE);
	await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBe(256);
}

/** Chooses a picture through the Upload picture button and returns the crop dialog. */
async function choose(page: Page) {
	await page.getByLabel('Upload picture').setInputFiles({
		name: 'me.png',
		mimeType: 'image/png',
		buffer: pngBuffer(400, 300)
	});
	const dialog = page.getByRole('dialog', { name: 'Crop your picture' });
	await expect(dialog).toBeVisible();
	return dialog;
}

/** Hands the page a picture as a drop on the section, or a paste. */
async function give(page: Page, how: 'drop' | 'paste') {
	await page.evaluate(
		([b64, how]) => {
			const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
			const dt = new DataTransfer();
			dt.items.add(new File([bytes], 'given.png', { type: 'image/png' }));
			if (how === 'paste') {
				document.body.dispatchEvent(
					new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true })
				);
				return;
			}
			const zone = document.querySelector('[role=group][aria-label="Your picture"]')!;
			for (const type of ['dragenter', 'dragover', 'drop'])
				zone.dispatchEvent(
					new DragEvent(type, { dataTransfer: dt, bubbles: true, cancelable: true })
				);
		},
		[pngBuffer(300, 300).toString('base64'), how] as const
	);
}

test('a picture is cropped with the keyboard, and shows to others at once', async ({
	page,
	browser
}) => {
	const {
		context,
		page: pia,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Pia');
	const ws = workspace.id;
	const piaId = (await apiCall(pia, 'GET', '/me')).body.id as string;

	// A task assigned to Pia, which she commented on.
	const title = `Prune the roses ${Date.now()}`;
	const task = await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, { title, sort_key: 'a0' });
	expect(task.status).toBe(201);
	expect(
		(
			await apiCall(page, 'POST', `/workspaces/${ws}/task-assignees`, {
				task_id: task.body.id,
				user_id: piaId
			})
		).status
	).toBe(201);
	expect(
		(
			await apiCall(pia, 'POST', `/workspaces/${ws}/comments`, {
				task_id: task.body.id,
				body: 'On it.'
			})
		).status
	).toBe(201);

	// The owner has the task open.
	await page.goto(`/?task=${task.body.id}&ws=${ws}`);
	const detail = page.getByRole('complementary', { name: 'Task details' });
	const comments = detail.getByRole('list', { name: 'Comments' });
	await expect(comments).toContainText('On it.');
	await expect(comments.locator('img')).toHaveCount(0);

	// Pia chooses a picture and crops it from the keyboard.
	await pia.goto('/settings/profile');
	await expect(
		pia.getByRole('img', { name: 'No picture; your initials show instead' })
	).toBeVisible();
	const dialog = await choose(pia);
	const stage = dialog.getByRole('application', { name: 'Picture position' });
	await expect(stage).toBeFocused();
	await pia.keyboard.press('+');
	await expect(dialog.getByRole('status')).toContainText('Zoom');
	await pia.keyboard.press('ArrowLeft');
	await expect(dialog.getByRole('status')).toContainText('Moved left');
	await dialog.getByRole('button', { name: 'Save' }).click();
	await expect(dialog).toBeHidden();
	await expect(pia.getByText('Picture saved').last()).toBeVisible();
	await shows(pia.getByRole('img', { name: 'Your picture' }));
	// Settings with a picture set: Change and Remove picture beside it.
	await expect(pia.getByRole('button', { name: 'Remove picture' })).toBeEnabled();
	await axe(pia);

	// The owner sees it without a reload: in the comments, and on the row.
	await shows(comments);
	await detail.getByRole('button', { name: 'Close' }).click();
	await shows(page.getByRole('img', { name: `Assigned to Pia` }).first());
	await context.close();
});

test('a picture can be dropped or pasted, falls back to initials, and is removed', async ({
	browser
}) => {
	const { context, page: quin } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Quin');
	await quin.goto('/settings/profile');
	await expect(quin.getByText('Choose a picture, or drop or paste one here.')).toBeVisible();
	const dialog = quin.getByRole('dialog', { name: 'Crop your picture' });

	// Dropped on the section, then canceled.
	await give(quin, 'drop');
	await expect(dialog).toBeVisible();
	await quin.keyboard.press('Escape');
	await expect(dialog).toBeHidden();
	expect((await apiCall(quin, 'GET', '/me')).body.avatar).toBeNull();

	// Pasted, then saved.
	await give(quin, 'paste');
	await expect(dialog).toBeVisible();
	await dialog.getByRole('button', { name: 'Save' }).click();
	await expect(dialog).toBeHidden();
	const mine = quin.getByRole('img', { name: 'Your picture' });
	await shows(mine);
	expect((await apiCall(quin, 'GET', '/me')).body.avatar).toMatch(PICTURE);

	// A picture that cannot be fetched leaves the initials.
	await quin.route('**/api/v1/avatars/**', (r) => r.abort());
	await quin.reload();
	await expect(mine).toHaveText('Q');
	await expect(mine.locator('img')).toHaveCount(0);
	await quin.unroute('**/api/v1/avatars/**');
	// Back online, it is tried again without a reload.
	await quin.evaluate(() => window.dispatchEvent(new Event('online')));
	await shows(mine);

	// With no connection, nothing can be changed, and the page says why.
	await context.setOffline(true);
	await quin.evaluate(() => window.dispatchEvent(new Event('offline')));
	await expect(quin.getByText('Changing your picture needs a connection.')).toBeVisible();
	await expect(quin.getByLabel('Upload picture')).toBeDisabled();
	await expect(quin.getByRole('button', { name: 'Remove picture' })).toBeDisabled();
	// A picture dropped meanwhile neither opens in the browser nor the cropper.
	await give(quin, 'drop');
	await expect(
		quin.getByRole('alert').filter({ hasText: 'Changing your picture needs a connection.' })
	).toBeVisible();
	await expect(dialog).toBeHidden();
	await expect(quin).toHaveURL(/\/settings\/profile$/);
	await context.setOffline(false);
	await quin.evaluate(() => window.dispatchEvent(new Event('online')));
	await expect(quin.getByRole('button', { name: 'Remove picture' })).toBeEnabled();

	// Removed, the initials show again.
	await quin.getByRole('button', { name: 'Remove picture' }).click();
	await expect(quin.getByText('Picture removed')).toBeVisible();
	await expect(
		quin.getByRole('img', { name: 'No picture; your initials show instead' })
	).toHaveText('Q');
	await expect(quin.getByRole('button', { name: 'Remove picture' })).toHaveCount(0);
	// Focus does not fall to the page: it is on Upload picture, beside where the button was.
	await expect(quin.getByLabel('Upload picture')).toBeFocused();
	expect((await apiCall(quin, 'GET', '/me')).body.avatar).toBeNull();
	await context.close();
});

test("an admin removes someone's picture", async ({ page, browser }) => {
	// A name of its own: the desktop and phone runs of this test share the list.
	const name = `Ros${Math.floor(Math.random() * 1e6)}`;
	const { context, page: ros } = await newMember(page, () => browser.newContext(SIGNED_OUT), name);
	const status = await ros.evaluate(async (b64) => {
		const body = new FormData();
		const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
		body.append('file', new Blob([bytes], { type: 'image/png' }), 'ros.png');
		return (await fetch('/api/v1/me/avatar', { method: 'PUT', body })).status;
	}, pngBuffer(64, 64).toString('base64'));
	expect(status).toBe(200);
	const rosId = (await apiCall(ros, 'GET', '/me')).body.id as string;

	await page.goto('/settings/admin');
	const remove = page
		.getByRole('list', { name: 'Accounts' })
		.getByRole('button', { name: `Remove picture: ${name}` });
	await expect(remove).toHaveCount(1);
	// The accounts list with a picture set, and its Remove picture button.
	await axe(page);
	await remove.click();
	await expect(page.getByText(`Removed ${name}'s picture`)).toBeVisible();
	await expect(remove).toHaveCount(0);
	// Focus moves on to the row's next control rather than falling to the page.
	await expect(page.getByRole('button', { name: `Sign out everywhere: ${name}` })).toBeFocused();
	const me = await apiCall(ros, 'GET', '/me');
	expect(me.body.id).toBe(rosId);
	expect(me.body.avatar).toBeNull();
	await context.close();
});

test('someone made a guest keeps names and pictures, and no addresses', async ({
	page,
	browser
}) => {
	const {
		context,
		page: mel,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Mel');
	const ws = workspace.id;
	const melId = (await apiCall(mel, 'GET', '/me')).body.id as string;
	// The members the device keeps for this workspace, or null.
	const kept = () =>
		mel.evaluate(
			(ws) => JSON.parse(localStorage.getItem('mila.people') ?? 'null')?.members?.[ws] ?? null,
			ws
		);
	// Connected, so the change reaches the open app (a missed one is
	// caught up by the next start).
	const connected = new Promise<void>((done) =>
		mel.on('websocket', (socket) =>
			socket.on('framereceived', (f) => {
				if (String(f.payload).includes('"hello"')) done();
			})
		)
	);
	await mel.goto(`/?ws=${ws}`);
	await connected;
	await expect.poll(async () => (await kept())?.length ?? 0).toBeGreaterThan(1);

	const addresses = async () =>
		((await kept()) ?? []).filter((m: { email: string }) => m.email).length;
	await expect.poll(addresses).toBeGreaterThan(0);

	// Made a guest, Mel's open app drops the addresses it kept, and a reload does
	// not bring them back, but the names stay for comments and mentions.
	const changed = await apiCall(page, 'PATCH', `/workspaces/${ws}/members/${melId}`, {
		role: 'guest'
	});
	expect(changed.status).toBe(204);
	await expect.poll(addresses).toBe(0);
	await mel.reload();
	await expect(mel.getByRole('button', { name: /^Switch workspace/ })).toBeVisible();
	await expect.poll(async () => (await kept())?.length ?? 0).toBeGreaterThan(1);
	expect(await addresses()).toBe(0);
	await context.close();
});
