// Saving templates and making things from them,
// with the toasts that follow. Loaded only when a template is saved or
// used, so no page pays for it up front; the snapshot and the rows to
// write come from templatePlan.ts.
import { madeForTemplate } from './templateFill';
import { formatDate } from './dates';
import { session } from './session.svelte';
import { createPage, deletePage, localToday, setPageTitle, setTaskDeleted } from './sync/data';
import { engine } from './sync/engine.svelte';
import type { Replica } from './sync/replica.svelte';
import { keyBetween } from './sync/sortkey';
import { openTask } from './taskLink';
import { tooBig } from './templateInfo';
import { MAX_TEMPLATES, type TemplateKind } from './templates';
import { toasts } from './toasts.svelte';
import { uuid } from './uuid';
import { pageHref } from './wiki';

const now = () => new Date().toISOString();
const me = () => session.user?.id ?? null;
const keyAfter = (items: { sortKey: string }[]) => keyBetween(items.at(-1)?.sortKey ?? null, null);

function replicaOf(ws: string) {
	const r = engine.replica(ws);
	if (!r) throw new Error('That workspace is not loaded yet.');
	return r;
}

/**
 * What a snapshot and a plan read of the workspace. Kept assignees go to
 * those who can reach the task; a new project is shared with no guest yet.
 */
function targetOf(ws: string, replica: Replica, projectId: string | null = null) {
	return {
		me: me() ?? '',
		uuid,
		labels: replica.labels(),
		members: new Set(session.reachable(ws, projectId).map((m) => m.userId)),
		fields: new Map(replica.fields().map((f) => [f.id, f]))
	};
}

/**
 * Saves a project or a task as a template, new or over one of the same
 * kind. Says what is wrong instead when it is too large or there are too
 * many templates.
 */
export async function saveTemplate(
	ws: string,
	opts: {
		kind: TemplateKind;
		sourceId: string;
		name: string;
		keepAssignees: boolean;
		/** For a page: whether its subpages come along. */
		subpages?: boolean;
		/** For a page: its text as it stands in its editor, if it is open. */
		body?: string;
		replace?: string | null;
	}
): Promise<{ id: string } | { error: string }> {
	const { snapshotPage, snapshotProject, snapshotTask } = await import('./templatePlan');
	const replica = replicaOf(ws);
	const snap =
		opts.kind === 'page'
			? snapshotPage(replica, opts.sourceId, {
					subpages: opts.subpages ?? false,
					title: replica.page(opts.sourceId)?.title ?? '',
					body: opts.body
				})
			: opts.kind === 'project'
				? snapshotProject(replica, opts.sourceId, {
						keepAssignees: opts.keepAssignees,
						me: me() ?? ''
					})
				: snapshotTask(replica, opts.sourceId, {
						keepAssignees: opts.keepAssignees,
						today: localToday()
					});
	if (!snap) return { error: 'That is no longer here.' };
	const problem = tooBig(snap.data);
	if (problem) return { error: problem };
	if (!opts.replace && replica.templates().length >= MAX_TEMPLATES)
		return { error: `A workspace keeps ${MAX_TEMPLATES} templates at most. Delete some first.` };
	const id = opts.replace ?? uuid();
	await engine.mutate(ws, 'template', id, (r) => ({
		...(opts.replace ? {} : { kind: opts.kind, sort_key: keyAfter(r.templates()) }),
		name: opts.name.trim(),
		data: snap.data,
		saved_by: me(),
		saved_at: now(),
		deleted_at: null
	}));
	return { id };
}

export function renameTemplate(ws: string, id: string, name: string) {
	return engine.mutate(ws, 'template', id, { name: name.trim() });
}

/** Moves a template to the trash, or brings it back. */
export function setTemplateDeleted(ws: string, id: string, deleted: boolean) {
	return engine.mutate(ws, 'template', id, { deleted_at: deleted ? now() : null });
}

/**
 * Makes a project from a template, its dates counted from start. Undone
 * as a whole, by deleting the project (setProjectDeleted).
 */
export async function makeProjectFromTemplate(
	ws: string,
	templateId: string,
	opts: { name: string; start: string }
): Promise<{ id: string; dropped: number }> {
	const { planProject } = await import('./templatePlan');
	const replica = replicaOf(ws);
	const t = replica.templates().find((x) => x.id === templateId);
	if (!t) throw new Error('That template is no longer here.');
	const plan = planProject(
		t.data,
		{
			name: opts.name,
			start: opts.start,
			projectSortKey: keyAfter(replica.projects()),
			labelsAfter: replica.labels().at(-1)?.sortKey ?? null
		},
		targetOf(ws, replica)
	);
	await engine.insert(ws, plan.rows);
	return { id: plan.id, dropped: plan.dropped };
}

/**
 * Makes a task from a template in a project (or the inbox, null), after
 * its last task. Undone as a whole, by deleting the task.
 */
export async function makeTaskFromTemplate(
	ws: string,
	templateId: string,
	projectId: string | null
): Promise<{ id: string; dropped: number; due: string | null }> {
	const { planTask } = await import('./templatePlan');
	const replica = replicaOf(ws);
	const t = replica.templates().find((x) => x.id === templateId);
	if (!t) throw new Error('That template is no longer here.');
	const plan = planTask(
		t.data,
		{
			projectId,
			statuses: projectId ? replica.statuses(projectId) : [],
			today: localToday(),
			after: replica.tasks(projectId).at(-1)?.sortKey ?? null,
			labelsAfter: replica.labels().at(-1)?.sortKey ?? null
		},
		targetOf(ws, replica, projectId)
	);
	await engine.insert(ws, plan.rows);
	return {
		id: plan.id,
		dropped: plan.dropped,
		due: (plan.rows[0]?.values.due_date as string) ?? null
	};
}

/** Makes a task from a template where the person is, with a toast that opens it or takes it back. */
export async function newTaskFromTemplate(
	ws: string,
	templateId: string,
	projectId: string | null
) {
	const t = engine
		.replica(ws)
		?.templates()
		.find((x) => x.id === templateId);
	if (!t) return;
	const made = await makeTaskFromTemplate(ws, templateId, projectId);
	const where = projectId
		? (engine.replica(ws)?.project(projectId)?.name ?? 'the project')
		: 'Inbox';
	const left = made.dropped
		? `; ${made.dropped} ${made.dropped === 1 ? 'assignee who left was' : 'assignees who left were'} left off`
		: '';
	toasts.show(
		`Made ${t.name} in ${where}${made.due ? `, due ${formatDate(made.due)}` : ''}${left}`,
		{
			action: { label: 'Open', run: () => void openTask(made.id) },
			onundo: () => void setTaskDeleted(ws, made.id, true)
		}
	);
	return made.id;
}

/**
 * Fills an empty page from a template, with a toast that takes it back. The
 * text goes through the page's editor, as if typed; a title already typed is
 * kept, and a main page has none to set.
 */
export async function fillPageFromTemplate(
	ws: string,
	templateId: string,
	target: {
		pageId: string;
		projectId: string | null;
		isMain: boolean;
		title: string;
		fill: (markdown: string) => boolean;
		clear: () => void;
		/** The page's text in its editor now; null once it has closed. */
		current: () => string | null;
	}
) {
	const { planPage } = await import('./templatePlan');
	const t = replicaOf(ws)
		.templates()
		.find((x) => x.id === templateId);
	if (!t) return;
	const plan = planPage(t.data, {
		pageId: target.pageId,
		projectId: target.projectId,
		today: formatDate(localToday()),
		uuid,
		href: pageHref
	});
	if (!target.fill(plan.body)) return;
	const filled = target.current();
	const setsTitle = !target.isMain && !target.title.trim() && !!plan.title.trim();
	if (setsTitle) await setPageTitle(ws, target.pageId, plan.title);
	if (plan.rows.length) await engine.insert(ws, plan.rows);
	const under = plan.subpages.length;
	toasts.show(
		`Used the template ${t.name}${under ? `, with ${under} ${under === 1 ? 'subpage' : 'subpages'}` : ''}`,
		{
			onundo: () => {
				if (filled === null || target.current() !== filled) {
					toasts.show('This page has changed or closed since, so it stays as it is.');
					return;
				}
				target.clear();
				if (setsTitle) void setPageTitle(ws, target.pageId, '');
				for (const id of plan.subpages) void deletePage(ws, id);
			}
		}
	);
}

/**
 * A new page in a wiki, made to be filled from a template: the address that opens
 * it and fills it. Its parent is the page given, or the wiki's main page.
 */
export async function newPageForTemplate(
	ws: string,
	templateId: string,
	where: { projectId: string | null; parentId: string }
): Promise<string> {
	const id = await createPage(ws, where.projectId, where.parentId, '');
	madeForTemplate(id, templateId);
	return `${pageHref({ id, projectId: where.projectId })}&template=${templateId}`;
}

/**
 * Duplicates a task (a right-click menu's Duplicate): a copy just under it, with
 * its notes, checklist, labels, assignees, custom fields and open subtasks, on the
 * same dates, and nothing that happened to it (comments, time, files, history).
 * Made the way a task template makes one, in one go, with a toast that takes it back.
 */
export async function duplicateTask(ws: string, taskId: string) {
	const { snapshotTask, planTask } = await import('./templatePlan');
	const replica = replicaOf(ws);
	const task = replica.task(taskId);
	if (!task) return;
	const snap = snapshotTask(replica, taskId, { keepAssignees: true, today: localToday() });
	if (!snap) return;
	const title = task.title || 'Untitled task';
	snap.data.tasks[0].title = `${title} (copy)`;
	const plan = planTask(
		snap.data,
		{
			projectId: task.projectId,
			statuses: task.projectId ? replica.statuses(task.projectId) : [],
			// Dates are kept from the day the snapshot counts from, so they come out the same.
			today: snap.anchor ?? localToday(),
			after: task.sortKey,
			labelsAfter: replica.labels().at(-1)?.sortKey ?? null
		},
		targetOf(ws, replica, task.projectId)
	);
	// Just under it, beside it: under the same task, if it's a subtask.
	const siblings = task.parentId ? replica.subtasks(task.parentId) : replica.tasks(task.projectId);
	const next = siblings.find((t) => t.sortKey > task.sortKey);
	const root = plan.rows.find((r) => r.entity === 'task' && r.id === plan.id);
	if (root) {
		root.values.sort_key = keyBetween(task.sortKey, next?.sortKey ?? null);
		root.values.parent_id = task.parentId;
	}
	await engine.insert(ws, plan.rows);
	toasts.show(`Made a copy of "${title}"`, {
		action: { label: 'Open', run: () => void openTask(plan.id) },
		onundo: () => void setTaskDeleted(ws, plan.id, true)
	});
	return plan.id;
}
