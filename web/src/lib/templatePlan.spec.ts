import { describe, expect, it } from 'vitest';
import type { CustomField, Label, Page, SavedView, Status, Task } from './sync/entities';
import { pairId } from './sync/entities';
import { mainPageId } from './sync/pages';
import { planProject, planTask, snapshotProject, snapshotTask, type Source } from './templatePlan';
import { contents, lastDay, tooBig } from './templateInfo';
import { readTemplate } from './templates';

const P = '11111111-1111-4111-8111-111111111111';
const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
const [DEMO, ELEC, INSP, CAB, INST, DONE] = [1, 2, 3, 4, 5, 6].map(id);
const [TODO, DOING] = [21, 22].map(id);
const SIZE = id(31);
const WIDE = id(32);
const PAGE = id(41);
const [OLD_LABEL, GONE_LABEL] = [51, 52].map(id);

const task = (tid: string, over: Partial<Task> = {}): Task => ({
	id: tid,
	projectId: P,
	statusId: TODO,
	parentId: null,
	title: tid,
	notes: '',
	sortKey: 'a0',
	completedAt: null,
	priority: 'none',
	startDate: null,
	dueDate: null,
	dueTime: null,
	createdBy: null,
	recurrence: null,
	recurrenceMode: 'schedule',
	seriesId: null,
	estimateMinutes: null,
	madeAt: null,
	startedAt: null,
	...over
});

function source(): Source {
	const tasks: Task[] = [
		task(DEMO, {
			title: 'Demo',
			startDate: '2026-10-26',
			dueDate: '2026-10-30',
			estimateMinutes: 240
		}),
		task(ELEC, {
			title: 'Electrical',
			statusId: DOING,
			startDate: '2026-11-02',
			dueDate: '2026-11-05',
			notes: `After [Demo](/?task=${DEMO}).`
		}),
		task(INSP, { title: 'Inspection', dueDate: '2026-11-09', dueTime: '09:00' }),
		task(CAB, { title: 'Cabinets' }),
		task(INST, { title: 'Install', parentId: CAB, startDate: '2026-11-16', dueDate: '2026-11-19' }),
		task(DONE, { title: 'Done already', completedAt: '2026-10-20T00:00:00Z' })
	];
	const statuses: Status[] = [
		{ id: TODO, projectId: P, name: 'To do', category: 'todo', sortKey: 'a0' },
		{ id: DOING, projectId: P, name: 'Doing', category: 'in-progress', sortKey: 'a1' }
	];
	const fields: CustomField[] = [
		{
			id: SIZE,
			projectId: P,
			name: 'Size',
			type: 'select',
			options: [{ id: 'o1', name: 'Large', color: 'red' }],
			sortKey: 'a0'
		},
		{ id: WIDE, projectId: null, name: 'Budget', type: 'number', options: [], sortKey: 'a1' }
	];
	const pages: Page[] = [
		{
			id: PAGE,
			projectId: P,
			parentId: mainPageId(P),
			title: 'Permits',
			body: `See [Electrical](/?task=${ELEC}).`,
			sortKey: 'a0',
			deletedAt: null
		}
	];
	const views: SavedView[] = [
		{
			id: id(61),
			projectId: P,
			ownerId: null,
			name: 'Big ones',
			type: 'list',
			filter: { projects: [P] },
			display: { group: { by: 'field', field: SIZE } },
			sortKey: 'a0'
		},
		{
			id: id(62),
			projectId: 'other',
			ownerId: null,
			name: 'Elsewhere',
			type: 'list',
			filter: {},
			display: {},
			sortKey: 'a1'
		}
	];
	return {
		automations: () => [],
		project: (pid) =>
			pid === P
				? {
						id: P,
						name: 'Kitchen',
						icon: '🔨',
						color: 'orange',
						description: 'Redo it',
						sortKey: 'a0',
						archivedAt: null
					}
				: undefined,
		task: (tid) => tasks.find((t) => t.id === tid),
		tasks: () => tasks.filter((t) => !t.parentId),
		subtasks: (tid) => tasks.filter((t) => t.parentId === tid),
		statuses: () => statuses,
		status: (sid) => statuses.find((s) => s.id === sid),
		fields: (pid) =>
			fields.filter((f) => pid === undefined || f.projectId === null || f.projectId === pid),
		fieldValues: (tid) => (tid === ELEC ? { [SIZE]: 'o1', [WIDE]: 120 } : {}),
		taskLabels: (tid): Label[] =>
			tid === ELEC
				? [
						{ id: OLD_LABEL, name: 'Trades', color: 'teal', sortKey: 'a0' },
						{ id: GONE_LABEL, name: 'Permit', color: 'red', sortKey: 'a1' }
					]
				: [],
		assignees: (tid) => (tid === ELEC ? ['u-sam', 'u-gone'] : []),
		checklist: (tid) => (tid === INSP ? [{ text: 'Book it' }, { text: 'Be there' }] : []),
		blockers: (tid) => (tid === ELEC ? [tasks[0]] : tid === INSP ? [tasks[1], tasks[5]] : []),
		pages: () => pages,
		views: () => views,
		comments: (tid) => (tid === ELEC ? [{}, {}] : []),
		attachments: (tid) => (tid === DEMO ? [{}] : [])
	};
}

let n = 0;
const target = (labels: Label[] = []) => ({
	me: 'u-me',
	uuid: () => `99999999-0000-4000-8000-${String(++n).padStart(12, '0')}`,
	labels,
	members: new Set(['u-me', 'u-sam']),
	fields: new Map(
		source()
			.fields()
			.map((f) => [f.id, f] as const)
	)
});

describe('templates', () => {
	it('takes a project: open tasks, dates from the first, and what stays behind', () => {
		const snap = snapshotProject(source(), P, { keepAssignees: true, me: 'u-me' })!;
		expect(snap.anchor).toBe('2026-10-26');
		expect(snap.left).toEqual({ done: 1, comments: 2, files: 1 });
		const byId = Object.fromEntries(snap.data.tasks.map((t) => [t.id, t]));
		expect(Object.keys(byId)).toEqual([DEMO, ELEC, INSP, CAB, INST]);
		expect(byId[ELEC]).toMatchObject({
			start: 7,
			due: 10,
			status: DOING,
			waits: [DEMO],
			assignees: ['u-sam', 'u-gone']
		});
		// A done blocker is left out of what a task waits for.
		expect(byId[INSP].waits).toEqual([ELEC]);
		expect(byId[INST].parent).toBe(CAB);
		expect(snap.data.project?.fields.map((f) => f.id)).toEqual([SIZE]);
		expect(snap.data.project?.views.map((v) => v.name)).toEqual(['Big ones']);
		expect(contents({ kind: 'project', data: snap.data })).toBe('5 tasks, 1 page, 2 statuses');
		expect(lastDay(snap.data)).toBe(24);
		expect(tooBig(snap.data)).toBeNull();
		// It reads back as stored.
		expect(readTemplate(JSON.parse(JSON.stringify(snap.data)))).toEqual(snap.data);
	});

	it('makes a project from it, every date moved to the start', () => {
		const snap = snapshotProject(source(), P, { keepAssignees: true, me: 'u-me' })!;
		const labels: Label[] = [{ id: 'l-trades', name: 'trades', color: 'teal', sortKey: 'a0' }];
		const plan = planProject(
			snap.data,
			{ name: 'Bathroom', start: '2026-12-01', projectSortKey: 'b0', labelsAfter: 'a0' },
			target(labels)
		);
		const rows = (e: string) => plan.rows.filter((r) => r.entity === e);
		expect(rows('project')[0]).toMatchObject({
			id: plan.id,
			values: { name: 'Bathroom', icon: '🔨', color: 'orange' }
		});
		const statusIds = rows('status').map((r) => r.id);
		const tasks = Object.fromEntries(rows('task').map((r) => [r.values.title, r]));
		expect(tasks.Demo.values).toMatchObject({
			project_id: plan.id,
			start_date: '2026-12-01',
			due_date: '2026-12-05',
			status_id: statusIds[0],
			estimate_minutes: 240
		});
		expect(tasks.Electrical.values.estimate_minutes).toBeNull();
		expect(tasks.Electrical.values).toMatchObject({
			start_date: '2026-12-08',
			status_id: statusIds[1],
			notes: `After [Demo](/?task=${tasks.Demo.id}).`
		});
		expect(tasks.Inspection.values).toMatchObject({ due_date: '2026-12-15', due_time: '09:00' });
		expect(tasks.Install.values.parent_id).toBe(tasks.Cabinets.id);
		// Labels found by name, or made; a member who left is dropped.
		const made = rows('label');
		expect(made.map((r) => r.values.name)).toEqual(['Permit']);
		expect(rows('task_label').map((r) => r.values.label_id)).toEqual(['l-trades', made[0].id]);
		expect(rows('task_assignee').map((r) => r.values.user_id)).toEqual(['u-sam']);
		expect(plan.dropped).toBe(1);
		expect(rows('task_dependency')).toContainEqual({
			entity: 'task_dependency',
			id: pairId(tasks.Electrical.id, tasks.Demo.id),
			values: { task_id: tasks.Electrical.id, blocker_id: tasks.Demo.id, deleted_at: null }
		});
		// The project's own field is made anew; a workspace field is kept.
		const field = rows('custom_field')[0];
		expect(
			rows('custom_field_value')
				.map((r) => r.values.field_id)
				.sort()
		).toEqual([field.id, WIDE].sort());
		expect(rows('checklist_item').map((r) => r.values.text)).toEqual(['Book it', 'Be there']);
		// The wiki: a main page made first, the page under it with its link moved.
		const pages = rows('page');
		expect(pages[0]).toMatchObject({ id: mainPageId(plan.id), values: { parent_id: null } });
		expect(pages[1].values).toMatchObject({
			parent_id: mainPageId(plan.id),
			body: `See [Electrical](/?task=${tasks.Electrical.id}).`
		});
		// The view is the maker's, naming the new project and field.
		expect(rows('saved_view')[0].values).toMatchObject({
			owner_id: 'u-me',
			config: { filter: { projects: [plan.id] } },
			display: { group: { by: 'field', field: field.id } }
		});
		// What a row names comes before it.
		const order = plan.rows.map((r) => r.entity);
		expect(order.indexOf('task_dependency')).toBeGreaterThan(order.lastIndexOf('task'));
	});

	it('takes a task and makes it again, due as many days after today', () => {
		const src = source();
		const snap = snapshotTask(src, ELEC, { keepAssignees: false, today: '2026-11-01' })!;
		expect(snap.data.tasks[0]).toMatchObject({
			id: ELEC,
			parent: null,
			start: 1,
			due: 4,
			category: 'in-progress'
		});
		expect(snap.data.tasks[0].assignees).toBeUndefined();
		const statuses: Status[] = [
			{ id: 's-todo', projectId: 'q', name: 'Next', category: 'todo', sortKey: 'a0' },
			{ id: 's-doing', projectId: 'q', name: 'Now', category: 'in-progress', sortKey: 'a1' }
		];
		const plan = planTask(
			snap.data,
			{ projectId: 'q', statuses, today: '2026-12-10', after: 'a5', labelsAfter: null },
			target()
		);
		const root = plan.rows.find((r) => r.id === plan.id)!;
		expect(root.values).toMatchObject({
			project_id: 'q',
			status_id: 's-doing',
			start_date: '2026-12-11',
			due_date: '2026-12-14',
			parent_id: null
		});
		expect((root.values.sort_key as string) > 'a5').toBe(true);
		// The project's own field does not reach another project; the workspace's does.
		expect(
			plan.rows.filter((r) => r.entity === 'custom_field_value').map((r) => r.values.field_id)
		).toEqual([WIDE]);
		// An overdue task saved is due the day it is used.
		const late = snapshotTask(src, INSP, { keepAssignees: false, today: '2026-11-20' })!;
		expect(late.data.tasks[0].due).toBe(0);
	});

	it('says when a template is too large', () => {
		const big = {
			v: 1 as const,
			tasks: Array.from({ length: 501 }, (_, i) => ({
				...readTemplate({ v: 1, tasks: [{ id: id(i), title: 'x' }] })!.tasks[0]
			}))
		};
		expect(tooBig(big)).toMatch(/501 open tasks; a template holds 500/);
		expect(readTemplate({ v: 2, tasks: [] })).toBeNull();
		expect(readTemplate('x')).toBeNull();
	});
});
