// The phone: the tab bar, swipes, the task sheet and touch target sizes.
// Playwright's touchscreen only taps, so swipes are pointer events with
// pointerType "touch", which is what swipe.ts listens for.
import { expect, test, type Locator, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { settle } from './axe';
import { OWNER, pngBuffer, settingsWorkspace } from './helpers';

test.use({ storageState: OWNER });
test.skip(({ isMobile }) => !isMobile, 'phone only');

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

/** Drags a finger across an element: from its middle (or near its top) by dx, dy. */
async function drag(target: Locator, dx: number, dy = 0) {
	// A row still sliding in would put the finger beside it.
	await settle(target.page());
	await target.evaluate(
		(el, { dx, dy }) => {
			const r = el.getBoundingClientRect();
			const x = r.left + r.width / 2;
			const y = r.top + Math.min(r.height / 2, 24);
			// What a finger there would touch.
			const target = document.elementFromPoint(x, y) ?? el;
			const fire = (type: string, i: number) =>
				target.dispatchEvent(
					new PointerEvent(type, {
						pointerId: 7,
						pointerType: 'touch',
						isPrimary: true,
						bubbles: true,
						cancelable: true,
						clientX: x + (dx * i) / 10,
						clientY: y + (dy * i) / 10
					})
				);
			fire('pointerdown', 0);
			for (let i = 1; i <= 10; i++) fire('pointermove', i);
			fire('pointerup', 10);
		},
		{ dx, dy }
	);
}

test('the tab bar has Search, and Settings is behind the gear', async ({ page }) => {
	await page.goto('/');
	const tabs = page.locator('nav.tabs');
	await expect(tabs.getByRole('link')).toHaveText(['Inbox', 'My Day', 'Upcoming', 'Projects']);
	await tabs.getByRole('button', { name: 'Search' }).click();
	const palette = page.getByRole('dialog', { name: 'Command palette' });
	await expect(palette).toBeVisible();
	// The whole screen, like the other tabs.
	const box = await palette.boundingBox();
	expect(box?.height).toBe(page.viewportSize()?.height);
	await palette.getByRole('combobox').fill('upcoming');
	await palette
		.getByRole('option', { name: /Upcoming/ })
		.first()
		.click();
	await expect(page).toHaveURL(/\/upcoming$/);
	// A phone has no Escape key: a button closes it.
	await tabs.getByRole('button', { name: 'Search' }).click();
	await expect(palette).toBeVisible();
	await palette.getByRole('button', { name: 'Close search' }).click();
	await expect(palette).toBeHidden();

	await page.getByRole('link', { name: 'Settings' }).click();
	await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Settings' })).toHaveAttribute(
		'aria-current',
		'page'
	);
});

test('swiping right completes a task, with an undo', async ({ page }) => {
	await newProject(page, `Swipe ${Date.now()}`);
	await addTask(page, 'Rake the leaves');
	await drag(row(page, 'Rake the leaves'), 220);
	// Held a moment, struck through, then gone into Completed.
	await expect(page.getByText('Completed "Rake the leaves"')).toBeVisible();
	await expect(row(page, 'Rake the leaves')).toBeHidden();
	await expect(page.getByRole('button', { name: /Completed · 1/ })).toBeVisible();
	await page.getByRole('button', { name: 'Undo' }).click();
	await expect(row(page, 'Rake the leaves')).toBeVisible();
	await expect(page.getByRole('button', { name: /Completed · / })).toBeHidden();
});

test("a swipe's undo takes back that swipe, not a later one", async ({ page }) => {
	await newProject(page, `Two swipes ${Date.now()}`);
	await addTask(page, 'Wash the car');
	await addTask(page, 'Walk the dog');
	await drag(row(page, 'Wash the car'), 220);
	await expect(row(page, 'Wash the car')).toBeHidden();
	await drag(row(page, 'Walk the dog'), 220);
	await expect(row(page, 'Walk the dog')).toBeHidden();
	// Both toasts are up; the first one's undo reopens only its own task.
	await page
		.locator('.toaster .item')
		.filter({ hasText: 'Completed "Wash the car"' })
		.getByRole('button', { name: 'Undo' })
		.click();
	await expect(row(page, 'Wash the car')).toBeVisible();
	await expect(row(page, 'Wash the car').getByRole('checkbox')).not.toBeChecked();
	await expect(row(page, 'Walk the dog')).toBeHidden();
	await expect(page.getByRole('button', { name: /Completed · 1/ })).toBeVisible();
});

test('undoing a swipe during the hold keeps the task open', async ({ page }) => {
	await newProject(page, `Hold ${Date.now()}`);
	await addTask(page, 'Sweep the path');
	await drag(row(page, 'Sweep the path'), 220);
	await page.getByRole('button', { name: 'Undo' }).click();
	await page.waitForTimeout(1300);
	await expect(row(page, 'Sweep the path')).toBeVisible();
	await expect(row(page, 'Sweep the path').getByRole('checkbox')).not.toBeChecked();
});

test('a short or vertical swipe does nothing', async ({ page }) => {
	await newProject(page, `Short ${Date.now()}`);
	await addTask(page, 'Stay put');
	await drag(row(page, 'Stay put'), 30);
	await drag(row(page, 'Stay put'), 40, 120);
	await page.waitForTimeout(1300);
	await expect(row(page, 'Stay put')).toBeVisible();
	await expect(page.getByRole('dialog', { name: /Schedule/ })).toBeHidden();
});

test('swiping left schedules, as the schedule button does', async ({ page }) => {
	await newProject(page, `Schedule ${Date.now()}`);
	await addTask(page, 'Mow the lawn');
	await drag(row(page, 'Mow the lawn'), -220);
	const sheet = page.getByRole('dialog', { name: 'Schedule Mow the lawn' });
	await expect(sheet).toBeVisible();
	await settle(page);
	// A sheet along the bottom of the screen.
	const box = await sheet.boundingBox();
	const view = page.viewportSize()!;
	expect(Math.round(box!.y + box!.height)).toBe(view.height);
	expect(box!.width).toBe(view.width);
	await sheet.getByRole('button', { name: /^Tomorrow/ }).click();
	await expect(sheet).toBeHidden();
	await expect(page.getByText('Due tomorrow')).toBeVisible();
	const due = row(page, 'Mow the lawn').getByRole('button', { name: /^Due .*, schedule Mow/ });
	await expect(due).toBeVisible();

	// The visible way, without a gesture: the due date is a button.
	await due.click();
	await sheet.getByRole('button', { name: /^No date/ }).click();
	await expect(page.getByText('Due date cleared')).toBeVisible();
	await expect(
		row(page, 'Mow the lawn').getByRole('button', { name: 'Schedule Mow the lawn' })
	).toBeVisible();
});

test('the task sheet fills the screen and pulls down to close', async ({ page }) => {
	await newProject(page, `Sheet ${Date.now()}`);
	await addTask(page, 'Paint the fence');
	await row(page, 'Paint the fence')
		.getByRole('button', { name: 'Paint the fence', exact: true })
		.click();
	const detail = page.getByRole('complementary', { name: 'Task details' });
	await expect(detail).toBeVisible();
	await expect(page).toHaveURL(/task=/);
	await settle(page);
	const box = await page.locator('.detail-pane').boundingBox();
	const view = page.viewportSize()!;
	expect(box).toEqual({ x: 0, y: 0, width: view.width, height: view.height });
	// The phone has no room for a full page mode; the sheet already is one.
	await expect(detail.getByRole('button', { name: /Expand/ })).toHaveCount(0);

	// A short pull springs back.
	await drag(detail.locator('header[data-grab]'), 0, 30);
	await expect(detail).toBeVisible();
	await drag(detail.locator('header[data-grab]'), 0, 400);
	await expect(detail).toBeHidden();
	await expect(page).not.toHaveURL(/task=/);
});

/**
 * Every control on screen that a finger can hit over less than 44px: its
 * middle, and 20px from it each way, must land on the control (or on the
 * edge of the scrolling area it is in, if it runs past it). Inline links
 * in text are exempt (WCAG 2.5.8), and so is anything a fixed button (the
 * FAB, a toast) happens to cover at this scroll position.
 */
async function smallTargets(page: Page): Promise<string[]> {
	await settle(page);
	return page.evaluate(() => {
		const R = 20;
		const out: string[] = [];
		const selector = [
			'a[href]',
			'button',
			'select',
			'textarea',
			'input:not([type=hidden])',
			'[role=button]',
			'[role=option]',
			'[role=separator][tabindex]',
			'[tabindex="0"]'
		].join(',');
		// A bar that stays put (fixed, or stuck while the page scrolls under it).
		const fixedOver = (hit: Element, el: Element) => {
			for (let n: Element | null = hit; n; n = n.parentElement)
				if (['fixed', 'sticky'].includes(getComputedStyle(n).position)) return !n.contains(el);
			return false;
		};
		for (let el of document.querySelectorAll<HTMLElement>(selector)) {
			// A hidden checkbox or radio is hit through its label, and so is a
			// box drawn small inside one: a tap anywhere on the label ticks it.
			if (el.matches('.visually-hidden')) {
				const label = el.closest('label');
				if (!label) continue;
				el = label;
			} else if (el.matches('input[type=checkbox], input[type=radio]') && el.closest('label')) {
				el = el.closest('label')!;
			}
			// A picture of controls (a theme preview) is no target.
			if (el.closest('[inert]')) continue;
			// Inside a closed disclosure: Chrome still gives it a box, unseen.
			const shut = el.closest('details:not([open])');
			if (shut && !el.closest('summary')) continue;
			const r = el.getBoundingClientRect();
			if (!r.width || !r.height) continue;
			const style = getComputedStyle(el);
			if (style.visibility === 'hidden' || Number(style.opacity) === 0) continue;
			if (el.matches('a') && style.display === 'inline') continue;
			// A row that is focusable for the keyboard is as big as the row.
			if (el.matches('[role=listitem]')) continue;
			// What can be seen of it: the screen, cut by every scrolling box it is in.
			let area = { top: 0, left: 0, bottom: innerHeight, right: innerWidth };
			for (let p = el.parentElement; p; p = p.parentElement) {
				if (getComputedStyle(p).overflow === 'visible') continue;
				const b = p.getBoundingClientRect();
				area = {
					top: Math.max(area.top, b.top),
					left: Math.max(area.left, b.left),
					bottom: Math.min(area.bottom, b.bottom),
					right: Math.min(area.right, b.right)
				};
			}
			const cx = r.left + r.width / 2;
			const cy = r.top + r.height / 2;
			if (cx < area.left || cx >= area.right || cy < area.top || cy >= area.bottom) continue;
			// A finger at the edge of what is visible still lands on it.
			const at = (x: number, y: number) =>
				document.elementFromPoint(
					Math.min(area.right - 1, Math.max(area.left, x)),
					Math.min(area.bottom - 1, Math.max(area.top, y))
				);
			const middle = at(cx, cy);
			if (middle && !el.contains(middle) && fixedOver(middle, el)) continue;
			const misses = [
				[cx, cy],
				[cx - R, cy],
				[cx + R, cy],
				[cx, cy - R],
				[cx, cy + R]
			].filter(([x, y]) => {
				const hit = at(x, y);
				return !hit || (!el.contains(hit) && !fixedOver(hit, el));
			});
			if (misses.length) {
				const name =
					el.getAttribute('aria-label') ||
					el.textContent?.trim().slice(0, 30) ||
					el.getAttribute('placeholder') ||
					el.tagName;
				out.push(
					`${el.tagName.toLowerCase()}.${[...el.classList].join('.')} "${name}" ${Math.round(r.width)}x${Math.round(r.height)}`
				);
			}
		}
		return out;
	});
}

test('controls are big enough to tap', async ({ page }) => {
	// Every main screen in turn, so it takes longer than most.
	test.slow();
	const ws = await settingsWorkspace(page);
	const project = `Targets ${Date.now()}`;
	const url = await newProject(page, project);
	await addTask(page, 'A task to tap');
	await row(page, 'A task to tap').getByRole('button', { name: 'Schedule A task to tap' }).click();
	await page
		.getByRole('dialog', { name: /Schedule/ })
		.getByRole('button', { name: /^Today/ })
		.click();
	const screens: [string, () => Promise<unknown>][] = [
		['project', async () => {}],
		['board', () => page.goto(url + '?view=board')],
		['calendar', () => page.goto(url + '?view=calendar')],
		['table', () => page.goto(url + '?view=table')],
		['timeline', () => page.goto(url + '?view=timeline')],
		['templates', () => page.goto('/settings/templates')],
		['time', () => page.goto('/time')],
		['reports', () => page.goto('/reports')],
		['project wiki', () => page.goto(url + '?tab=wiki')],
		['project time', () => page.goto(url + '?tab=time')],
		['wiki', () => page.goto('/wiki')],
		[
			'wiki pages',
			async () => {
				await page.getByRole('button', { name: 'Show pages' }).click();
				await expect(page.getByRole('dialog', { name: 'Wiki pages' })).toBeVisible();
			}
		],
		[
			'project time log',
			async () => {
				await page.goto(url + '?tab=time');
				const panel = page.getByRole('tabpanel', { name: 'Time' });
				await panel.getByRole('button', { name: 'Log time' }).click();
				await expect(panel.getByRole('form', { name: `Log time to ${project}` })).toBeVisible();
			}
		],
		[
			'time log',
			async () => {
				await page.goto('/time');
				await page.getByRole('button', { name: 'Log time', exact: true }).click();
				const form = page.getByRole('form', { name: 'Log time to a project' });
				await form.getByLabel('Project').selectOption({ label: project });
				await form.getByLabel('Time spent').fill('1h');
			}
		],
		[
			'time by week',
			async () => {
				const form = page.getByRole('form', { name: 'Log time to a project' });
				await form.getByRole('button', { name: 'Log it' }).click();
				await expect(form).toHaveCount(0);
				await page
					.getByRole('combobox', { name: 'Period', exact: true })
					.selectOption({ label: 'Last 8 weeks' });
				await expect(page.getByRole('group', { name: /^Hours logged each week by/ })).toBeVisible();
			}
		],
		['inbox', () => page.goto('/')],
		['my day', () => page.goto('/today')],
		['upcoming', () => page.goto('/upcoming')],
		...(
			[
				['inbox board', '/'],
				['my day board', '/today'],
				['upcoming board', '/upcoming']
			] as const
		).map(
			([name, path]) =>
				[
					name,
					async () => {
						await page.goto(path);
						await page
							.getByRole('group', { name: 'Show as' })
							.getByRole('button', { name: 'Board' })
							.click();
						await expect(page.getByRole('group', { name: 'Board' })).toBeVisible();
					}
				] as [string, () => Promise<unknown>]
		),
		['projects', () => page.goto('/projects')],
		['settings', () => page.goto('/settings')],
		['profile', () => page.goto('/settings/profile')],
		[
			'picture crop',
			async () => {
				await page.getByLabel('Upload picture').setInputFiles({
					name: 'me.png',
					mimeType: 'image/png',
					buffer: pngBuffer(300, 200)
				});
				await expect(page.getByRole('dialog', { name: 'Crop your picture' })).toBeVisible();
			}
		],
		['preferences', () => page.goto('/settings/preferences')],
		['notifications', () => page.goto('/settings/notifications')],
		...['general', 'people', 'labels', 'templates', 'themes', 'webhooks', 'calendar'].map(
			(p) =>
				[`workspace ${p}`, () => page.goto(`/settings/w/${ws}/${p}`)] as [
					string,
					() => Promise<unknown>
				]
		),
		['appearance', () => page.goto('/settings/appearance')],
		[
			'theme editor',
			async () => {
				await page.getByRole('button', { name: 'New theme', exact: true }).click();
				await expect(page.getByLabel('Theme name')).toBeVisible();
			}
		],
		[
			'theme group',
			async () => {
				await page.getByRole('button', { name: /^Surfaces/ }).click();
				await expect(page.getByRole('heading', { name: 'Surfaces' })).toBeVisible();
			}
		],
		[
			'theme color',
			async () => {
				await page.getByRole('button', { name: /^Page:/ }).click();
				await expect(page.getByLabel('Hex', { exact: true })).toBeVisible();
			}
		],
		[
			'theme preview',
			async () => {
				await page.keyboard.press('Escape');
				await expect(page.getByLabel('Hex', { exact: true })).toBeHidden();
				await page.getByText('Whole app', { exact: true }).click();
				await expect(page.locator('.theme-preview .board')).toBeVisible();
			}
		],
		[
			'theme search',
			async () => {
				await page.getByLabel('Find a setting').fill('heading');
				await expect(page.getByRole('button', { name: /^Heading 1:/ })).toBeVisible();
			}
		],
		[
			'theme menu',
			async () => {
				await page.goto('/settings/themes');
				await page
					.getByRole('list', { name: 'Your themes' })
					.getByRole('button', { name: /^More for / })
					.first()
					.click();
				await expect(page.getByRole('button', { name: 'Export file' }).first()).toBeVisible();
			}
		],
		[
			'theme import',
			async () => {
				await page.keyboard.press('Escape');
				await page.locator('#theme-file').setInputFiles({
					name: 'sample.mila-theme.json',
					mimeType: 'application/json',
					buffer: readFileSync(new URL('./visual/fixtures/sample.mila-theme.json', import.meta.url))
				});
				await expect(page.getByRole('dialog', { name: /^Import Harbor/ })).toBeVisible();
			}
		],
		['keyboard', () => page.goto('/settings/keyboard')],
		[
			'saved view',
			async () => {
				await page.goto(url);
				await page.getByRole('button', { name: 'Save as a view' }).click();
				await expect(page.getByLabel('View name')).toBeVisible();
				await page.getByRole('button', { name: 'Filter', exact: true }).click();
				await page.getByRole('button', { name: 'Display' }).click();
				await page
					.getByRole('group', { name: 'Display' })
					.getByRole('button', { name: 'Add a sort' })
					.click();
				await expect(page.getByLabel('Direction of Title')).toBeVisible();
			}
		],
		['help', () => page.goto('/help')],
		['help topic', () => page.goto('/help/touch-gestures')],
		['shortcut reference', () => page.goto('/help/shortcuts')],
		['your data', () => page.goto('/settings/data')],
		['email to task', () => page.goto('/settings/email')],
		['automations', () => page.goto('/settings/automations')],
		[
			'api reference',
			async () => {
				await page.goto('/api/docs');
				const nav = page.getByRole('navigation', { name: 'Endpoints' });
				await nav.locator('summary').click();
				await nav.getByRole('link', { name: /Make a task$/ }).click();
				await expect(
					page.locator('#op-createTask').getByRole('button', { name: 'Send POST' })
				).toBeVisible();
			}
		],
		['security', () => page.goto('/settings/security')],
		['admin', () => page.goto('/settings/admin')],
		[
			'csv import',
			async () => {
				await page.goto('/settings/data/csv');
				await page.getByLabel('CSV file').setInputFiles({
					name: 'taps.csv',
					mimeType: 'text/csv',
					buffer: Buffer.from('Title,Due date\nTap me,2026-11-03\n')
				});
				await page.getByRole('button', { name: 'Check the import' }).click();
				await expect(page.getByText('1 task would come in')).toBeVisible();
			}
		],
		[
			'github import',
			async () => {
				await page.goto('/settings/data/apps');
				await page.getByLabel('Issues file (JSON)').setInputFiles({
					name: 'issues.json',
					mimeType: 'application/json',
					buffer: Buffer.from(
						JSON.stringify([
							{
								number: 1,
								title: 'Tap me',
								state: 'OPEN',
								url: 'https://github.com/acme/taps/issues/1'
							}
						])
					)
				});
				await expect(page.getByText('1 task would come in')).toBeVisible();
			}
		],
		[
			'clickup import',
			async () => {
				await page.goto('/settings/data/apps');
				await page.getByRole('button', { name: 'ClickUp', exact: true }).click();
				await page.getByLabel('ClickUp file (CSV)').setInputFiles({
					name: 'export.csv',
					mimeType: 'text/csv',
					buffer: Buffer.from(
						'Task ID,Task Type,Task Name,Status,List Name\n86tap,Task,Tap me,to do,Taps\n'
					)
				});
				await page.getByRole('button', { name: 'Check the import' }).click();
				await expect(page.getByText('1 task would come in')).toBeVisible();
			}
		],
		[
			'task',
			async () => {
				await page.goto(url);
				await row(page, 'A task to tap')
					.getByRole('button', { name: 'A task to tap', exact: true })
					.click();
			}
		],
		[
			'schedule',
			async () => {
				await page.goto(url);
				await row(page, 'A task to tap').getByRole('button', { name: /^Due / }).click();
			}
		]
	];
	const found: string[] = [];
	for (const [name, show] of screens) {
		await show();
		await page.waitForLoadState('networkidle');
		for (const t of await smallTargets(page)) found.push(`${name}: ${t}`);
	}
	expect(found).toEqual([]);
});

test('the task sheet holds focus, and the page under it is out of reach', async ({ page }) => {
	await newProject(page, `Sheet ${Date.now()}`);
	await addTask(page, 'Covered up');
	await row(page, 'Covered up').getByRole('button', { name: 'Covered up', exact: true }).click();
	const sheet = page.getByRole('complementary', { name: 'Task details' });
	await expect(sheet).toBeFocused();
	await settle(page);
	// What the sheet covers takes no focus and is hidden from screen readers.
	for (const under of ['#main', 'nav.tabs', 'a.skip'])
		await expect(page.locator(under)).toHaveAttribute('inert', '');
	// Tab goes round the sheet's own controls, never behind it.
	for (let i = 0; i < 40; i++) {
		await page.keyboard.press('Tab');
		const inSheet = await page.evaluate(
			() =>
				!!document.activeElement?.closest('#task-panel') ||
				document.activeElement === document.body ||
				!!document.activeElement?.closest('.toaster')
		);
		expect(inSheet, `stop ${i}`).toBe(true);
	}
	await sheet.getByRole('button', { name: 'Close' }).click();
	await expect(sheet).toBeHidden();
	await expect(page.locator('#main')).not.toHaveAttribute('inert');
	await expect(row(page, 'Covered up')).toBeFocused();
});

test('a setting found from the preview opens below it, in view', async ({ page }) => {
	await page.goto('/settings/themes');
	await page.getByRole('button', { name: 'New theme', exact: true }).click();
	await page
		.getByRole('navigation', { name: 'Parts of the theme' })
		.getByRole('button', { name: /^Pages/ })
		.click();
	const heading = page.locator('.theme-preview h1').first();
	await expect(heading).toHaveText('Garden plan');
	await settle(page);
	// A tap on the preview's heading opens its color, clear of the preview over it.
	const box = (await heading.boundingBox())!;
	await page.mouse.click(box.x + 10, box.y + box.height / 2);
	await expect(page.getByRole('button', { name: /^Heading 1:/ })).toHaveAttribute(
		'aria-expanded',
		'true'
	);
	await expect(page.getByRole('button', { name: /^Heading 1:/ })).toBeInViewport();
	await expect(page.getByRole('button', { name: /^Heading 1:/ })).toBeFocused();
});
