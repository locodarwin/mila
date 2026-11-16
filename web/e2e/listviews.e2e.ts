// Inbox, My Day and Upcoming take a view, a filter and a display, kept on the
// device for each list, a way back (Reset) and Save as view (docs/PLAN.md
// section 3).
import { expect, test } from '@playwright/test';
import { axe } from './axe';
import { pairId, SYNC_SCHEMA } from '../src/lib/sync/entities';
import { apiCall, newPerson, SIGNED_OUT } from './helpers';

const day = (n: number) => {
	const d = new Date();
	d.setDate(d.getDate() + n);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

test('Upcoming as a table, filtered, kept on the device, reset and saved as a view', async ({
	browser,
	isMobile
}) => {
	const { page, context, workspace } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Ula');
	const ws = workspace.id;
	for (const [title, due, priority] of [
		['Pay the electricity bill', day(0), 'high'],
		['Order printer ink', day(2), 'low'],
		['Book the dentist', null, 'high']
	] as const) {
		const res = await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, {
			title,
			due_date: due,
			priority
		});
		expect(res.status).toBe(201);
	}
	await page.goto(`/upcoming?ws=${ws}`);
	const list = page.getByRole('list', { name: 'Upcoming tasks' });
	await expect(list).toContainText('Order printer ink');
	await expect(list).not.toContainText('Book the dentist');
	await expect(page.getByRole('button', { name: 'Reset' })).toHaveCount(0);

	// A table, kept through a reload.
	const showAs = page.getByRole('group', { name: 'Show as' });
	await showAs.getByRole('button', { name: 'Table' }).click();
	// A phone shows a table's rows as a list.
	const table = isMobile ? list : page.getByRole('treegrid');
	await expect(table).toContainText('Pay the electricity bill');
	await expect(table).not.toContainText('Book the dentist');
	await page.reload();
	await expect(table).toContainText('Order printer ink');
	await expect(showAs.getByRole('button', { name: 'Table' })).toHaveAttribute(
		'aria-pressed',
		'true'
	);

	// Filtered to high priority, in the table and back in the list.
	await page.getByRole('button', { name: 'Filter', exact: true }).click();
	await page.getByRole('group', { name: 'Priority' }).getByRole('button', { name: 'High' }).click();
	// At once, with the filter open (a table once kept its first answers).
	await expect(table).not.toContainText('Order printer ink');
	await expect(page.getByRole('button', { name: /Filter/ }).locator('.badge')).toHaveText('1');
	await axe(page);
	await showAs.getByRole('button', { name: 'List' }).click();
	await expect(list).toContainText('Pay the electricity bill');
	await expect(list).not.toContainText('Order printer ink');
	// The list's own sections, filtered.
	await expect(list.getByRole('heading', { name: 'Today' })).toBeVisible();

	// Saved as a view of its own: only dated, high-priority tasks, as here.
	await page.getByRole('button', { name: 'Save as view' }).click();
	await expect(page).toHaveURL(/\/views\//);
	await expect(page.getByLabel('View name')).toHaveValue('Upcoming list');
	const saved = page.getByRole('list', { name: 'Tasks' });
	await expect(saved).toContainText('Pay the electricity bill');
	await expect(saved).not.toContainText('Book the dentist');
	await expect(saved).not.toContainText('Order printer ink');

	// Back on Upcoming, Reset puts it back as it comes, and Undo undoes that.
	await page.goto(`/upcoming?ws=${ws}`);
	await page.getByRole('button', { name: 'Reset' }).click();
	await expect(list).toContainText('Order printer ink');
	await expect(page.getByRole('button', { name: 'Reset' })).toHaveCount(0);
	await page.getByRole('button', { name: 'Undo' }).click();
	await expect(list).not.toContainText('Order printer ink');
	await context.close();
});

test('My Day as a calendar shows what is picked, and Inbox keeps its own list', async ({
	browser,
	isMobile
}) => {
	const { page, context, workspace } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Mo');
	const ws = workspace.id;
	const me = (await apiCall(page, 'GET', '/me')).body.id as string;
	const task = (
		await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, {
			title: 'Water the seedlings',
			due_date: day(0)
		})
	).body.id as string;
	await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, { title: 'Not picked', due_date: day(0) });
	const push = await apiCall(page, 'POST', `/workspaces/${ws}/sync/push`, {
		schema: SYNC_SCHEMA,
		ops: [
			{
				opId: crypto.randomUUID(),
				entity: 'my_day_entry',
				id: pairId(task, me),
				hlc: `${String(Date.now()).padStart(13, '0')}-0000-test`,
				fields: { task_id: task, user_id: me, day: day(0), sort_key: 'a0' }
			}
		]
	});
	expect(push.status).toBe(200);
	await page.goto(`/today?ws=${ws}`);
	await expect(page.getByRole('list', { name: 'My Day' })).toContainText('Not picked');
	await page
		.getByRole('group', { name: 'Show as' })
		.getByRole('button', { name: 'Calendar' })
		.click();
	const main = page.locator('main');
	await expect(main).toContainText('Water the seedlings');
	await expect(main).not.toContainText('Not picked');
	// My Day's own filter has no use for "In my My Day".
	await page.getByRole('button', { name: 'Filter', exact: true }).click();
	await expect(page.getByLabel('Show finished tasks')).toBeVisible();
	await expect(page.getByLabel('In my My Day')).toHaveCount(0);

	// Each list keeps its own: the Inbox is still a list, with its add box.
	await page.goto(`/?ws=${ws}`);
	await expect(
		page.getByRole('group', { name: 'Show as' }).getByRole('button', { name: 'List' })
	).toHaveAttribute('aria-pressed', 'true');
	if (!isMobile) await expect(page.getByLabel('New task')).toBeVisible();
	await page.goto(`/today?ws=${ws}`);
	await expect(
		page.getByRole('group', { name: 'Show as' }).getByRole('button', { name: 'Calendar' })
	).toHaveAttribute('aria-pressed', 'true');
	await context.close();
});

test('a busy day in the calendar scrolls in its own square, and every week still shows', async ({
	browser,
	isMobile
}) => {
	test.skip(isMobile, 'a phone shows the agenda instead');
	const { page, context, workspace } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Cy');
	const ws = workspace.id;
	// Today is always in the month shown, whichever week it falls in.
	for (let i = 0; i < 20; i++)
		await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, {
			title: `Rehearsal step ${i}`,
			due_date: day(0)
		});
	await page.goto(`/upcoming?ws=${ws}`);
	await expect(page.locator('main')).toContainText('Rehearsal step 19');
	await page
		.getByRole('group', { name: 'Show as' })
		.getByRole('button', { name: 'Calendar' })
		.click();
	const weeks = page.locator('.calendar .days');
	await expect(weeks.locator('.day')).toHaveCount(42);
	// The six weeks share the calendar's height: none grows to the busy
	// day's, which once pushed the later weeks out of sight.
	const box = await page.locator('.calendar .grid').boundingBox();
	const tops = await weeks
		.locator('.day')
		.evaluateAll((days) => [0, 7, 14, 21, 28, 35].map((i) => days[i].getBoundingClientRect()));
	const heights = tops.map((r) => Math.round(r.height));
	expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(1);
	expect(tops[5].bottom).toBeLessThanOrEqual(box!.y + box!.height + 1);
	// What doesn't fit is reached by scrolling the day.
	const last = page.getByRole('button', { name: 'Rehearsal step 9', exact: true });
	await last.scrollIntoViewIfNeeded();
	await expect(last).toBeInViewport();
	// And from the keyboard: Tab goes through the day's entries, each
	// scrolled into view as it takes focus.
	const busy = page.locator('.calendar .day').filter({ has: last });
	await busy.evaluate((el) => el.scrollTo(0, 0));
	await expect(last).not.toBeInViewport();
	const entries = busy.getByRole('button', { name: /^Rehearsal step/ });
	await entries.first().focus();
	for (let i = 1; i < 20; i++) await page.keyboard.press('Tab');
	await expect(entries.last()).toBeFocused();
	await expect(entries.last()).toBeInViewport();
	await axe(page);
	await context.close();
});

test('the Inbox stays together, and keeps its add box when filtered', async ({
	browser,
	isMobile
}) => {
	test.skip(isMobile, 'the same layout; once is enough');
	const { page, context, workspace } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Ina');
	const ws = workspace.id;
	for (const [title, priority] of [
		['Call the broker', 'high'],
		['Read the lease', 'low']
	] as const)
		await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, { title, priority });
	const done = (await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, { title: 'Filed already' }))
		.body.id as string;
	await apiCall(page, 'PATCH', `/workspaces/${ws}/tasks/${done}`, {
		completed_at: new Date().toISOString()
	});
	await page.goto(`/?ws=${ws}`);
	const add = page.getByLabel('New task');
	const first = page.getByRole('button', { name: 'Call the broker', exact: true });
	const last = page.getByRole('button', { name: 'Read the lease', exact: true });
	const completed = page.getByRole('button', { name: /^Completed/ });
	await expect(completed).toBeVisible();
	const box = async (l: typeof add) => (await l.boundingBox())!;
	// Each piece right under the one before, not spread down the page.
	expect((await box(first)).y - ((await box(add)).y + (await box(add)).height)).toBeLessThan(40);
	expect((await box(completed)).y - ((await box(last)).y + (await box(last)).height)).toBeLessThan(
		60
	);

	// Filtered, the add box stays, and a task added there arrives.
	await page.getByRole('button', { name: 'Filter', exact: true }).click();
	await page.getByRole('group', { name: 'Priority' }).getByRole('button', { name: 'High' }).click();
	await expect(page.getByRole('list', { name: 'Inbox' })).not.toContainText('Read the lease');
	await add.fill('Renew the van insurance');
	await add.press('Enter');
	await expect
		.poll(async () => JSON.stringify((await apiCall(page, 'GET', `/workspaces/${ws}/tasks`)).body))
		.toContain('Renew the van insurance');
	await context.close();
});

test('Inbox, My Day and Upcoming as a board, by date, priority or a workspace field', async ({
	browser,
	isMobile
}) => {
	const { page, context, workspace } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Bo');
	const ws = workspace.id;
	const project = (await apiCall(page, 'POST', `/workspaces/${ws}/projects`, { name: 'Garden' }))
		.body.id as string;
	const make = async (fields: Record<string, unknown>) => {
		const res = await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, fields);
		expect(res.status).toBe(201);
		return res.body.id as string;
	};
	const ferns = await make({ title: 'Water the ferns', due_date: day(1), priority: 'low' });
	await make({ title: 'Prune the hedge', due_date: day(3), priority: 'high', project_id: project });
	await make({ title: 'Undated chore', priority: 'high' });
	await make({ title: 'Renew the permit', due_date: day(-2), priority: 'medium' });
	const dueOf = async (id: string) =>
		(await apiCall(page, 'GET', `/workspaces/${ws}/tasks/${id}`)).body.due_date as string | null;

	// Upcoming's board starts with its own days: Overdue, Today, Tomorrow,
	// the days of the week by name, then Later.
	await page.goto(`/upcoming?ws=${ws}`);
	const showAs = page.getByRole('group', { name: 'Show as' });
	await showAs.getByRole('button', { name: 'Board' }).click();
	const board = page.getByRole('group', { name: 'Board' });
	const column = (name: string) => board.locator(`section[aria-label="${name}"]`);
	await expect(board.locator('section h2')).toHaveText([
		'Overdue',
		'Today',
		'Tomorrow',
		/day, /,
		/day, /,
		/day, /,
		/day, /,
		/day, /,
		'Later'
	]);
	await expect(column('Overdue')).toContainText('Renew the permit');
	await expect(column('Tomorrow')).toContainText('Water the ferns');
	await expect(board.locator('section').nth(4)).toContainText('Prune the hedge');
	await expect(board).not.toContainText('Undated chore');
	// Nothing is added to the past; a task added to a day is due that day.
	await expect(column('Overdue').getByRole('button', { name: '+ Add a task' })).toHaveCount(0);
	await column('Today').getByRole('button', { name: '+ Add a task' }).click();
	await page.getByLabel('New task in Today').fill('Mail the forms');
	await page.getByLabel('New task in Today').press('Enter');
	await expect(column('Today')).toContainText('Mail the forms');
	await axe(page);

	if (!isMobile) {
		// A card moved a column over is due that day; none moves into the past.
		// Focus comes back to the card a frame after each move.
		const card = board.getByRole('button', { name: /Water the ferns/ });
		await card.focus();
		await page.keyboard.press('Alt+ArrowRight');
		await expect(board.locator('section').nth(3)).toContainText('Water the ferns');
		await expect.poll(() => dueOf(ferns)).toBe(day(2));
		await expect(card).toBeFocused();
		// The move is said with the day it went to.
		const dayName = await board.locator('section h2').nth(3).textContent();
		await expect(page.locator('#announcer')).toContainText(`Moved Water the ferns to ${dayName}`);
		await page.keyboard.press('Alt+ArrowLeft');
		await expect(column('Tomorrow')).toContainText('Water the ferns');
		await expect(card).toBeFocused();
		await page.keyboard.press('Alt+ArrowLeft');
		await expect(column('Today')).toContainText('Water the ferns');
		await expect(card).toBeFocused();
		await page.keyboard.press('Alt+ArrowLeft');
		await expect(column('Overdue')).not.toContainText('Water the ferns');
		await expect(column('Today')).toContainText('Water the ferns');
		await expect.poll(() => dueOf(ferns)).toBe(day(0));
		// The arrows alone walk the cards across the days.
		await page.keyboard.press('ArrowLeft');
		await expect(board.getByRole('button', { name: /Renew the permit/ })).toBeFocused();
	}

	// By priority instead: a column for each, the cards from every project,
	// and nowhere to add one, since it would have no date.
	await page.getByRole('button', { name: 'Display' }).click();
	const columns = page.getByRole('combobox', { name: /^Columns/ });
	// Columns are what every task has: no statuses, which are a project's own.
	await expect(columns.locator('option')).toHaveText(['Priority', 'Due date']);
	await columns.selectOption('Priority');
	await page.getByRole('button', { name: 'Display' }).click();
	await expect(column('Low')).toContainText('Water the ferns');
	await expect(column('High')).toContainText('Prune the hedge');
	// Each card says whose project it is, the Inbox's included.
	await expect(column('High')).toContainText('Garden');
	await expect(column('Low')).toContainText('Inbox');
	await expect(board.getByRole('button', { name: '+ Add a task' })).toHaveCount(0);

	if (!isMobile) {
		// A card moved a column over takes its priority.
		await board.getByRole('button', { name: /Water the ferns/ }).focus();
		await page.keyboard.press('Alt+ArrowLeft');
		await expect(column('Medium')).toContainText('Water the ferns');
		await expect
			.poll(
				async () => (await apiCall(page, 'GET', `/workspaces/${ws}/tasks/${ferns}`)).body.priority
			)
			.toBe('medium');
	}

	// The Inbox's board adds a task to a column, with its priority, and saved
	// as a view it stays a board.
	await page.goto(`/?ws=${ws}`);
	await showAs.getByRole('button', { name: 'Board' }).click();
	await column('Urgent').getByRole('button', { name: '+ Add a task' }).click();
	await page.getByLabel('New task in Urgent').fill('Call the plumber');
	await page.getByLabel('New task in Urgent').press('Enter');
	await expect(column('Urgent')).toContainText('Call the plumber');
	await expect(board).not.toContainText('Prune the hedge');
	// Every card here is the Inbox's, so none says so.
	await expect(board).not.toContainText('Inbox');
	await page.getByRole('button', { name: 'Save as view' }).click();
	await expect(page).toHaveURL(/\/views\//);
	await expect(page.getByLabel('View name')).toHaveValue('Inbox board');
	await expect(column('Urgent')).toContainText('Call the plumber');
	await expect(board).not.toContainText('Prune the hedge');
	await expect(board).not.toContainText('Inbox');

	// My Day's board: a task added there is in My Day.
	await page.goto(`/today?ws=${ws}`);
	await showAs.getByRole('button', { name: 'Board' }).click();
	await column('Low').getByRole('button', { name: '+ Add a task' }).click();
	await page.getByLabel('New task in Low').fill('Sweep the porch');
	await page.getByLabel('New task in Low').press('Enter');
	await expect(column('Low')).toContainText('Sweep the porch');
	await page.reload();
	await expect(column('Low')).toContainText('Sweep the porch');
	await context.close();
});

test("a board's columns and a timeline's task names resize, kept on the device", async ({
	browser,
	isMobile
}) => {
	test.skip(
		isMobile,
		"a phone's board columns are the screen's width, and its timeline has no names"
	);
	const { page, context, workspace } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Wy');
	const ws = workspace.id;
	await apiCall(page, 'POST', `/workspaces/${ws}/tasks`, {
		title: 'Fix the gate',
		due_date: day(0)
	});
	await page.goto(`/upcoming?ws=${ws}`);
	const showAs = page.getByRole('group', { name: 'Show as' });
	await showAs.getByRole('button', { name: 'Board' }).click();
	const board = page.getByRole('group', { name: 'Board' });
	const widthOf = async (l: ReturnType<typeof page.locator>) =>
		Math.round((await l.boundingBox())!.width);
	const today = board.locator('section[aria-label="Today"]');
	await expect(today).toContainText('Fix the gate');
	const start = await widthOf(today);

	// Every column is one width; the first edge takes the arrow keys.
	const edges = board.getByRole('separator', { name: "Resize the board's columns" });
	await edges.first().focus();
	await page.keyboard.press('ArrowRight');
	await page.keyboard.press('ArrowRight');
	await expect.poll(() => widthOf(today)).toBe(start + 32);
	await expect.poll(() => widthOf(board.locator('section').nth(5))).toBe(start + 32);
	// One Tab stop for every edge: the others are reached by the first.
	await expect(edges.first()).toHaveAttribute('tabindex', '0');
	await expect(edges.nth(1)).toHaveAttribute('tabindex', '-1');
	await expect(edges.first()).toHaveAttribute('aria-valuenow', /^\d+$/);
	// The edge after the third column, dragged 60 px, makes each 20 wider.
	const third = (await edges.nth(2).boundingBox())!;
	await page.mouse.move(third.x + third.width / 2, third.y + 40);
	await page.mouse.down();
	await page.mouse.move(third.x + third.width / 2 + 30, third.y + 40);
	await page.mouse.move(third.x + third.width / 2 + 60, third.y + 40);
	await page.mouse.up();
	await expect.poll(() => widthOf(today)).toBe(start + 52);
	await page.reload();
	await expect.poll(() => widthOf(today)).toBe(start + 52);
	// A double-click puts the board back to its own fit.
	await edges.first().dblclick();
	await expect.poll(() => widthOf(today)).toBe(start);

	// The timeline's task names, the same way.
	await showAs.getByRole('button', { name: 'Timeline' }).click();
	const names = page.getByRole('separator', { name: 'Resize the task names' });
	await expect(names).toHaveAttribute('aria-valuenow', '280');
	await names.focus();
	await page.keyboard.press('Shift+ArrowRight');
	await expect(names).toHaveAttribute('aria-valuenow', '344');
	await expect.poll(() => widthOf(page.locator('.corner'))).toBe(344);
	await axe(page);
	await context.close();
});
