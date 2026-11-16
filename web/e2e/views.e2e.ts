import { expect, test, type Page } from '@playwright/test';
import { axe } from './axe';
import { apiCall, newMember, OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });

async function newProject(page: Page, name: string) {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	return page.url();
}

async function addTask(page: Page, title: string) {
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await expect(row(page, title)).toBeVisible();
}

const row = (page: Page, title: string) =>
	page.getByRole('list', { name: 'Tasks' }).getByRole('listitem').filter({ hasText: title });

const unique = (word: string) => `${word}${Date.now().toString(36)}`;

function ymd(d: Date) {
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

test.describe('the keyboard', () => {
	test.skip(({ isMobile }) => isMobile, 'phones have no keyboard shortcuts');

	test('quick add understands a date, a project and a priority', async ({ page }) => {
		const project = unique('Garden');
		const url = await newProject(page, project);
		await page.goto('/');
		await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
		await page.locator('body').press('q');
		const input = page.getByLabel('Quick add', { exact: true });
		await expect(input).toBeFocused();
		await input.fill(`Water the roses tomorrow #${project} !high`);
		await input.press('Enter');
		await expect(page.getByText(`Added "Water the roses" to ${project}`)).toBeVisible();

		await page.goto(url);
		const roses = row(page, 'Water the roses');
		await expect(roses.getByRole('img', { name: 'High priority' })).toBeVisible();
		await roses.getByRole('button', { name: 'Water the roses', exact: true }).click();
		const tomorrow = new Date();
		tomorrow.setDate(tomorrow.getDate() + 1);
		await expect(page.getByLabel('Due date')).toHaveValue(ymd(tomorrow));
	});

	test('quick add in a saved view of one project adds to that project', async ({ page }) => {
		const project = unique('Bulbs');
		await newProject(page, project);
		await page.getByRole('button', { name: 'Save as a view' }).click();
		await page.getByLabel('View name').fill(unique('Planting'));
		await page.getByLabel('View name').press('Enter');
		await expect(page).toHaveURL(/\/views\//);
		await page.locator('body').press('q');
		const dialog = page.getByRole('dialog', { name: 'Add a task' });
		await expect(dialog.getByText(`to add to ${project}`)).toBeVisible();
		await page.getByLabel('Quick add', { exact: true }).fill('Plant the tulips');
		await page.getByLabel('Quick add', { exact: true }).press('Enter');
		await expect(page.getByText(`Added "Plant the tulips" to ${project}`)).toBeVisible();
		await expect(page.getByRole('list', { name: 'Tasks' })).toContainText('Plant the tulips');
	});

	test('the palette finds a project and goes there', async ({ page }) => {
		const project = unique('Atlas');
		await newProject(page, project);
		await page.goto('/');
		await expect(page.getByRole('heading', { name: 'Inbox', exact: true })).toBeVisible();
		await page.keyboard.press('Control+k');
		await page.getByLabel('Search for a task, project or command').fill(project);
		await page.keyboard.press('Enter');
		await expect(page.getByLabel('Project name')).toHaveValue(project);
		// G then a letter goes to the main pages.
		await page.locator('body').press('g');
		await page.locator('body').press('u');
		await expect(page.getByRole('heading', { name: 'Upcoming' })).toBeVisible();
		await page.locator('body').press('?');
		await expect(page.getByRole('dialog', { name: 'Keyboard shortcuts' })).toBeVisible();
	});

	test('a list is worked from the keyboard', async ({ page }) => {
		await newProject(page, unique('Keys'));
		await addTask(page, 'First');
		await addTask(page, 'Second');
		await addTask(page, 'Third');
		const rows = page.getByRole('list', { name: 'Tasks' }).locator('[data-task-id]');
		await rows.first().focus();
		await page.keyboard.press('j');
		await expect(rows.nth(1)).toBeFocused();
		// Alt+Up moves it; 1 to 4 set the priority; T puts it in My Day; X completes it.
		await page.keyboard.press('Alt+ArrowUp');
		await expect(rows.first()).toContainText('Second');
		await expect(rows.first()).toBeFocused();
		await page.keyboard.press('1');
		await expect(row(page, 'Second').getByRole('img', { name: 'Urgent' })).toBeVisible();
		await page.keyboard.press('t');
		await expect(page.getByText('Added to My Day')).toBeVisible();
		await page.keyboard.press('x');
		await expect(row(page, 'Second')).toBeHidden();

		await page.locator('body').press('g');
		await page.locator('body').press('d');
		await expect(page.getByRole('heading', { name: 'My Day' })).toBeVisible();
		await expect(page.getByRole('main')).toContainText('Second');
	});

	test('board cards move between columns', async ({ page }) => {
		const url = await newProject(page, unique('Board'));
		await addTask(page, 'Card one');
		await addTask(page, 'Card two');
		await page.goto(url + '?view=board');
		const column = (name: string) => page.getByRole('region', { name, exact: true });
		await expect(column('To do')).toContainText('Card one');
		await column('To do')
			.getByRole('button', { name: /Card one/ })
			.focus();
		await page.keyboard.press('Alt+ArrowRight');
		await expect(column('In progress')).toContainText('Card one');
		await expect(column('To do')).not.toContainText('Card one');
		// And with the mouse.
		await column('To do')
			.getByRole('button', { name: /Card two/ })
			.dragTo(column('Done').locator('ol'));
		await expect(column('Done')).toContainText('Card two');
		await page.reload();
		await expect(column('Done')).toContainText('Card two');
		await expect(column('In progress')).toContainText('Card one');
	});

	test('calendar chips reschedule with Alt and the arrows', async ({ page }) => {
		const url = await newProject(page, unique('Dates'));
		await addTask(page, 'Dentist');
		await row(page, 'Dentist').getByRole('button', { name: 'Dentist', exact: true }).click();
		const today = new Date();
		await page.getByLabel('Due date').fill(ymd(today));
		await page.getByRole('button', { name: 'Close' }).click();
		await page.goto(url + '?view=calendar');
		const chip = page.getByRole('button', { name: /^Dentist/ }).first();
		await chip.focus();
		await page.keyboard.press('Alt+ArrowDown');
		const next = new Date(today);
		next.setDate(next.getDate() + 7);
		const label = next.toLocaleDateString(undefined, {
			weekday: 'long',
			month: 'long',
			day: 'numeric'
		});
		await expect(page.getByRole('group', { name: label, exact: true })).toContainText('Dentist');
	});
});

test('custom fields show on tasks and keep their values', async ({ page }) => {
	const field = unique('Cost');
	const choice = unique('Size');
	// Fields for this project only, so other tests' tasks don't grow them.
	const project = unique('Fields');
	const url = await newProject(page, project);
	await page.goto('/settings/fields');
	await page.getByLabel('Where the new field applies').selectOption({ label: project });
	await page.getByLabel('New field', { exact: true }).fill(field);
	await page.getByLabel('Type of the new field').selectOption('number');
	await page.getByRole('button', { name: 'Add field' }).click();
	await page.getByLabel('New field', { exact: true }).fill(choice);
	await page.getByLabel('Type of the new field').selectOption('select');
	await page.getByRole('button', { name: 'Add field' }).click();
	// Choices added in quick succession all stay.
	for (const size of ['Small', 'Medium', 'Large']) {
		await page.getByLabel(`New choice for ${choice}`).fill(size);
		await page.getByLabel(`New choice for ${choice}`).press('Enter');
	}
	await expect(
		page.getByRole('list', { name: `Choices for ${choice}` }).getByLabel('Name of the choice Large')
	).toBeVisible();
	await expect(
		page.getByRole('list', { name: `Choices for ${choice}` }).getByRole('textbox')
	).toHaveCount(4); // three choices and the box for another

	await expect(page.getByRole('list', { name: 'Fields' })).toContainText(`Number · ${project}`);
	await axe(page);
	await page.goto(url);
	await addTask(page, 'Order lumber');
	await row(page, 'Order lumber')
		.getByRole('button', { name: 'Order lumber', exact: true })
		.click();
	const detail = page.getByRole('complementary', { name: 'Task details' });
	await detail.getByLabel(field, { exact: true }).fill('120.5');
	await detail.getByLabel(field, { exact: true }).press('Tab');
	await detail.getByLabel(choice, { exact: true }).selectOption({ label: 'Large' });
	// Reopened, the panel shows what was saved, not what was typed.
	await page.getByRole('button', { name: 'Close' }).click();
	await expect(page).not.toHaveURL(/task=/);
	await row(page, 'Order lumber')
		.getByRole('button', { name: 'Order lumber', exact: true })
		.click();
	// Open again, not the closing sheet still on its way out (phones).
	await expect(page).toHaveURL(/task=/);
	await expect(detail.getByLabel(field, { exact: true })).toHaveValue('120.5');
	await page.reload();
	await expect(detail.getByLabel(field, { exact: true })).toHaveValue('120.5');
	await expect(detail.getByLabel(choice, { exact: true }).locator('option:checked')).toHaveText(
		'Large'
	);
	await axe(page);
});

test('a saved view filters and remembers', async ({ page, isMobile }) => {
	const url = await newProject(page, unique('Filter'));
	await addTask(page, 'Loud task');
	await addTask(page, 'Quiet task');
	await row(page, 'Loud task').getByRole('button', { name: 'Loud task', exact: true }).click();
	await page.getByLabel('Priority', { exact: true }).selectOption('urgent');
	await page.getByRole('button', { name: 'Close' }).click();

	await page.goto(url);
	await page.getByRole('button', { name: 'Save as a view' }).click();
	await expect(page.getByLabel('View name')).toBeVisible();
	const name = unique('Hot');
	await page.getByLabel('View name').fill(name);
	await page.getByLabel('View name').press('Enter');
	const list = page.getByRole('list', { name: 'Tasks' });
	await expect(list).toContainText('Quiet task');
	await page.getByRole('button', { name: 'Filter', exact: true }).click();
	await page
		.getByRole('group', { name: 'Priority' })
		.getByRole('button', { name: 'Urgent' })
		.click();
	await expect(list).toContainText('Loud task');
	await expect(list).not.toContainText('Quiet task');
	await axe(page); // with the filter open

	await page.reload();
	await expect(page.getByLabel('View name')).toHaveValue(name);
	await expect(list).not.toContainText('Quiet task');
	if (!isMobile) await expect(page.getByRole('list', { name: 'Saved views' })).toContainText(name);

	await page.getByRole('button', { name: 'Share with the workspace' }).click();
	await expect(page.getByText(`Shared ${name} with the workspace`)).toBeVisible();
	await page.getByRole('button', { name: `Delete ${name}` }).click();
	await expect(page.getByText(`Deleted the view ${name}`)).toBeVisible();
});

test("a saved view filters on a project's own status", async ({ page }) => {
	const url = await newProject(page, unique('Review'));
	const status = unique('Agent review');
	await page.getByRole('button', { name: 'Edit statuses' }).click();
	const editor = page.getByRole('list', { name: 'Statuses' });
	await editor.getByLabel('Status name').nth(0).fill(status);
	await editor.getByLabel('Status name').nth(0).press('Enter');
	await page.keyboard.press('Escape');
	await addTask(page, 'Waiting task');
	await addTask(page, 'Moving task');
	await editTask(page, 'Waiting task', (d) =>
		d.getByLabel('Status', { exact: true }).selectOption({ label: status })
	);
	await editTask(page, 'Moving task', (d) =>
		d.getByLabel('Status', { exact: true }).selectOption({ label: 'In progress' })
	);

	await page.goto(url);
	await page.getByRole('button', { name: 'Save as a view' }).click();
	const name = unique('Waiting');
	await page.getByLabel('View name').fill(name);
	await page.getByLabel('View name').press('Enter');
	const list = page.getByRole('list', { name: 'Tasks' });
	await expect(list).toContainText('Moving task');
	await page.getByRole('button', { name: 'Filter', exact: true }).click();
	const statuses = page.getByRole('group', { name: 'Status' });
	// Named for their category, the project's other statuses are the category chips.
	await expect(statuses.getByRole('button')).toHaveCount(6);
	await statuses.getByRole('button', { name: status }).click();
	await expect(statuses.getByRole('button', { name: status })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	await expect(list).toContainText('Waiting task');
	await expect(list).not.toContainText('Moving task');

	// A category beside it adds that category's tasks.
	await statuses.getByRole('button', { name: 'In progress' }).click();
	await expect(list).toContainText('Moving task');
	await statuses.getByRole('button', { name: 'In progress' }).click();
	await expect(list).not.toContainText('Moving task');

	await page.reload();
	await expect(page.getByLabel('View name')).toHaveValue(name);
	await expect(list).toContainText('Waiting task');
	await expect(list).not.toContainText('Moving task');
	await page.getByRole('button', { name: `Delete ${name}` }).click();
	await expect(page.getByText(`Deleted the view ${name}`)).toBeVisible();
});

const detailOf = (page: Page) => page.getByRole('complementary', { name: 'Task details' });

/** Opens a task from its row, runs some edits in its panel, and closes it. */
async function editTask(
	page: Page,
	title: string,
	edit: (d: ReturnType<typeof detailOf>) => Promise<void>
) {
	await row(page, title).getByRole('button', { name: title, exact: true }).first().click();
	await expect(page).toHaveURL(/task=/);
	await edit(detailOf(page));
	await page.getByRole('button', { name: 'Close' }).click();
	await expect(page).not.toHaveURL(/task=/);
}

function daysFromNow(n: number) {
	const d = new Date();
	d.setDate(d.getDate() + n);
	return ymd(d);
}

/** Makes fields for one project only, so other tests' tasks don't grow them. */
async function projectFields(
	page: Page,
	project: string,
	fields: { name: string; type: string; choices?: string[] }[]
) {
	await page.goto('/settings/fields');
	for (const f of fields) {
		await page.getByLabel('Where the new field applies').selectOption({ label: project });
		await page.getByLabel('New field', { exact: true }).fill(f.name);
		await page.getByLabel('Type of the new field').selectOption(f.type);
		await page.getByRole('button', { name: 'Add field' }).click();
		for (const c of f.choices ?? []) {
			await page.getByLabel(`New choice for ${f.name}`).fill(c);
			await page.getByLabel(`New choice for ${f.name}`).press('Enter');
			await expect(
				page
					.getByRole('list', { name: `Choices for ${f.name}` })
					.getByLabel(`Name of the choice ${c}`)
			).toBeVisible();
		}
	}
}

async function saveAsView(page: Page, name: string) {
	await page.getByRole('button', { name: 'Save as a view' }).click();
	await expect(page.getByLabel('View name')).toBeVisible();
	await page.getByLabel('View name').fill(name);
	await page.getByLabel('View name').press('Enter');
}

const headings = (page: Page) =>
	page.getByRole('list', { name: 'Tasks' }).getByRole('heading', { level: 2 });
const titles = (page: Page) =>
	page.getByRole('list', { name: 'Tasks' }).locator('[data-task-id] .title .text');

test('a saved view sorts, groups and shows a field, and remembers', async ({ page }) => {
	const project = unique('Beds');
	const size = unique('Size');
	const url = await newProject(page, project);
	await projectFields(page, project, [{ name: size, type: 'select', choices: ['Small', 'Large'] }]);
	await page.goto(url);
	for (const [title, due, choice] of [
		['Plant', 3, 'Large'],
		['Weed', 1, 'Large'],
		['Mulch', 2, 'Large'],
		['Water', 5, 'Small']
	] as const) {
		await addTask(page, title);
		await editTask(page, title, async (d) => {
			await d.getByLabel('Due date').fill(daysFromNow(due));
			await d.getByLabel(size, { exact: true }).selectOption({ label: choice });
		});
	}
	await saveAsView(page, unique('Sorted'));
	await page.getByRole('button', { name: 'Display' }).click();
	const display = page.getByRole('group', { name: 'Display' });
	await display.getByRole('button', { name: 'Add a sort' }).click();
	await display.getByLabel('Sort by', { exact: true }).selectOption('due');
	await expect(page.locator('#announcer')).toHaveText(/Sorted by due date, soonest first/);
	await display.getByLabel('Group by').selectOption({ label: size });
	// Two quick changes to the shown fields: the second builds on the first.
	const show = display.getByRole('group', { name: 'Show' });
	await show.getByText(size, { exact: true }).click();
	await show.getByText('Start date', { exact: true }).click();
	await expect(display.getByRole('checkbox', { name: size })).toBeChecked();
	await expect(display.getByRole('checkbox', { name: 'Start date' })).toBeChecked();
	await axe(page); // with the display panel open

	const check = async () => {
		await expect(headings(page)).toHaveText([/^Small/, /^Large/]);
		await expect(titles(page)).toHaveText(['Water', 'Weed', 'Mulch', 'Plant']);
		await expect(row(page, 'Weed')).toContainText(`${size}: Large`);
	};
	await check();
	await page.reload();
	await check();
	// Back to manual order; focus goes on to Add a sort, and Escape still
	// closes the panel.
	await page.getByRole('button', { name: 'Display' }).click();
	await expect(display.getByRole('checkbox', { name: 'Start date' })).toBeChecked();
	await display.getByRole('button', { name: 'Remove sort by Due date' }).click();
	await expect(titles(page)).toHaveText(['Water', 'Plant', 'Weed', 'Mulch']);
	await expect(display.getByRole('button', { name: 'Add a sort' })).toBeFocused();
	await page.keyboard.press('Escape');
	await expect(display).toBeHidden();
	await expect(page.getByRole('button', { name: 'Display' })).toBeFocused();
});

test('a saved view filters on custom fields', async ({ page }) => {
	const project = unique('Costs');
	const [cost, by, kind] = [unique('Cost'), unique('By'), unique('Kind')];
	const url = await newProject(page, project);
	await projectFields(page, project, [
		{ name: cost, type: 'number' },
		{ name: by, type: 'date' },
		{ name: kind, type: 'select', choices: ['Alpha', 'Beta'] }
	]);
	await page.goto(url);
	await addTask(page, 'Pricey');
	await editTask(page, 'Pricey', async (d) => {
		await d.getByLabel(cost, { exact: true }).fill('50');
		await d.getByLabel(cost, { exact: true }).press('Tab');
		await d.getByLabel(by, { exact: true }).fill(daysFromNow(-1));
		await d.getByLabel(kind, { exact: true }).selectOption({ label: 'Alpha' });
	});
	await addTask(page, 'Cheap');
	await editTask(page, 'Cheap', async (d) => {
		await d.getByLabel(cost, { exact: true }).fill('5');
		await d.getByLabel(cost, { exact: true }).press('Tab');
		await d.getByLabel(by, { exact: true }).fill(daysFromNow(1));
		await d.getByLabel(kind, { exact: true }).selectOption({ label: 'Beta' });
	});
	await addTask(page, 'Blank');
	await saveAsView(page, unique('Fields'));
	const list = page.getByRole('list', { name: 'Tasks' });
	await page.getByRole('button', { name: 'Filter', exact: true }).click();
	const filter = page.getByRole('group', { name: 'Filter' });
	const add = filter.getByRole('button', { name: 'Add a field condition' });

	await add.click();
	await filter.getByLabel('Condition 1 field').selectOption({ label: cost });
	await filter.getByLabel(`${cost} comparison`).selectOption('gt');
	await filter.getByLabel(`${cost} number`).fill('10');
	await filter.getByLabel(`${cost} number`).press('Tab');
	await expect(list).toContainText('Pricey');
	await expect(list).not.toContainText('Cheap');
	await expect(list).not.toContainText('Blank');
	await expect(page.locator('#announcer')).toHaveText(/1 task matches/);
	await filter.getByRole('button', { name: `Remove ${cost} condition` }).click();

	await add.click();
	await filter.getByLabel('Condition 1 field').selectOption({ label: by });
	await filter.getByLabel(`${by} comparison`).selectOption('before');
	await expect(filter.getByLabel(`${by} date`)).toHaveValue('0');
	await expect(list).toContainText('Pricey');
	await expect(list).not.toContainText('Cheap');
	await filter.getByRole('button', { name: `Remove ${by} condition` }).click();

	await add.click();
	await filter.getByLabel('Condition 1 field').selectOption({ label: kind });
	await filter.getByLabel(`${kind} comparison`).selectOption('none_of');
	await filter
		.getByRole('group', { name: `${kind} choices` })
		.getByRole('button', { name: 'Alpha' })
		.click();
	await expect(list).toContainText('Cheap');
	await expect(list).toContainText('Blank');
	await expect(list).not.toContainText('Pricey');
	await axe(page);
	await page.reload();
	await expect(list).toContainText('Blank');
	await expect(list).not.toContainText('Pricey');
});

test.describe('views from the keyboard', () => {
	test.skip(({ isMobile }) => isMobile, 'phones have no keyboard shortcuts');

	test('a sorted list keeps its order, and a task in two groups is reached in each', async ({
		browser
	}) => {
		// Labels are workspace-wide, so this happens in someone else's workspace.
		const { page: own, context } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Kit');
		await own.goto('/settings/labels');
		for (const name of ['Errand', 'Home']) {
			await own.getByLabel('New label', { exact: true }).fill(name);
			await own.getByRole('button', { name: 'Add label' }).click();
			await expect(
				own.getByRole('list', { name: 'Labels' }).getByLabel(`Name of ${name}`)
			).toBeVisible();
		}
		const project = unique('Keys');
		await newProject(own, project);
		for (const text of [
			`Both +Errand +Home #${project}`,
			`Solo +Home #${project}`,
			`Late #${project}`
		]) {
			await own.locator('body').press('q');
			await own.getByLabel('Quick add', { exact: true }).fill(text);
			await own.getByLabel('Quick add', { exact: true }).press('Enter');
			await expect(row(own, text.split(' ')[0])).toBeVisible();
		}
		await editTask(own, 'Both', (d) => d.getByLabel('Due date').fill(daysFromNow(2)));
		await editTask(own, 'Solo', (d) => d.getByLabel('Due date').fill(daysFromNow(1)));
		await saveAsView(own, 'By label');
		await own.getByRole('button', { name: 'Display' }).click();
		const display = own.getByRole('group', { name: 'Display' });
		await display.getByRole('button', { name: 'Add a sort' }).click();
		await display.getByLabel('Sort by', { exact: true }).selectOption('due');
		await expect(titles(own)).toHaveText(['Solo', 'Both', 'Late']);

		// Sorted, the move keys only say why they cannot move a task.
		const rows = own.getByRole('list', { name: 'Tasks' }).locator('[data-task-id]');
		await rows.nth(1).focus();
		await own.keyboard.press('Alt+ArrowUp');
		await expect(own.locator('#announcer')).toHaveText(/Choose manual order/);
		await expect(titles(own)).toHaveText(['Solo', 'Both', 'Late']);

		await display.getByLabel('Group by').selectOption('label');
		await expect(headings(own)).toHaveText([/^Errand/, /^Home/, /^No label/]);
		// Errand: Both; Home: Solo, Both; No label: Late.
		const focused = () =>
			own.evaluate(() => (document.activeElement as HTMLElement).dataset.rowKey ?? '');
		await rows.first().focus();
		const first = await focused();
		expect(first).toMatch(/^l:.+\/.+/);
		await own.keyboard.press('j');
		const second = await focused();
		await own.keyboard.press('j');
		const third = await focused();
		// The same task, under its other label.
		expect(third).not.toBe(first);
		expect(third.split('/')[1]).toBe(first.split('/')[1]);
		await own.keyboard.press('k');
		expect(await focused()).toBe(second);
		// Completing a task leaves focus on another task, not on its other
		// copy, which goes too.
		await rows.first().focus();
		await own.keyboard.press('x');
		await expect(row(own, 'Both')).toHaveCount(0);
		await expect(own.locator(':focus')).toContainText('Solo');
		await context.close();
	});

	test('a view across projects moves tasks only when grouped by project', async ({ page }) => {
		const word = unique('Mixed');
		const left = await newProject(page, unique('Left'));
		await addTask(page, `${word} one`);
		await newProject(page, unique('Right'));
		await addTask(page, `${word} two`);
		await page.goto(left);
		await expect(row(page, `${word} one`)).toBeVisible();
		await addTask(page, `${word} three`);
		await page.getByRole('button', { name: 'New view' }).click();
		await expect(page.getByLabel('View name')).toHaveValue('New view');
		await page.getByRole('button', { name: /^Filter/ }).click();
		const filter = page.getByRole('group', { name: 'Filter' });
		await filter
			.getByRole('group', { name: 'Assignee' })
			.getByRole('button', { name: 'Me' })
			.click();
		await filter.getByLabel('Words in the title or notes').fill(word);
		await filter.getByLabel('Words in the title or notes').press('Tab');
		// Grouped by project, as a view across projects starts.
		await expect(titles(page)).toHaveText([`${word} one`, `${word} three`, `${word} two`]);
		await page.getByRole('button', { name: 'Display' }).click();
		const display = page.getByRole('group', { name: 'Display' });
		await display.getByLabel('Group by').selectOption('priority');
		await expect(headings(page)).toHaveText([/^No priority/]);
		const rows = page.getByRole('list', { name: 'Tasks' }).locator('[data-task-id]');
		const before = await titles(page).allTextContents();
		await rows.nth(2).focus();
		await page.keyboard.press('Alt+ArrowUp');
		await expect(page.locator('#announcer')).toHaveText(/Group by project to move tasks/);
		await expect(titles(page)).toHaveText(before);

		await display.getByLabel('Group by').selectOption('project');
		await expect(titles(page)).toHaveText([`${word} one`, `${word} three`, `${word} two`]);
		await expect(page.locator('#announcer')).toHaveText('Grouped by project');
		await rows.nth(1).focus();
		await page.keyboard.press('Alt+ArrowUp');
		await expect(titles(page)).toHaveText([`${word} three`, `${word} one`, `${word} two`]);
		await page.getByRole('button', { name: 'Delete New view' }).click();
	});

	test('a board with priority columns sets the priority', async ({ page }) => {
		const url = await newProject(page, unique('Prio'));
		await addTask(page, 'Card one');
		await page.goto(url);
		await saveAsView(page, unique('Prio board'));
		await page
			.getByRole('group', { name: 'Show as' })
			.getByRole('button', { name: 'Board' })
			.click();
		await page.getByRole('button', { name: 'Display' }).click();
		await page
			.getByRole('group', { name: 'Display' })
			.getByLabel('Columns')
			.selectOption('priority');
		const column = (name: string) => page.getByRole('region', { name, exact: true });
		await expect(column('No priority')).toContainText('Card one');
		await column('No priority')
			.getByRole('button', { name: /Card one/ })
			.focus();
		await page.keyboard.press('Alt+ArrowLeft');
		await expect(column('Low')).toContainText('Card one');
		await column('High').getByRole('button', { name: '+ Add a task' }).click();
		await page.getByLabel('New task in High').fill('Card two');
		await page.getByLabel('New task in High').press('Enter');
		await expect(column('High')).toContainText('Card two');
		await page.reload();
		await expect(column('Low')).toContainText('Card one');
		await column('High')
			.getByRole('button', { name: /Card two/ })
			.click();
		await expect(detailOf(page).getByLabel('Priority', { exact: true })).toHaveValue('high');
	});
});

test('a guest changes a shared view for themselves only', async ({ page, browser }) => {
	const {
		page: guest,
		context,
		workspace
	} = await newMember(page, () => browser.newContext(SIGNED_OUT), 'Gwen', 'guest');
	await page.evaluate((id) => localStorage.setItem('mila.workspace', id), workspace.id);
	const project = unique('Shared');
	const url = await newProject(page, project);
	for (const t of ['Zinnia', 'Aster', 'Moss']) await addTask(page, t);
	// Sharing asks the server about the project, so it has to have arrived there.
	const projectId = url.split('/').pop()!.split('?')[0];
	await expect
		.poll(
			async () =>
				(await apiCall(page, 'GET', `/workspaces/${workspace.id}/projects/${projectId}`)).status
		)
		.toBe(200);
	await page.getByRole('button', { name: `Share ${project} with guests` }).click();
	const dialog = page.getByRole('dialog', { name: `Share ${project}` });
	await dialog.getByRole('checkbox', { name: `Gwen can see ${project}` }).check();
	await expect(page.getByText(`Shared ${project} with Gwen`).last()).toBeVisible();
	await dialog.getByRole('button', { name: 'Done' }).click();
	await page.goto(url);
	const team = unique('Team');
	await saveAsView(page, team);
	const personal = page.url();
	await page.getByRole('button', { name: 'Share with the workspace' }).click();
	await expect(page).not.toHaveURL(personal);
	const viewUrl = page.url();
	await expect(titles(page)).toHaveText(['Zinnia', 'Aster', 'Moss']);

	await guest.evaluate((id) => localStorage.setItem('mila.workspace', id), workspace.id);
	await guest.goto(viewUrl);
	await expect(titles(guest)).toHaveText(['Zinnia', 'Aster', 'Moss'], { timeout: 15_000 });
	await expect(guest.getByLabel('View name')).toHaveAttribute('readonly', '');
	await expect(guest.getByRole('button', { name: `Delete ${team}` })).toHaveCount(0);
	await expect(guest.getByRole('button', { name: 'Share with the workspace' })).toHaveCount(0);
	await guest.getByRole('button', { name: 'Display' }).click();
	await guest
		.getByRole('group', { name: 'Display' })
		.getByRole('button', { name: 'Add a sort' })
		.click();
	await expect(titles(guest)).toHaveText(['Aster', 'Moss', 'Zinnia']);
	const note = guest.getByRole('region', { name: 'Unsaved changes' });
	await expect(note).toContainText('only for you');
	await axe(guest);
	// Nor can a guest make views.
	await expect(guest.getByRole('button', { name: 'New view' })).toHaveCount(0);
	await guest.goto(url);
	await expect(guest.getByRole('button', { name: 'Save as a view' })).toHaveCount(0);
	await guest.goto(viewUrl);
	await guest.getByRole('button', { name: 'Display' }).click();
	await guest
		.getByRole('group', { name: 'Display' })
		.getByRole('button', { name: 'Add a sort' })
		.click();
	await expect(titles(guest)).toHaveText(['Aster', 'Moss', 'Zinnia']);

	// Nothing was sent: the owner's view keeps its order.
	await page.reload();
	await expect(titles(page)).toHaveText(['Zinnia', 'Aster', 'Moss']);
	await note.getByRole('button', { name: 'Reset' }).click();
	await expect(note).toBeHidden();
	await expect(titles(guest)).toHaveText(['Zinnia', 'Aster', 'Moss']);
	await context.close();
});

test('Upcoming lists what is due soon', async ({ page }) => {
	await newProject(page, unique('Soon'));
	await addTask(page, 'Renew passport');
	await row(page, 'Renew passport')
		.getByRole('button', { name: 'Renew passport', exact: true })
		.click();
	const soon = new Date();
	soon.setDate(soon.getDate() + 2);
	await page.getByLabel('Due date').fill(ymd(soon));
	await page.getByRole('button', { name: 'Close' }).click();
	await page.goto('/upcoming');
	await expect(page.getByRole('main')).toContainText('Renew passport');
	await axe(page);
});

test("a deleted label's Undo brings back that label, not the newest change", async ({
	browser
}) => {
	// Labels are workspace-wide, so this happens in someone else's workspace.
	const { page: own, context } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Linden');
	await own.goto('/settings/labels');
	const labels = own.getByRole('list', { name: 'Labels' });
	for (const name of ['Keep me', 'Rename me']) {
		await own.getByLabel('New label', { exact: true }).fill(name);
		await own.getByRole('button', { name: 'Add label' }).click();
		await expect(labels.getByLabel(`Name of ${name}`)).toBeVisible();
	}
	await labels
		.getByRole('listitem')
		.filter({ has: own.getByLabel('Name of Keep me') })
		.getByRole('button', { name: 'Delete' })
		.click();
	await expect(labels.getByLabel('Name of Keep me')).toBeHidden();
	// A later edit, which the toast's Undo must leave alone.
	await labels.getByLabel('Name of Rename me').fill('Renamed');
	await labels.getByLabel('Name of Rename me').press('Tab');
	await expect(labels.getByLabel('Name of Renamed')).toBeVisible();
	await own.getByRole('button', { name: 'Undo', exact: true }).click();
	await expect(labels.getByLabel('Name of Keep me')).toBeVisible();
	await expect(labels.getByLabel('Name of Renamed')).toBeVisible();
	await context.close();
});
