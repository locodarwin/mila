// The weekly report against shared/weekly-vectors.json, which holds the
// server's Go copy (server/internal/reports) to the same answers. With
// WEEKLY_VECTORS=write it writes the app's answers into the file instead.
import { readFileSync, writeFileSync } from 'node:fs';
import { afterAll, describe, expect, it } from 'vitest';
import { inScope, weeklyReport, type WeeklyEntry, type WeeklyReport } from './reports';
import type { StatusCategory, Task } from './sync/entities';

interface VectorTask {
	id: string;
	title: string;
	projectId?: string | null;
	parentId?: string | null;
	category: StatusCategory;
	madeAt?: string | null;
	completedAt?: string | null;
	startedAt?: string | null;
	dueDate?: string | null;
	estimateMinutes?: number | null;
	mine?: boolean;
}

interface Case {
	name: string;
	zone: string;
	today: string;
	me: string;
	projects: { id: string; name: string; inUse: boolean }[];
	tasks: VectorTask[];
	dependencies: { taskId: string; blockerId: string }[];
	entries: WeeklyEntry[];
	expect: unknown;
}

const file = new URL('../../../shared/weekly-vectors.json', import.meta.url);
const vectors = JSON.parse(readFileSync(file, 'utf8')) as { about: string; cases: Case[] };
const writing = process.env.WEEKLY_VECTORS === 'write';

/** The report as the vectors write it: tasks by their IDs. */
function plain(r: WeeklyReport) {
	return { ...r, needs: r.needs.map((n) => ({ ...n, task: n.task.id })) };
}

function run(c: Case) {
	const tasks = c.tasks.map((v): Task => ({
		id: v.id,
		projectId: v.projectId ?? null,
		statusId: null,
		parentId: v.parentId ?? null,
		title: v.title,
		notes: '',
		sortKey: 'a0',
		completedAt: v.completedAt ?? null,
		priority: 'none',
		startDate: null,
		dueDate: v.dueDate ?? null,
		dueTime: null,
		createdBy: null,
		recurrence: null,
		recurrenceMode: 'schedule',
		seriesId: null,
		estimateMinutes: v.estimateMinutes ?? null,
		madeAt: v.madeAt ?? null,
		startedAt: v.startedAt ?? null
	}));
	const byId = new Map(tasks.map((t) => [t.id, t]));
	const vector = new Map(c.tasks.map((v) => [v.id, v]));
	const category = (t: Task) => vector.get(t.id)!.category;
	const inUse = new Set(c.projects.filter((p) => p.inUse).map((p) => p.id));
	const scoped = inScope({
		tasks,
		task: (id) => byId.get(id),
		projectInUse: (id) => inUse.has(id),
		category,
		keep: () => true
	});
	return weeklyReport(scoped, {
		today: c.today,
		category,
		openBlockers: (id) =>
			c.dependencies.filter((d) => d.taskId === id && byId.get(d.blockerId)?.completedAt === null)
				.length,
		logged: (id) => c.entries.filter((e) => e.taskId === id).reduce((n, e) => n + e.minutes, 0),
		mine: (t) => vector.get(t.id)!.mine ?? false,
		projectName: (id) => c.projects.find((p) => p.id === id)?.name ?? '',
		projectInUse: (id) => inUse.has(id),
		entries: c.entries,
		me: c.me
	});
}

describe('the weekly report', () => {
	const zone = process.env.TZ;
	afterAll(() => {
		if (zone === undefined) delete process.env.TZ;
		else process.env.TZ = zone;
		if (writing) writeFileSync(file, JSON.stringify(vectors, null, 2) + '\n');
	});
	for (const c of vectors.cases) {
		it(c.name, () => {
			process.env.TZ = c.zone;
			const got = plain(run(c));
			if (writing) {
				c.expect = got;
				expect(got).toBeTruthy();
				return;
			}
			expect(got).toEqual(c.expect);
		});
	}

	it('reads days in the zone each case names', () => {
		const c = vectors.cases.find((x) => x.zone !== 'UTC')!;
		process.env.TZ = 'UTC';
		const utc = plain(run(c));
		process.env.TZ = c.zone;
		expect(plain(run(c))).not.toEqual(utc);
	});
});
