import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, emailTo, newMember, OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });

const switcher = (page: Page) => page.getByRole('button', { name: /^Switch workspace/ });

test('a mention and an assignment reach the person, live, with links into the task', async ({
	page,
	browser
}) => {
	const {
		context,
		page: dana,
		email,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Dana');

	// Dana looks at her own workspace, so following a link has to switch her.
	await switcher(dana).click();
	await dana.getByRole('button', { name: 'Personal' }).click();
	await expect(switcher(dana)).toContainText('Personal');
	await expect(dana.getByRole('button', { name: 'Notifications', exact: true })).toBeVisible();

	// A link with the workspace switches the owner to it and drops it from the URL.
	await page.goto(`/?ws=${workspace.id}`);
	await expect(switcher(page)).toContainText(workspace.name);
	await expect(page).not.toHaveURL(/ws=/);

	const title = `Repaint the porch ${Date.now()}`;
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await page
		.getByRole('list', { name: 'Tasks' })
		.getByRole('button', { name: title, exact: true })
		.click();
	const detail = page.getByRole('complementary', { name: 'Task details' });

	// Mention Dana from the keyboard.
	const box = detail.getByLabel('New comment');
	await box.click();
	await page.keyboard.type('Hey @da');
	const people = page.getByRole('listbox', { name: 'People to mention' });
	await expect(people.getByRole('option', { name: 'Dana' })).toHaveAttribute(
		'aria-selected',
		'true'
	);
	// Screen readers hear that the list opened and what Enter now does.
	await expect(detail.getByRole('status').filter({ hasText: 'to mention' })).toHaveText(
		/^\d+ (person|people) to mention\. Arrow keys to choose, Enter or Tab to pick/
	);
	await page.keyboard.press('Enter');
	await expect(people).toBeHidden();
	await expect(box).toHaveValue('Hey @Dana ');
	await expect(detail.getByRole('status').filter({ hasText: 'to mention' })).toHaveCount(0);
	await page.keyboard.type('can you start Monday?');
	// Escape closes the list and leaves the task open.
	await page.keyboard.type(' @');
	await expect(people).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(people).toBeHidden();
	await expect(detail).toBeVisible();
	await page.keyboard.press('Backspace');
	await page.keyboard.press('Backspace');
	await detail.getByRole('button', { name: 'Comment', exact: true }).click();
	const comments = detail.getByRole('list', { name: 'Comments' });
	await expect(comments.locator('.mention')).toHaveText('@Dana');
	await expect(comments).toContainText('Hey @Dana can you start Monday?');

	// Dana's bell counts it without a reload.
	const bell = dana.getByRole('button', { name: 'Notifications, 1 unread' });
	await expect(bell).toBeVisible();
	await bell.click();
	const panel = dana.getByRole('dialog', { name: 'Notifications' });
	const item = panel.getByRole('link', { name: new RegExp(`mentioned you in ${title}`) });
	await expect(item).toContainText('Hey @Dana can you start Monday?');
	await expect(item).toContainText('Unread');
	await axe(dana);

	// Following it switches workspace, opens the task and marks it read.
	await item.click();
	await expect(panel).toBeHidden();
	const danaDetail = dana.getByRole('complementary', { name: 'Task details' });
	await expect(danaDetail.getByLabel('Task title')).toHaveValue(title);
	await expect(switcher(dana)).toContainText(workspace.name);
	await expect(dana).not.toHaveURL(/ws=/);
	await expect(dana.getByRole('button', { name: 'Notifications', exact: true })).toBeVisible();

	// The email names who, where, and links back.
	const mail = await emailTo(email, new RegExp(`mentioned you in ${title}`));
	expect(mail).toContain('> Hey @Dana can you start Monday?');
	expect(mail).toMatch(/\/\?task=[\w-]+&ws=[\w-]+/);
	expect(mail).toContain('/settings/notifications');

	// Being assigned notifies too.
	await detail.getByRole('button', { name: 'Edit assignees' }).click();
	await page.getByRole('checkbox', { name: 'Dana', exact: true }).check();
	await page.keyboard.press('Escape');
	await expect(dana.getByRole('button', { name: 'Notifications, 1 unread' })).toBeVisible();
	// On a phone the open task covers the top bar.
	await danaDetail.getByRole('button', { name: 'Close' }).click();
	await dana.getByRole('button', { name: 'Notifications, 1 unread' }).click();
	await expect(panel.getByRole('link', { name: /assigned you to/ })).toBeVisible();
	await panel.getByRole('button', { name: 'Mark all read' }).click();
	await expect(panel.getByText('Unread')).toHaveCount(0);
	await dana.keyboard.press('Escape');
	await expect(dana.getByRole('button', { name: 'Notifications', exact: true })).toBeVisible();

	// One opened from its email is marked read, as a click in the panel
	// marks it, and the link leaves the URL as it was.
	await box.click();
	await page.keyboard.type('Also @da');
	await expect(people.getByRole('option', { name: 'Dana' })).toHaveAttribute(
		'aria-selected',
		'true'
	);
	await page.keyboard.press('Enter');
	await page.keyboard.type('the brushes are in the shed.');
	await detail.getByRole('button', { name: 'Comment', exact: true }).click();
	await expect(dana.getByRole('button', { name: 'Notifications, 1 unread' })).toBeVisible();
	const again = await emailTo(email, new RegExp(`mentioned you in ${title}`), 'the brushes');
	await dana.goto(again.match(/^Open it: (\S+)\r?$/m)![1]);
	await expect(danaDetail.getByLabel('Task title')).toHaveValue(title);
	await expect(dana.getByRole('button', { name: 'Notifications', exact: true })).toBeVisible();
	await expect(dana).not.toHaveURL(/[?&](ws|n)=/);
	await danaDetail.getByRole('button', { name: 'Close' }).click();

	// With no connection, the last list still shows.
	await context.setOffline(true);
	await dana.getByRole('button', { name: 'Notifications', exact: true }).click();
	await expect(panel.getByRole('link')).toHaveCount(3);
	await context.setOffline(false);
	await context.close();
});

test('notification settings save, and the test email arrives', async ({ browser }) => {
	const {
		context,
		page: lee,
		email
	} = await newPerson(
		() => browser.newContext({ ...SIGNED_OUT, timezoneId: 'America/Chicago' }),
		'Lee'
	);
	await lee.goto('/settings/notifications');
	await expect(lee.getByRole('heading', { name: 'What reaches you' })).toBeVisible();
	await expect(lee.getByText('America/Chicago, detected on this device')).toBeVisible();

	// Mentions always show in the app; every kind can go by push.
	await expect(lee.getByRole('checkbox', { name: 'Mentions: in the app' })).toBeDisabled();
	await expect(lee.getByRole('checkbox', { name: 'Mentions: in the app' })).toBeChecked();
	await expect(
		lee.getByRole('checkbox', { name: 'Mentions: in the app' })
	).toHaveAccessibleDescription('Mentions always show in the app.');
	await expect(lee.getByRole('checkbox', { name: 'Comments: push' })).toBeEnabled();
	await expect(lee.getByRole('checkbox', { name: 'Comments: push' })).not.toBeChecked();
	await expect(lee.getByRole('checkbox', { name: 'Mentions: push' })).toBeChecked();

	const commentEmail = lee.getByRole('checkbox', { name: 'Comments: email' });
	await expect(commentEmail).not.toBeChecked();
	await commentEmail.focus();
	await lee.keyboard.press('Space');
	await expect(commentEmail).toBeChecked();
	await expect(lee.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();

	// The hour is for either summary, so it waits for one to be on.
	await expect(lee.getByLabel('Send them at')).toBeDisabled();
	await lee.getByText('Email me a weekly report').click();
	await expect(lee.getByLabel('Email me a weekly report')).toBeChecked();
	await expect(lee.getByLabel('Send them at')).toBeEnabled();
	await lee.getByText('Email me a daily digest').click();
	await expect(lee.getByLabel('Email me a daily digest')).toBeChecked();
	await lee.getByLabel('Send them at').selectOption('18');
	await expect(lee.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();
	await lee.reload();
	await expect(lee.getByRole('checkbox', { name: 'Comments: email' })).toBeChecked();
	await expect(lee.getByLabel('Email me a daily digest')).toBeChecked();
	await expect(lee.getByLabel('Email me a weekly report')).toBeChecked();
	await expect(lee.getByLabel('Send them at')).toHaveValue('18');
	await axe(lee);

	// One save failing among several queued still ends on "Saved" once the
	// later ones, which carry everything, have gone through.
	let failed = false;
	await lee.route('**/api/v1/me/settings/notifications', async (route) => {
		if (route.request().method() !== 'PUT' || failed) return route.continue();
		failed = true;
		await new Promise((r) => setTimeout(r, 500));
		await route.fulfill({
			status: 503,
			json: { error: 'unavailable', message: 'Mila is restarting.' }
		});
	});
	await lee.getByRole('checkbox', { name: 'Comments: push' }).check({ force: true });
	await lee.getByRole('checkbox', { name: 'Mentions: push' }).uncheck({ force: true });
	await lee.getByRole('checkbox', { name: 'Comments: email' }).uncheck({ force: true });
	await expect(lee.getByText('Mila is restarting.')).toBeVisible();
	await expect(lee.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();
	await lee.unroute('**/api/v1/me/settings/notifications');
	await lee.reload();
	await expect(lee.getByRole('checkbox', { name: 'Comments: push' })).toBeChecked();
	await expect(lee.getByRole('checkbox', { name: 'Mentions: push' })).not.toBeChecked();
	await expect(lee.getByRole('checkbox', { name: 'Comments: email' })).not.toBeChecked();

	await lee.getByRole('button', { name: 'Send a test email' }).click();
	await expect(lee.getByText('Test email sent')).toBeVisible();
	expect(await emailTo(email, /^Mila test email$/)).toContain(
		'This is the test email you asked for'
	);
	await context.close();
});

test('unread notifications go when what they are about is removed, and come back with it', async ({
	page,
	browser,
	isMobile
}) => {
	test.skip(isMobile, 'the bell behaves the same on a phone; this checks the count');
	const {
		page: dana,
		email,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Dana');
	const ws = workspace.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const members = (await apiCall(page, 'GET', `/workspaces/${ws}/members`)).body as {
		id?: string;
		userId?: string;
		email: string;
	}[];
	const member = members.find((m) => m.email === email)!;
	const project = await make('projects', { name: 'Porch' });
	const task = await make('tasks', { title: 'Repaint', project_id: project, sort_key: 'a0' });
	await make('task-assignees', { task_id: task, user_id: member.userId ?? member.id });

	const bell = dana.getByRole('button', { name: /^Notifications/ }).first();
	await expect(bell).toHaveAccessibleName(/1 unread/);

	// The bell follows with nothing reloaded.
	const remove = (value: string | null) =>
		apiCall(page, 'PATCH', `/workspaces/${ws}/projects/${project}`, { deleted_at: value });
	await remove(new Date().toISOString());
	await expect(bell).not.toHaveAccessibleName(/unread/);
	await bell.click();
	await expect(dana.getByText('Nothing yet.', { exact: false })).toBeVisible();
	await dana.keyboard.press('Escape');

	await remove(null);
	await expect(bell).toHaveAccessibleName(/1 unread/);

	// As with a workspace, for everyone in it.
	expect((await apiCall(page, 'DELETE', `/workspaces/${ws}`)).status).toBeLessThan(300);
	await expect(bell).not.toHaveAccessibleName(/unread/);
});

test('notifications can be cleared, one at a time or all at once', async ({ page, browser }) => {
	const {
		context,
		page: dana,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Dana');

	// Three, from comments on a task of Dana's.
	const created = await apiCall(dana, 'POST', `/workspaces/${workspace.id}/tasks`, {
		title: `Clear me ${Date.now()}`
	});
	expect(created.status).toBe(201);
	for (const body of ['First note', 'Second note', 'Third note']) {
		const said = await apiCall(page, 'POST', `/workspaces/${workspace.id}/comments`, {
			task_id: created.body.id,
			body
		});
		expect(said.status).toBe(201);
	}

	await dana.getByRole('button', { name: 'Notifications, 3 unread' }).click();
	const panel = dana.getByRole('dialog', { name: 'Notifications' });
	await expect(panel.getByRole('link')).toHaveCount(3);
	await axe(dana);

	// One goes, the count with it, and focus stays in the list. It stays gone.
	await panel.getByRole('link', { name: /First note/ }).hover();
	const cleared = dana.waitForResponse((r) => r.request().method() === 'DELETE');
	await panel
		.getByRole('button', { name: /^Clear:/ })
		.last()
		.click();
	await cleared;
	await expect(panel.getByRole('link')).toHaveCount(2);
	await expect(panel.getByRole('link', { name: /First note/ })).toHaveCount(0);
	await expect(dana.getByRole('button', { name: 'Notifications, 2 unread' })).toBeVisible();
	await expect(panel.getByRole('button', { name: /^Clear:/ }).last()).toBeFocused();
	await dana.reload();
	await dana.getByRole('button', { name: 'Notifications, 2 unread' }).click();
	await expect(panel.getByRole('link')).toHaveCount(2);

	// Clear all asks first, and Keep keeps them.
	await panel.getByRole('button', { name: 'Clear all' }).click();
	await expect(panel.getByRole('button', { name: 'Keep' })).toBeFocused();
	await panel.getByRole('button', { name: 'Keep' }).click();
	await expect(panel.getByRole('link')).toHaveCount(2);
	await expect(panel.getByRole('button', { name: 'Clear all' })).toBeFocused();

	await panel.getByRole('button', { name: 'Clear all' }).click();
	await panel.getByRole('button', { name: 'Clear', exact: true }).click();
	await expect(panel.getByRole('link')).toHaveCount(0);
	await expect(panel.getByText('Nothing yet.')).toBeVisible();
	await expect(dana.getByRole('button', { name: 'Notifications', exact: true })).toBeVisible();
	await dana.reload();
	await expect(dana.getByRole('button', { name: 'Notifications', exact: true })).toBeVisible();
	await context.close();
});
