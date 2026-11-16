// The layout: resizable sidebar and task panel, the full-page task, menus
// under their buttons, names at their longest, moving between views,
// reduced motion, the installable app, and what signing out leaves behind.
import { expect, test, type Page } from '@playwright/test';
import { axe, settle } from './axe';
import { apiCall, OWNER, SIGNED_OUT, newPerson } from './helpers';

test.use({ storageState: OWNER });

async function newProject(page: Page, name: string) {
	await page.goto('/projects');
	await page.getByLabel('Name a new project').fill(name);
	await page.getByLabel('Name a new project').press('Enter');
	await expect(page.getByLabel('Project name')).toHaveValue(name);
	return page.url();
}

const row = (page: Page, title: string) =>
	page.getByRole('list', { name: 'Tasks' }).getByRole('listitem').filter({ hasText: title });

async function addTask(page: Page, title: string) {
	await page.getByLabel('New task').fill(title);
	await page.getByLabel('New task').press('Enter');
	await expect(row(page, title)).toBeVisible();
}

async function openTask(page: Page, title: string) {
	await row(page, title).getByRole('button', { name: title, exact: true }).click();
	const detail = page.getByRole('complementary', { name: 'Task details' });
	await expect(detail).toBeVisible();
	return detail;
}

const width = (page: Page, selector: string) =>
	page.locator(selector).evaluate((el) => Math.round(el.getBoundingClientRect().width));

test.describe('on a wide screen', () => {
	test.skip(({ isMobile }) => isMobile, 'a desktop layout');

	test('the sidebar and task panel resize from the keyboard and pointer, and stay so', async ({
		page
	}) => {
		await newProject(page, `Widths ${Date.now()}`);
		await addTask(page, 'Measure twice');
		const sidebar = page.getByRole('separator', { name: 'Resize sidebar' });
		const start = await width(page, 'nav.sidebar');
		await sidebar.focus();
		await sidebar.press('ArrowRight');
		await sidebar.press('Shift+ArrowRight');
		await expect(sidebar).toHaveAttribute('aria-valuenow', String(start + 80));
		await expect.poll(() => width(page, 'nav.sidebar')).toBe(start + 80);
		await sidebar.press('Home');
		await expect(sidebar).toHaveAttribute('aria-valuenow', '180');

		await openTask(page, 'Measure twice');
		await settle(page);
		const panel = page.getByRole('separator', { name: 'Resize task panel' });
		const before = await width(page, '.detail-pane');
		// Dragging the panel's edge left widens it.
		const box = (await panel.boundingBox())!;
		await page.mouse.move(box.x + box.width / 2, box.y + 200);
		await page.mouse.down();
		await page.mouse.move(box.x + box.width / 2 - 60, box.y + 200, { steps: 4 });
		await page.mouse.up();
		await expect.poll(() => width(page, '.detail-pane')).toBe(before + 60);

		// Remembered on this device.
		await page.reload();
		await expect(page.getByRole('complementary', { name: 'Task details' })).toBeVisible();
		await settle(page);
		expect(await width(page, 'nav.sidebar')).toBe(180);
		expect(await width(page, '.detail-pane')).toBe(before + 60);
		await page.evaluate(() => {
			localStorage.removeItem('mila.sidebar.width');
			localStorage.removeItem('mila.detail.width');
		});
	});

	test('in a narrow window the panel edge moves from the width drawn, and every settings tab shows', async ({
		page
	}) => {
		await page.setViewportSize({ width: 960, height: 800 });
		await newProject(page, `Narrow ${Date.now()}`);
		await addTask(page, 'Squeeze in');
		await openTask(page, 'Squeeze in');
		await settle(page);
		// The column leaves the list 360px, so less than the chosen 460 shows.
		const drawn = await width(page, '.detail-pane');
		expect(drawn).toBeLessThan(460);
		const panel = page.getByRole('separator', { name: 'Resize task panel' });
		await expect(panel).toHaveAttribute('aria-valuenow', String(drawn));
		await panel.focus();
		await panel.press('ArrowRight');
		await expect.poll(() => width(page, '.detail-pane')).toBe(drawn - 16);
		await page.evaluate(() => localStorage.removeItem('mila.detail.width'));

		await page.goto('/settings');
		for (const name of ['Import and export', 'Admin'])
			await expect(
				page.getByRole('navigation', { name: 'Settings' }).getByRole('link', { name })
			).toBeInViewport();
	});

	test('a task expands to the full page and back', async ({ page }) => {
		await newProject(page, `Full ${Date.now()}`);
		await addTask(page, 'Read closely');
		const detail = await openTask(page, 'Read closely');
		await detail.getByRole('button', { name: 'Expand to the full page' }).click();
		await expect(page.locator('main')).toBeHidden();
		await expect(page.getByRole('separator', { name: 'Resize task panel' })).toHaveCount(0);

		// From the keyboard, focus follows the task over the hidden list and
		// comes back to its row when it closes.
		await detail.getByRole('button', { name: 'Close' }).click();
		await row(page, 'Read closely').focus();
		await page.keyboard.press('Enter');
		await expect(detail).toBeVisible();
		await expect(page.locator('main')).toBeHidden();
		await expect(detail).toBeFocused();
		await page.keyboard.press('Escape');
		await expect(detail).toBeHidden();
		await expect(row(page, 'Read closely')).toBeFocused();
		await page.keyboard.press('Enter');

		const back = detail.getByRole('button', { name: 'Show beside the list' });
		// Its name says what it does; a pressed state as well would contradict it.
		await expect(back).not.toHaveAttribute('aria-pressed');
		await back.click();
		await expect(page.locator('main')).toBeVisible();
		await expect(detail).toBeVisible();
	});

	test('the task panel leaves at once, and the layout holds still', async ({ page }) => {
		await newProject(page, `Close ${Date.now()}`);
		await addTask(page, 'Shut the door');
		await openTask(page, 'Shut the door');
		await settle(page);
		const frames = await page.evaluate(async () => {
			const sidebar = document.querySelector('nav.sidebar')!;
			const height = () => Math.round(sidebar.getBoundingClientRect().height);
			const before = height();
			document.querySelector<HTMLElement>('#task-panel button[aria-label="Close"]')!.click();
			const seen: { height: number; lingering: boolean }[] = [];
			const end = performance.now() + 500;
			while (performance.now() < end) {
				await new Promise(requestAnimationFrame);
				seen.push({
					height: height(),
					// Still there after the task closed.
					lingering:
						!new URL(location.href).searchParams.has('task') &&
						document.querySelector('#task-panel') !== null
				});
			}
			return { before, seen };
		});
		expect(frames.seen.every((f) => f.height === frames.before)).toBe(true);
		expect(frames.seen.some((f) => f.lingering)).toBe(false);
		await expect(page.locator('#task-panel')).toHaveCount(0);
	});

	test('moving between views animates the page in; opening a task does not', async ({ page }) => {
		await page.addInitScript(() => {
			const w = window as unknown as { arrivals: number };
			w.arrivals = 0;
			const animate = Element.prototype.animate;
			Element.prototype.animate = function (
				this: Element,
				...args: Parameters<Element['animate']>
			) {
				if (this.id === 'main') w.arrivals++;
				return animate.apply(this, args);
			};
		});
		const url = await newProject(page, `Views ${Date.now()}`);
		await addTask(page, 'Look around');
		const count = () => page.evaluate(() => (window as unknown as { arrivals: number }).arrivals);
		const before = await count();
		await openTask(page, 'Look around');
		expect(await count()).toBe(before);
		await page
			.getByRole('navigation', { name: 'Main' })
			.getByRole('link', { name: 'Upcoming' })
			.click();
		await expect(page.getByRole('heading', { name: 'Upcoming' })).toBeVisible();
		await expect.poll(count).toBe(before + 1);
		expect(url).toContain('/projects/');
	});
});

test('menus open under their button and stay on the screen', async ({ page }) => {
	await newProject(page, `Menus ${Date.now()}`);
	await addTask(page, 'Label me');
	const detail = await openTask(page, 'Label me');
	await settle(page);
	const view = page.viewportSize()!;
	for (const [button, menu] of [
		['Edit labels', 'Labels'],
		['Edit assignees', 'Assignees'],
		['Add a reminder', 'Remind me']
	]) {
		const trigger = detail.getByRole('button', { name: button });
		await trigger.scrollIntoViewIfNeeded();
		await trigger.click();
		const open = page.locator(':popover-open');
		await expect(open).toContainText(menu);
		const t = (await trigger.boundingBox())!;
		const m = (await open.boundingBox())!;
		// Just below the button (or just above it, when there is no room below).
		const below = Math.abs(m.y - (t.y + t.height)) <= 8;
		const above = Math.abs(m.y + m.height - t.y) <= 8;
		expect(below || above, `${menu} menu at ${m.y}, button at ${t.y}`).toBe(true);
		expect(m.x).toBeGreaterThanOrEqual(0);
		expect(m.x + m.width).toBeLessThanOrEqual(view.width);
		await page.keyboard.press('Escape');
		await expect(open).toHaveCount(0);
	}
});

test.describe('with reduced motion', () => {
	test.use({ reducedMotion: 'reduce' });

	test('the task panel and rows fade in rather than moving', async ({ page }) => {
		// Every animation the page starts, as it starts it: the ones that
		// move something.
		await page.addInitScript(() => {
			const w = window as unknown as { moves: string[] };
			w.moves = [];
			const animate = Element.prototype.animate;
			Element.prototype.animate = function (keyframes, options) {
				const frames = Array.isArray(keyframes) ? keyframes : [];
				for (const k of frames) {
					const t = String((k as Keyframe).transform ?? 'none');
					if (!/^(none|translate\(0(px)?(, ?0(px)?)?\)|)$/.test(t))
						w.moves.push(`${(this as Element).className}: ${t}`);
				}
				return animate.call(this, keyframes, options);
			};
		});
		await newProject(page, `Calm ${Date.now()}`);
		await addTask(page, 'Hold still');
		await row(page, 'Hold still').getByRole('button', { name: 'Hold still', exact: true }).click();
		await expect(page.getByRole('complementary', { name: 'Task details' })).toBeVisible();
		await settle(page);
		expect(await page.evaluate(() => (window as unknown as { moves: string[] }).moves)).toEqual([]);
		expect(
			await page.evaluate(() =>
				getComputedStyle(document.documentElement).getPropertyValue('--motion').trim()
			)
		).toBe('0');
		// The columns jump to a new width rather than sliding there.
		expect(
			await page.locator('.shell').evaluate((el) => getComputedStyle(el).transitionProperty)
		).toBe('none');
	});
});

test('Mila can be installed: a manifest with icons, and the page color for the bars', async ({
	page,
	request
}) => {
	await page.goto('/');
	const href = await page.locator('link[rel=manifest]').getAttribute('href');
	const manifest = await (await request.get(href!)).json();
	expect(manifest).toMatchObject({ name: 'Mila', display: 'standalone', start_url: '/' });
	const purposes = manifest.icons.map((i: { purpose: string }) => i.purpose);
	expect(purposes).toContain('maskable');
	expect(purposes).toContain('any');
	for (const icon of manifest.icons) {
		const res = await request.get(icon.src);
		expect(res.status(), icon.src).toBe(200);
		expect(res.headers()['content-type']).toBe('image/png');
	}
	const apple = await page.locator('link[rel=apple-touch-icon]').getAttribute('href');
	expect((await request.get(apple!)).status()).toBe(200);
	// The browser's bars take the page's color, whichever theme is shown.
	const colors = () =>
		page
			.locator('meta[name=theme-color]')
			.evaluateAll((metas) => metas.map((m) => (m as HTMLMetaElement).content));
	await expect.poll(colors).toEqual(['#fbfeff', '#fbfeff']);
	await page.emulateMedia({ colorScheme: 'dark' });
	await expect.poll(colors).toEqual(['#101214', '#101214']);
});

test('signing out empties Cache Storage but for the app, and an empty panel offers nothing to mark', async ({
	browser
}) => {
	const { context, page: kim } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Kim');
	await kim.getByRole('button', { name: 'Notifications', exact: true }).click();
	await expect(kim.getByRole('dialog', { name: 'Notifications' })).toContainText('Nothing yet');
	await expect(kim.getByRole('button', { name: 'Mark all read' })).toHaveCount(0);
	await kim.keyboard.press('Escape');

	// The service worker's copy of the app, which holds nobody's data.
	const app = await kim.evaluate(async () => {
		await navigator.serviceWorker.ready;
		return (await caches.keys()).filter((k) => k.startsWith('mila-'));
	});
	expect(app).toHaveLength(1);
	await kim.evaluate(async () => {
		const cache = await caches.open('mila-files');
		await cache.put('/private-file', new Response('a file of Kim’s'));
	});
	await kim.goto('/settings/security');
	await kim
		.getByRole('list', { name: 'Signed-in devices' })
		.getByRole('listitem')
		.filter({ hasText: 'This device' })
		.getByRole('button', { name: 'Sign out' })
		.click();
	await expect(kim).toHaveURL(/\/login/);
	// Everything else goes; the app stays, so the sign-in page opens offline
	// even where no service worker controls the page to fetch it again.
	expect(await kim.evaluate(() => caches.keys())).toEqual(app);
	await context.close();
});

test('finishing a project in Playful throws confetti, unless motion is reduced', async ({
	browser
}) => {
	const { context, page: pat } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Pat');
	const saved = await apiCall(pat, 'PATCH', '/me/settings/preferences', { theme: 'playful' });
	expect(saved.status).toBe(200);
	await pat.reload();
	await expect(pat.locator('html')).toHaveAttribute('data-theme', 'playful');
	await newProject(pat, `Party ${Date.now()}`);
	await addTask(pat, 'First');
	await addTask(pat, 'Last');
	await row(pat, 'First').locator('label.checkbox').click();
	await expect(row(pat, 'First')).toBeHidden();
	await expect(pat.locator('.confetti')).toHaveCount(0);
	await row(pat, 'Last').locator('label.checkbox').click();
	await expect(pat.locator('.confetti')).toHaveCount(1);
	await expect(pat.locator('.confetti')).toHaveAttribute('aria-hidden', 'true');
	// It cleans up after itself.
	await expect(pat.locator('.confetti')).toHaveCount(0, { timeout: 4000 });

	await pat.emulateMedia({ reducedMotion: 'reduce' });
	await addTask(pat, 'Again');
	await row(pat, 'Again').locator('label.checkbox').click();
	await expect(row(pat, 'Again')).toBeHidden();
	await expect(pat.locator('.confetti')).toHaveCount(0);
	await context.close();
});

test('the schedule chooser works from the keyboard and passes axe', async ({ page }) => {
	await newProject(page, `Keys ${Date.now()}`);
	await addTask(page, 'Plan the week');
	const button = row(page, 'Plan the week').getByRole('button', { name: 'Schedule Plan the week' });
	await button.focus();
	await page.keyboard.press('Enter');
	const chooser = page.getByRole('dialog', { name: 'Schedule Plan the week' });
	await expect(chooser).toBeVisible();
	await expect(chooser.getByRole('button', { name: /^Today/ })).toBeFocused();
	await axe(page);
	await page.keyboard.press('ArrowDown');
	await expect(chooser.getByRole('button', { name: /^Tomorrow/ })).toBeFocused();
	await page.keyboard.press('Enter');
	await expect(chooser).toBeHidden();
	// Back on the row's button, which now shows the date.
	await expect(
		row(page, 'Plan the week').getByRole('button', { name: /^Due .*, schedule Plan the week/ })
	).toBeFocused();
	// Escape closes it without a change.
	await row(page, 'Plan the week').getByRole('button', { name: /^Due / }).click();
	await expect(chooser).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(chooser).toBeHidden();
	await expect(row(page, 'Plan the week').getByRole('button', { name: /^Due / })).toBeFocused();

	// Also when a new date moves the row to another day in Upcoming, which
	// draws it again there. Upcoming shows the whole workspace, so the
	// button is found by its task.
	const id = await row(page, 'Plan the week').getAttribute('data-task-id');
	await page
		.getByRole('navigation', { name: 'Main' })
		.getByRole('link', { name: 'Upcoming' })
		.click();
	await expect(page.getByRole('heading', { name: 'Upcoming', level: 1 })).toBeVisible();
	await settle(page);
	const due = page.locator(`[data-schedule="${id}"]`);
	const labels = () => due.evaluateAll((els) => els.map((e) => e.getAttribute('aria-label')));
	const before = await labels();
	await due.focus();
	await page.keyboard.press('Enter');
	await expect(chooser).toBeVisible();
	// The date field keeps its own arrow keys.
	await chooser.locator('input[type=date]').focus();
	await page.keyboard.press('ArrowUp');
	await expect(chooser.locator('input[type=date]')).toBeFocused();
	await chooser.locator('input[type=date]').fill(inDays(9));
	await chooser.locator('input[type=date]').press('Enter');
	await expect(chooser).toBeHidden();
	// Once the row it left has gone.
	await expect.poll(labels).not.toEqual(before);
	await expect.poll(labels).toHaveLength(1);
	await expect(due).toBeFocused();
});

/** The local date some days from now, as YYYY-MM-DD. */
function inDays(n: number): string {
	const d = new Date();
	d.setDate(d.getDate() + n);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

test('names at their longest shorten in place and leave the controls beside them alone', async ({
	page,
	isMobile
}) => {
	// A workspace of its own, so the long view name shows in no other test's sidebar.
	await page.goto('/');
	const ws: string = (
		await apiCall(page, 'POST', '/workspaces', { name: `Long names ${Date.now()}` })
	).body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', {
		name: 'P'.repeat(200),
		description: 'A long description. '.repeat(100)
	});
	const status = await make('statuses', {
		project_id: project,
		name: 'S'.repeat(60),
		category: 'todo'
	});
	const parent = await make('tasks', {
		project_id: project,
		status_id: status,
		title: 'T'.repeat(500)
	});
	const subtask = await make('tasks', {
		project_id: project,
		status_id: status,
		parent_id: parent,
		title: 'A subtask'
	});
	await make('saved-views', { name: 'V'.repeat(80), type: 'list', config: { filter: {} } });

	const box = async (locator: ReturnType<Page['locator']>) => (await locator.boundingBox())!;
	const detail = page.getByRole('complementary', { name: 'Task details' });

	// The project's name in the panel's header stops short of its buttons,
	// and the status fits the panel.
	await page.goto(`/projects/${project}?ws=${ws}&task=${parent}`);
	await expect(detail.getByLabel('Status')).toBeVisible();
	const select = await box(detail.getByLabel('Project'));
	const remove = await box(detail.getByRole('button', { name: 'Delete task' }));
	expect(select.x + select.width).toBeLessThanOrEqual(remove.x);
	const panel = await box(detail);
	const statusBox = await box(detail.getByLabel('Status'));
	expect(statusBox.x + statusBox.width).toBeLessThanOrEqual(panel.x + panel.width);

	// So does a subtask's link to its parent.
	await page.goto(`/projects/${project}?ws=${ws}&task=${subtask}`);
	const crumb = detail.getByRole('button', { name: 'T'.repeat(500) });
	await expect(crumb).toBeVisible();
	const link = await box(crumb);
	expect(link.x + link.width).toBeLessThanOrEqual(
		(await box(detail.getByRole('button', { name: 'Delete task' }))).x
	);

	// The description keeps to a few lines until it is edited.
	await page.goto(`/projects/${project}?ws=${ws}`);
	const description = page.getByLabel('Project description');
	await expect(description).toBeVisible();
	const resting = (await box(description)).height;
	expect(resting).toBeLessThan(120);
	await description.focus();
	await expect.poll(async () => (await box(description)).height).toBeGreaterThan(resting);

	// The sidebar shortens a view's name rather than running past its edge.
	if (isMobile) return;
	const sidebar = await box(page.getByRole('navigation', { name: 'Main' }));
	const view = page.getByRole('link', { name: 'V'.repeat(80) });
	await expect(view).toBeVisible();
	const viewBox = await box(view);
	expect(viewBox.x + viewBox.width).toBeLessThanOrEqual(sidebar.x + sidebar.width);
	const name = view.locator('.name');
	expect(await name.evaluate((el) => el.scrollWidth > el.clientWidth)).toBe(true);
});

test('the collapsed sidebar keeps its buttons on one line and at one size', async ({
	page,
	isMobile
}) => {
	test.skip(isMobile, 'phones have no sidebar');
	await page.goto('/');
	const ws = await page.evaluate(() => localStorage.getItem('mila.workspace'));
	if (ws) await apiCall(page, 'POST', `/workspaces/${ws}/projects`, { name: 'On the line' });
	await page.getByRole('button', { name: 'Collapse sidebar' }).click();
	await expect(page.getByRole('button', { name: 'Expand sidebar' })).toBeVisible();
	await settle(page);

	const marks = await page.evaluate(() => {
		const sidebar = document.querySelector('.sidebar-body')!.getBoundingClientRect();
		const middle = (sidebar.left + sidebar.right) / 2;
		const seen = [
			...document.querySelectorAll('.sidebar a svg, .sidebar button svg, .sidebar .sync .dot')
		];
		return seen
			.map((el) => {
				const box = el.getBoundingClientRect();
				const owner = el.closest('a, button, .sync') as HTMLElement;
				return {
					name: owner.getAttribute('aria-label') ?? owner.title ?? owner.textContent,
					off: Math.abs((box.left + box.right) / 2 - middle),
					width: box.width,
					dot: el.matches('.dot')
				};
			})
			.filter((m) => m.width > 0);
	});
	expect(marks.length).toBeGreaterThan(10);
	expect(marks.filter((m) => m.off > 1)).toEqual([]);
	expect(marks.filter((m) => !m.dot && m.width < 16)).toEqual([]);
});

test("a project's and a view's name keep their room beside the task panel", async ({
	page,
	isMobile
}) => {
	test.skip(isMobile, 'a phone shows the panel as a sheet');
	test.slow();
	await page.goto('/');
	const name = `Header ${Date.now()}`;
	const ws: string = (await apiCall(page, 'POST', '/workspaces', { name })).body.id;
	const make = async (entity: string, body: unknown) =>
		(await apiCall(page, 'POST', `/workspaces/${ws}/${entity}`, body)).body.id as string;
	const project = await make('projects', { name: 'Wholesale accounts' });
	const status = await make('statuses', { project_id: project, name: 'To do', category: 'todo' });
	const task = await make('tasks', {
		title: 'Repaint',
		project_id: project,
		status_id: status,
		sort_key: 'a0'
	});
	const view = await make('saved-views', {
		name: 'This week',
		type: 'list',
		config: {},
		sort_key: 'a0'
	});
	const screens = [
		[`/projects/${project}?ws=${ws}&tab=tasks&task=${task}`, 'Project name'],
		[`/views/${view}?ws=${ws}&task=${task}`, 'View name']
	] as const;
	const seen: string[] = [];
	for (const [url, label] of screens) {
		await page.goto(url);
		const field = page.getByLabel(label, { exact: true });
		await expect(field).toBeVisible();
		await expect(page.getByRole('complementary', { name: 'Task details' })).toBeVisible();
		for (const width of [1100, 1200, 1300, 1400, 1500, 1600, 1800, 2200]) {
			await page.setViewportSize({ width, height: 800 });
			await settle(page);
			const room = Math.round((await field.boundingBox())!.width);
			if (room < 120) seen.push(`${label} at ${width}: ${room} px`);
			// And nothing in the header runs off its edge.
			const over = await page
				.locator('#main header.page')
				.first()
				.evaluate((el) => el.scrollWidth - el.clientWidth);
			if (over > 1) seen.push(`${label} at ${width}: ${over} px over`);
		}
	}
	expect(seen).toEqual([]);
});

test('the lists of views and projects fold away, and the device remembers', async ({
	browser,
	isMobile
}) => {
	test.skip(isMobile, 'a phone has its own lists, without a sidebar');
	const { page, context } = await newPerson(() => browser.newContext(SIGNED_OUT), 'Finn');
	const name = `Folded ${Date.now()}`;
	await newProject(page, name);
	const nav = page.getByRole('navigation', { name: 'Main' });
	const link = nav.getByRole('link', { name });
	const fold = nav.getByRole('button', { name: 'Projects', exact: true });
	await expect(link).toBeVisible();
	await expect(fold).toHaveAttribute('aria-expanded', 'true');

	await fold.click();
	await expect(fold).toHaveAttribute('aria-expanded', 'false');
	await expect(link).toHaveCount(0);
	await page.reload();
	await expect(fold).toHaveAttribute('aria-expanded', 'false');
	await expect(link).toHaveCount(0);

	// Making a project opens the list it goes in.
	await nav.getByRole('button', { name: 'New project' }).click();
	await expect(link).toBeVisible();
	await expect(fold).toHaveAttribute('aria-expanded', 'true');
	await expect(nav.getByLabel('New project name')).toBeFocused();
	await page.keyboard.press('Escape');
	await expect(nav.getByLabel('New project name')).toHaveCount(0);

	// Folded from the keyboard, and a collapsed sidebar still shows its lists.
	await fold.focus();
	await page.keyboard.press('Enter');
	await expect(link).toHaveCount(0);
	await page.getByRole('button', { name: 'Collapse sidebar' }).click();
	await expect(nav.getByRole('link', { name })).toBeVisible();
	await context.close();
});
