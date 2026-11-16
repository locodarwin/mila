// A small workspace for the style guide's board, calendar, filter panel,
// field inputs, view list, quick add, palette and wiki. It lives in memory only
// (engine.sample): what is done to it on the page is never saved.
import { DEFAULTS, pairId, type Entity, type FieldOption } from '#lib/sync/entities.js';
import { mainPageId } from '#lib/sync/pages.js';
import type { Row } from '#lib/sync/replica.svelte.js';

export const SAMPLE_WS = 'style-guide';
export const SAMPLE_ME = 'u-sample';
export const SAMPLE_PROJECT = 'p-garden';
/** The Garden wiki's main page. */
export const GARDEN_WIKI = mainPageId(SAMPLE_PROJECT);
/** A page under it, linked to from it, as the app's links need an ID of this shape. */
export const PLANTING = '5a1e0c3e-2b7d-4f6a-9c1e-3d8b2a7f4e10';
const pageLink = (id: string) => `/projects/${SAMPLE_PROJECT}?tab=wiki&page=${id}`;

/** A date some days from today, as tasks keep them. */
function day(offset: number): string {
	const d = new Date();
	d.setDate(d.getDate() + offset);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** A moment some days from now, at an hour of that day, as the server keeps them. */
function at(offset: number, hour = 12): string {
	const d = new Date();
	d.setDate(d.getDate() + offset);
	d.setHours(hour, 0, 0, 0);
	return d.toISOString();
}

const row = (entity: Entity, id: string, values: Record<string, unknown>): Row => ({
	entity,
	id,
	values: { ...DEFAULTS[entity], ...values },
	clocks: {}
});

export const SIZES: FieldOption[] = [
	{ id: 'o-small', name: 'Small', color: 'green' },
	{ id: 'o-large', name: 'Large', color: 'purple' }
];

/** A template's task with nothing set, for the sample templates. */
function tplTask(id: string, title: string) {
	return {
		id,
		title,
		parent: null,
		notes: '',
		priority: 'none',
		status: null,
		category: null,
		start: null,
		due: null,
		due_time: null,
		recurrence: null,
		recurrence_mode: 'schedule',
		labels: [],
		values: {},
		checklist: [],
		waits: []
	};
}

/**
 * The Kitchen's six weeks of work, for the Reports page's charts: each task
 * made, started (or never) and done on days before today, and the open ones
 * with their estimates and people. They have no due dates, so the calendar
 * and lists of the other sections are as they were.
 */
function kitchenRows(): Row[] {
	const P = 'p-kitchen';
	const statuses = [
		['s-k-backlog', 'Someday', 'backlog'],
		['s-k-todo', 'To do', 'todo'],
		['s-k-cooking', 'Cooking', 'in-progress'],
		['s-k-done', 'Done', 'done']
	] as const;
	// Title, made, started (or null), done (or the open status), people, estimate.
	const work: [string, number, number | null, number | string, string[], number | null][] = [
		['Clear the old cabinets', -40, -38, -36, ['u-sam'], null],
		['Measure the walls', -40, -39, -39, [SAMPLE_ME], null],
		['Pick a countertop', -38, -33, -27, ['u-dana'], null],
		['Order the tiles', -35, -34, -32, ['u-lee'], null],
		['Book the electrician', -33, null, -30, [SAMPLE_ME], null],
		['Patch the plaster', -30, -26, -22, ['u-sam'], null],
		['Sand the floor', -28, -24, -20, ['u-sam'], null],
		['Paint the ceiling', -26, -21, -17, ['u-lee'], null],
		['Fit the sink', -24, -15, -6, ['u-dana'], null],
		['Hang the shelves', -21, -14, -13, [SAMPLE_ME], null],
		['Seal the grout', -18, -10, -9, ['u-lee'], null],
		['Wire the lights', -16, -12, -4, ['u-sam'], null],
		['Return the spare tiles', -12, null, -10, ['u-dana'], null],
		['Prime the doors', -9, -5, -3, ['u-lee'], null],
		['Fix the drawer runners', -6, -2, -1, [SAMPLE_ME], null],
		['Install the backsplash', -14, -3, 's-k-cooking', ['u-sam', 'u-lee'], 240],
		['Paint the cabinets', -10, null, 's-k-todo', ['u-dana'], 360],
		['Choose the handles', -7, null, 's-k-todo', [SAMPLE_ME], null],
		['Fit the kickboards', -4, null, 's-k-todo', [], 90],
		['Plan the pantry', -2, null, 's-k-backlog', [], null],
		['Grout the backsplash', -1, null, 's-k-todo', ['u-lee'], 60]
	];
	const id = (i: number) => `t-k${String(i).padStart(2, '0')}`;
	return [
		...statuses.map(([sid, name, category], i) =>
			row('status', sid, { project_id: P, name, category, sort_key: `a${i}` })
		),
		...work.flatMap(([title, made, started, done, people, estimate], i) => [
			row('task', id(i), {
				project_id: P,
				status_id: typeof done === 'number' ? 's-k-done' : done,
				title,
				sort_key: `b${String(i).padStart(2, '0')}`,
				made_at: at(made, 9),
				started_at: started === null ? null : at(started, 10),
				completed_at: typeof done === 'number' ? at(done, started === done ? 16 : 15) : null,
				estimate_minutes: estimate
			}),
			...people.map((u) => row('task_assignee', pairId(id(i), u), { task_id: id(i), user_id: u }))
		]),
		// The grout waits for the backsplash, which is not done.
		row('task_dependency', pairId(id(20), id(15)), { task_id: id(20), blocker_id: id(15) })
	];
}

export function sampleRows(): Row[] {
	const statuses = [
		['s-backlog', 'Backlog', 'backlog'],
		['s-todo', 'To do', 'todo'],
		['s-doing', 'Doing', 'in-progress'],
		['s-done', 'Done', 'done']
	] as const;
	const tasks: [string, string, string, Record<string, unknown>][] = [
		[
			't-seeds',
			'Order seeds',
			's-todo',
			{ priority: 'high', due_date: day(1), estimate_minutes: 30 }
		],
		[
			't-beds',
			'Build the raised beds',
			's-doing',
			{
				priority: 'medium',
				start_date: day(-1),
				due_date: day(3),
				estimate_minutes: 360,
				started_at: at(-1, 8)
			}
		],
		['t-compost', 'Turn the compost', 's-todo', { due_date: day(0), due_time: '08:30' }],
		[
			't-water',
			'Water the seedlings',
			's-todo',
			{ due_date: day(2), recurrence: 'FREQ=DAILY;INTERVAL=2' }
		],
		['t-fence', 'Mend the fence', 's-backlog', { priority: 'low', estimate_minutes: 90 }],
		[
			't-soil',
			'Test the soil',
			's-done',
			{ started_at: at(-2, 10), completed_at: new Date().toISOString() }
		]
	];
	return [
		...kitchenRows(),
		row('automation', 'a-follow-up', {
			project_id: SAMPLE_PROJECT,
			name: 'Finished, then tell Sam',
			enabled: true,
			rule: {
				when: { on: 'status', value: 's-done' },
				then: [
					{ do: 'priority', value: 'low' },
					{ do: 'subtask', value: 'Book the follow-up' }
				]
			},
			made_by: SAMPLE_ME,
			sort_key: 'a0'
		}),
		row('project', SAMPLE_PROJECT, {
			name: 'Garden',
			icon: 'lucide:sprout',
			color: 'green',
			sort_key: 'a0'
		}),
		row('project', 'p-kitchen', { name: 'Kitchen', icon: '🍳', color: 'orange', sort_key: 'a1' }),
		...statuses.map(([id, name, category], i) =>
			row('status', id, { project_id: SAMPLE_PROJECT, name, category, sort_key: `a${i}` })
		),
		...tasks.map(([id, title, status, extra], i) =>
			row('task', id, {
				project_id: SAMPLE_PROJECT,
				status_id: status,
				title,
				sort_key: `a${i}`,
				made_at: at(-8 + i, 9),
				...extra
			})
		),
		row('task', 't-beds-cut', {
			project_id: SAMPLE_PROJECT,
			status_id: 's-doing',
			parent_id: 't-beds',
			title: 'Cut the boards',
			start_date: day(-1),
			due_date: day(1),
			estimate_minutes: 120,
			sort_key: 'a0'
		}),
		row('label', 'l-outdoors', { name: 'Outdoors', color: 'teal', sort_key: 'a0' }),
		row('label', 'l-errand', { name: 'Errand', color: 'amber', sort_key: 'a1' }),
		row('task_label', 'tl-1', { task_id: 't-seeds', label_id: 'l-errand' }),
		row('task_label', 'tl-2', { task_id: 't-beds', label_id: 'l-outdoors' }),
		// The beds wait for the seeds (open: blocked) and the soil test (done);
		// the fence waits for the beds.
		row('task_dependency', pairId('t-beds', 't-seeds'), {
			task_id: 't-beds',
			blocker_id: 't-seeds'
		}),
		row('task_dependency', pairId('t-beds', 't-soil'), { task_id: 't-beds', blocker_id: 't-soil' }),
		row('task_dependency', pairId('t-fence', 't-beds'), {
			task_id: 't-fence',
			blocker_id: 't-beds'
		}),
		// Time logged (lib/sync/time.ts): the seeds took longer than thought.
		row('time_entry', 'te-1', {
			task_id: 't-beds',
			user_id: SAMPLE_ME,
			day: day(0),
			minutes: 65,
			note: 'Pulled the old edging',
			started_at: new Date(Date.now() - 3 * 3600_000).toISOString()
		}),
		row('time_entry', 'te-2', {
			task_id: 't-beds',
			user_id: 'u-sam',
			day: day(-1),
			minutes: 135,
			note: 'Leveled the ground'
		}),
		row('time_entry', 'te-3', { task_id: 't-seeds', user_id: 'u-lee', day: day(-1), minutes: 50 }),
		row('time_entry', 'te-4', {
			task_id: 't-beds-cut',
			user_id: SAMPLE_ME,
			day: day(-1),
			minutes: 45
		}),
		row('time_entry', 'te-5', { task_id: 't-soil', user_id: 'u-dana', day: day(-2), minutes: 40 }),
		// Time on the project itself, not one of its tasks (lib/sync/time.ts).
		row('project_time_entry', 'pte-1', {
			project_id: SAMPLE_PROJECT,
			user_id: 'u-sam',
			day: day(0),
			minutes: 90,
			note: 'Planning the plot'
		}),
		row('project_time_entry', 'pte-2', {
			project_id: SAMPLE_PROJECT,
			user_id: SAMPLE_ME,
			day: day(-2),
			minutes: 30,
			note: 'Garden center trip'
		}),
		// Two templates for Settings, Templates (lib/templates.ts).
		row('template', 'tpl-garden', {
			kind: 'project',
			name: 'Vegetable garden',
			saved_by: SAMPLE_ME,
			saved_at: '2026-10-28T09:00:00Z',
			sort_key: 'a0',
			data: {
				v: 1,
				project: {
					id: 'p-old',
					name: 'Vegetable garden',
					description: 'Spring planting',
					icon: 'lucide:sprout',
					color: 'green',
					statuses: [
						{ id: 's-1', name: 'To do', category: 'todo' },
						{ id: 's-2', name: 'Growing', category: 'in-progress' },
						{ id: 's-3', name: 'Harvested', category: 'done' }
					],
					fields: [{ id: 'f-1', name: 'Bed', type: 'text', options: [] }],
					views: [{ name: 'This week', type: 'list', filter: { due: 'week' }, display: {} }],
					pages: [{ id: 'pg-1', parent: null, title: '', body: 'What grows where.' }]
				},
				tasks: [
					{ ...tplTask('t-1', 'Test the soil'), start: 0, due: 1 },
					{ ...tplTask('t-2', 'Order seeds'), due: 3 },
					{ ...tplTask('t-3', 'Build the beds'), start: 5, due: 9, waits: ['t-1'] },
					{ ...tplTask('t-4', 'Cut the boards'), parent: 't-3', start: 5, due: 6 },
					{ ...tplTask('t-5', 'Sow'), due: 14, waits: ['t-2', 't-3'] }
				]
			}
		}),
		row('template', 'tpl-weekly', {
			kind: 'task',
			name: 'Weekly review',
			saved_by: SAMPLE_ME,
			saved_at: '2026-10-20T09:00:00Z',
			sort_key: 'a1',
			data: {
				v: 1,
				tasks: [
					{
						...tplTask('t-w', 'Weekly review'),
						due: 0,
						recurrence: 'FREQ=WEEKLY;BYDAY=FR',
						checklist: ['Empty the inbox', 'Check overdue', 'Plan next week']
					}
				]
			}
		}),
		row('saved_view', 'v-mine', {
			owner_id: SAMPLE_ME,
			name: 'Due this week',
			config: {
				filter: { due: 'week', fields: [{ field: 'f-budget', op: 'gt', value: 10 }] }
			},
			display: {
				sort: [{ by: 'due', dir: 'asc' }],
				fields: { list: { show: ['f-size'] } }
			},
			sort_key: 'a0'
		}),
		row('saved_view', 'v-team', {
			name: 'Garden board',
			type: 'board',
			project_id: SAMPLE_PROJECT,
			sort_key: 'a1'
		}),
		row('saved_view', 'v-report', {
			name: 'Garden report',
			type: 'report',
			project_id: SAMPLE_PROJECT,
			display: { report: { tab: 'status', period: 'weeks6' } },
			sort_key: 'a2'
		}),
		row('custom_field', 'f-supplier', { name: 'Supplier', type: 'text', sort_key: 'a0' }),
		row('custom_field', 'f-budget', { name: 'Budget', type: 'number', sort_key: 'a1' }),
		row('custom_field', 'f-size', {
			name: 'Bed size',
			type: 'select',
			options: SIZES,
			sort_key: 'a2'
		}),
		row('custom_field', 'f-ordered', { name: 'Ordered', type: 'checkbox', sort_key: 'a3' }),
		row('custom_field', 'f-delivery', { name: 'Delivery', type: 'date', sort_key: 'a4' }),
		row('custom_field', 'f-link', { name: 'Catalog', type: 'url', sort_key: 'a5' }),
		row('custom_field_value', 'fv-1', {
			task_id: 't-beds',
			field_id: 'f-supplier',
			value: 'Hillside Lumber'
		}),
		row('custom_field_value', 'fv-2', { task_id: 't-beds', field_id: 'f-size', value: 'o-large' }),
		row('custom_field_value', 'fv-3', { task_id: 't-seeds', field_id: 'f-budget', value: 40 }),
		row('custom_field_value', 'fv-4', { task_id: 't-seeds', field_id: 'f-size', value: 'o-small' }),
		row('custom_field_value', 'fv-5', { task_id: 't-seeds', field_id: 'f-ordered', value: true }),
		row('page', GARDEN_WIKI, {
			project_id: SAMPLE_PROJECT,
			body: [
				`Everything about the garden that is not a task. Sowing dates are in the [Planting calendar](${pageLink(PLANTING)}).`,
				'## This year',
				'- [x] Tomatoes along the south fence\n- [ ] **Beans** where the squash was last year',
				'> [!TIP]\n> Water in the morning, before the heat.',
				'| Bed | Size | Sun |\n| --- | ---: | --- |\n| North | 4 x 8 | Full |\n| South | 4 x 4 | Half |',
				'```python\n# Soil test, in parts per million\nreadings = {"N": 40, "P": 22}\n```'
			].join('\n\n'),
			sort_key: 'a0'
		}),
		row('page_thread', 'th-beans', {
			page_id: GARDEN_WIKI,
			created_by: 'u-pia',
			quote: 'where the squash was last year',
			anchor: ''
		}),
		row('page_comment', 'pc-beans', {
			page_id: GARDEN_WIKI,
			thread_id: 'th-beans',
			author_id: 'u-pia',
			body: 'Should we rest this bed instead?'
		}),
		row('page_thread', 'th-fence', {
			page_id: GARDEN_WIKI,
			created_by: SAMPLE_ME,
			quote: 'along the south fence',
			anchor: '',
			resolved_at: '2026-10-20T12:00:00Z'
		}),
		row('page_comment', 'pc-fence', {
			page_id: GARDEN_WIKI,
			thread_id: 'th-fence',
			author_id: SAMPLE_ME,
			body: 'Or the north one? It gets less wind.'
		}),
		row('page', PLANTING, {
			project_id: SAMPLE_PROJECT,
			parent_id: GARDEN_WIKI,
			title: 'Planting calendar',
			body: `Start peppers indoors in *late February*. Back to the [Garden](${pageLink(GARDEN_WIKI)}).`,
			sort_key: 'a0'
		}),
		row('page', 'pg-frost', {
			project_id: SAMPLE_PROJECT,
			parent_id: PLANTING,
			title: 'Frost dates',
			sort_key: 'a0'
		}),
		row('page', 'pg-suppliers', {
			project_id: SAMPLE_PROJECT,
			parent_id: GARDEN_WIKI,
			title: 'Suppliers',
			sort_key: 'a1'
		}),
		row('custom_field_value', 'fv-6', {
			task_id: 't-compost',
			field_id: 'f-link',
			value: 'https://seeds.example.com/compost'
		})
	];
}
