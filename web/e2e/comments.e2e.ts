import { expect, test, type Browser, type Page } from '@playwright/test';
import { apiCall, linkFromEmail, newMember, OWNER, SIGNED_OUT } from './helpers';
import { axe } from './axe';

// Comments: who can be named and mentioned, and what a comment can link to.
test.use({ storageState: OWNER });

const panel = (page: Page) => page.getByRole('complementary', { name: 'Task details' });
const box = (page: Page) => panel(page).getByLabel('New comment');
const send = (page: Page) => panel(page).getByRole('button', { name: 'Comment', exact: true });
const list = (page: Page, name: string) => panel(page).getByRole('listbox', { name });

/**
 * A shared workspace with a member, a guest given the Garden project, and a
 * guest given only another project, all opening the same task or a task of theirs.
 */
async function team(page: Page, browser: Browser) {
	const open = () => browser.newContext(SIGNED_OUT);
	const { page: gus, workspace } = await newMember(page, open, 'Gus', 'guest');
	const ws = workspace.id;
	const join = async (name: string, role: string) => {
		const email = `${name.toLowerCase()}-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`;
		const inv = await apiCall(page, 'POST', `/workspaces/${ws}/invites`, { email, role });
		expect(inv.status).toBe(201);
		const context = await open();
		const own = await context.newPage();
		await own.goto(await linkFromEmail(email));
		await own.getByLabel('Your name').fill(name);
		await own.getByRole('button', { name: /^Join/ }).click();
		// A guest has no inbox, and arrives at Projects.
		const arrives = role === 'guest' ? 'Projects' : 'Inbox';
		await expect(own.getByRole('heading', { level: 1, name: arrives, exact: true })).toBeVisible();
		return own;
	};
	const mo = await join('Mo', 'member');
	const lou = await join('Lou', 'guest');
	const idOf = async (p: Page) => (await apiCall(p, 'GET', '/me')).body.id as string;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Garden' });
	const other = await make('projects', { name: 'Elsewhere' });
	const status = await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	const task = await make('tasks', { project_id: project, status_id: status, title: 'Beds' });
	const second = await make('tasks', {
		project_id: project,
		status_id: status,
		title: 'Paint the shed'
	});
	const hidden = await make('tasks', { project_id: other, title: 'Secret plans' });
	const shared = async (p: string, who: Page) =>
		expect(
			(await apiCall(page, 'PUT', `/workspaces/${ws}/projects/${p}/guests/${await idOf(who)}`))
				.status
		).toBeLessThan(300);
	await shared(project, gus);
	await shared(other, lou);
	return {
		ws,
		project,
		task,
		second,
		hidden,
		gus,
		mo,
		lou,
		url: `/projects/${project}?ws=${ws}&task=${task}`
	};
}

test('a guest sees who commented, and can mention who can reach the task', async ({
	page,
	browser
}) => {
	const { url, gus, mo } = await team(page, browser);
	const ownerName = (await apiCall(page, 'GET', '/me')).body.name as string;

	await mo.goto(url);
	await box(mo).fill('Looks good to me');
	await send(mo).click();

	await gus.goto(url);
	const comments = panel(gus).getByRole('list', { name: 'Comments' });
	await expect(comments.getByText('Looks good to me')).toBeVisible();
	await expect(comments.getByText('Mo', { exact: true })).toBeVisible();
	await expect(comments.getByText('Someone')).toHaveCount(0);

	// The guest's list holds the members, and not a guest of another project.
	await box(gus).click();
	await box(gus).pressSequentially('@');
	const people = list(gus, 'People to mention');
	await expect(people.getByRole('option', { name: 'Mo' })).toBeVisible();
	await expect(people.getByRole('option', { name: ownerName })).toBeVisible();
	await expect(people.getByRole('option', { name: 'Lou' })).toHaveCount(0);
	await expect(people.getByRole('option', { name: 'Gus' })).toHaveCount(0);
	await axe(gus);

	// A name nobody with access has says so, rather than looking like it worked.
	await box(gus).pressSequentially('Lou');
	await expect(
		panel(gus).locator('p', { hasText: 'Nobody with access to this has that name.' })
	).toBeVisible();

	// A guest's own mention reaches the member.
	await box(gus).fill('');
	await box(gus).pressSequentially('@Mo');
	await people.getByRole('option', { name: 'Mo' }).click();
	await box(gus).pressSequentially('what do you think?');
	await send(gus).click();
	await expect(mo.getByRole('button', { name: /^Notifications, \d+ unread/ })).toBeVisible();
});

test('a member can mention a guest the project is shared with, who is told', async ({
	page,
	browser
}) => {
	const { url, gus, mo } = await team(page, browser);
	await mo.goto(url);
	await box(mo).click();
	await box(mo).pressSequentially('@');
	const people = list(mo, 'People to mention');
	await expect(people.getByRole('option', { name: 'Gus' })).toBeVisible();
	await expect(people.getByRole('option', { name: 'Lou' })).toHaveCount(0);

	await box(mo).pressSequentially('Gu');
	await people.getByRole('option', { name: 'Gus' }).click();
	await box(mo).pressSequentially('can you check this?');
	await send(mo).click();

	const bell = gus.getByRole('button', { name: 'Notifications, 1 unread' });
	await expect(bell).toBeVisible();
	await bell.click();
	await expect(gus.getByRole('dialog', { name: 'Notifications' })).toContainText(
		'Mo mentioned you in Beds'
	);
});

test('a guest gets no addresses', async ({ page, browser }) => {
	const { ws, gus } = await team(page, browser);
	const people = await apiCall(gus, 'GET', `/workspaces/${ws}/people`);
	expect(people.status).toBe(200);
	expect(JSON.stringify(people.body)).not.toContain('@');
	expect((await apiCall(gus, 'GET', `/workspaces/${ws}/members`)).status).toBe(403);
});

test('a pasted address becomes a link, and punctuation stays out of it', async ({
	page,
	browser
}) => {
	const { url, mo } = await team(page, browser);
	await mo.goto(url);
	await box(mo).fill('See https://example.com/a?b=1, or (http://example.org/x).');
	await send(mo).click();
	const comments = panel(mo).getByRole('list', { name: 'Comments' });
	const first = comments.getByRole('link', { name: 'https://example.com/a?b=1' });
	await expect(first).toHaveAttribute('href', 'https://example.com/a?b=1');
	await expect(first).toHaveAttribute('target', '_blank');
	await expect(first).toHaveAttribute('rel', /noopener/);
	await expect(comments.getByRole('link', { name: 'http://example.org/x' })).toHaveAttribute(
		'href',
		'http://example.org/x'
	);
	// Anything but a web address stays as words.
	await box(mo).fill('Not a link: javascript:alert(1) or ftp://example.com');
	await send(mo).click();
	await expect(comments.getByText('javascript:alert(1)')).toBeVisible();
	await expect(comments.getByRole('link', { name: /javascript|ftp/ })).toHaveCount(0);
});

test('/task refers to a task, follows its title, and opens it', async ({ page, browser }) => {
	const { ws, url, second, mo } = await team(page, browser);
	await mo.goto(url);
	await box(mo).click();
	await box(mo).pressSequentially('/');
	const commands = list(mo, 'Commands');
	await expect(commands.getByRole('option')).toHaveCount(2);
	await box(mo).press('Enter');
	await expect(box(mo)).toHaveValue('/task ');
	await box(mo).pressSequentially('Paint');
	await list(mo, 'Tasks')
		.getByRole('option', { name: /Paint the shed/ })
		.click();
	await expect(box(mo)).toHaveValue('[[Paint the shed]] ');
	await box(mo).pressSequentially('comes first');
	await send(mo).click();

	const comments = panel(mo).getByRole('list', { name: 'Comments' });
	const link = comments.getByRole('link', { name: 'Paint the shed' });
	await expect(link).toBeVisible();

	// Stored as a plain link, and the box shows it the same way when edited.
	await comments.getByRole('button', { name: 'Edit' }).click();
	await expect(panel(mo).getByLabel('Edit comment')).toHaveValue('[[Paint the shed]] comes first');
	await panel(mo).getByRole('button', { name: 'Cancel' }).click();

	// A renamed task shows its new title.
	const renamed = await apiCall(page, 'PATCH', `/workspaces/${ws}/tasks/${second}`, {
		title: 'Paint the barn'
	});
	expect(renamed.status).toBeLessThan(300);
	await expect(comments.getByRole('link', { name: 'Paint the barn' })).toBeVisible();

	await comments.getByRole('link', { name: 'Paint the barn' }).click();
	await expect(panel(mo).getByLabel('Task title')).toHaveValue('Paint the barn');
	await expect(mo).toHaveURL(new RegExp(`task=${second}`));
});

test('a reference someone cannot open does not show its name', async ({ page, browser }) => {
	const { ws, url, task, hidden, gus, mo } = await team(page, browser);
	const made = await apiCall(mo, 'POST', `/workspaces/${ws}/comments`, {
		task_id: task,
		body: `Waiting on [Secret plans](/?task=${hidden})`
	});
	expect(made.status).toBeLessThan(300);
	await gus.goto(url);
	const comments = panel(gus).getByRole('list', { name: 'Comments' });
	await expect(comments.getByText('Waiting on')).toBeVisible();
	await expect(comments).toContainText('a task');
	await expect(comments).not.toContainText('Secret plans');
});

test('/wiki refers to a page or a section of one, and goes there', async ({ page, browser }) => {
	const { ws, project, url, mo } = await team(page, browser);
	const filler = Array.from({ length: 60 }, (_, i) => `Line ${i}`).join('\n\n');
	// Opening the project's wiki makes its main page, which a page hangs from.
	await mo.goto(`/projects/${project}?ws=${ws}&tab=wiki`);
	let main = '';
	await expect
		.poll(async () => {
			const pages = await apiCall(mo, 'GET', `/workspaces/${ws}/pages?project_id=${project}`);
			main = pages.body?.items?.[0]?.id ?? pages.body?.[0]?.id ?? '';
			return main;
		})
		.not.toBe('');
	const made = await apiCall(mo, 'POST', `/workspaces/${ws}/pages`, {
		project_id: project,
		parent_id: main,
		title: 'Planting guide',
		body: `# Soil\n\n${filler}\n\n## When to water\n\nMornings.`
	});
	expect(made.status, JSON.stringify(made.body)).toBeLessThan(300);

	await mo.goto(url);
	await box(mo).click();
	await box(mo).pressSequentially('/wiki Plant');
	await list(mo, 'Wiki pages')
		.getByRole('option', { name: /Planting guide/ })
		.click();
	const sections = list(mo, 'Sections');
	await expect(sections.getByRole('option')).toHaveText([
		'The whole page',
		'Soil',
		'When to water'
	]);
	await sections.getByRole('option', { name: 'When to water' }).click();
	await expect(box(mo)).toHaveValue('[[Planting guide › When to water]] ');
	await send(mo).click();

	const link = panel(mo)
		.getByRole('list', { name: 'Comments' })
		.getByRole('link', { name: 'Planting guide › When to water' });
	await expect(link).toHaveAttribute('href', /#when-to-water$/);
	await link.click();
	await expect(mo).toHaveURL(/#when-to-water$/);
	await expect(mo.getByRole('heading', { name: 'When to water' })).toBeInViewport();
});

test('the assignee list offers who can reach the task', async ({ page, browser }) => {
	const { url, mo } = await team(page, browser);
	await mo.goto(url);
	await panel(mo).getByRole('button', { name: 'Edit assignees' }).click();
	const menu = mo.locator('.menu:popover-open');
	await expect(menu.getByRole('checkbox', { name: 'Gus' })).toBeVisible();
	await expect(menu.getByRole('checkbox', { name: 'Mo' })).toBeVisible();
	await expect(menu.getByRole('checkbox', { name: 'Lou' })).toHaveCount(0);
});

test('an admin can delete what someone else wrote, and take that back', async ({
	page,
	browser
}) => {
	const { url, mo, gus } = await team(page, browser);
	await gus.goto(url);
	await box(gus).fill('Delete me if you must');
	await send(gus).click();
	const comment = (p: Page) =>
		panel(p).getByRole('list', { name: 'Comments' }).getByRole('listitem');

	// A member has no such button on a guest's comment.
	await mo.goto(url);
	await expect(comment(mo)).toContainText('Delete me if you must');
	await expect(comment(mo).getByRole('button', { name: 'Delete' })).toHaveCount(0);

	await page.goto(url);
	await comment(page).getByRole('button', { name: 'Delete' }).click();
	await expect(comment(page)).toHaveCount(0);
	await page.getByRole('button', { name: 'Undo' }).click();
	await expect(comment(page)).toContainText('Delete me if you must');
});

test('a shared inbox says that it is shared', async ({ page, browser }) => {
	const { ws, mo } = await team(page, browser);
	await mo.goto(`/?ws=${ws}`);
	await expect(mo.getByRole('heading', { name: 'The inbox is clear' })).toBeVisible();
	await expect(mo.getByText('Everyone in this workspace shares it.')).toBeVisible();
});
