// What the right-click menus of a task, a view and a project hold, in the
// same order everywhere. Each is an action the app has somewhere else
// already, done the same way, with the same undo. Someone who can't do a
// thing isn't offered it. Loaded the first time a menu opens.
import { goto } from '$app/navigation';
import { tick } from 'svelte';
import { resolve } from '$app/paths';
import { copyText } from './clipboard';
import type { MenuItem, MenuSpec } from './contextMenu.svelte';
import { referencesToDisplay, toDisplay } from './mentions';
import { intents } from './intents.svelte';
import type { CommandId } from './keys/commands';
import {
	canDeleteOthersComments,
	canEditViews,
	canOrganize,
	canShare,
	canUseTemplates
} from './roles';
import { scheduleChoices, scheduler } from './schedule.svelte';
import { selection } from './selection.svelte';
import { session } from './session.svelte';
import {
	deleteView,
	localToday,
	moveTask,
	setAssignee,
	setInMyDay,
	setProjectArchived,
	setProjectDeleted,
	setTaskCompleted,
	setTaskDeleted,
	setTaskFields,
	setTaskLabel,
	setTaskStatus,
	shareView,
	createView,
	deleteAttachment,
	deleteComment,
	deleteStatus,
	moveStatus,
	updateProject,
	updateStatus
} from './sync/data';
import { engine } from './sync/engine.svelte';
import type { Attachment, Comment, Priority, StatusCategory } from './sync/entities';
import { fileUrl } from './uploads';
import { openTask } from './taskLink';
import { PALETTE } from './theme/palette';
import { toasts } from './toasts.svelte';

const role = () => session.workspace?.role;
const may = (check: (r: NonNullable<ReturnType<typeof role>>) => boolean) => {
	const r = role();
	return !r || check(r);
};
const me = () => session.user?.id ?? '';
const sep = 'separator' as const;
const cap = (s: string) => s[0].toUpperCase() + s.slice(1);

/** Does an action, with a message that takes back just it. */
async function tracked(said: string, fn: () => Promise<unknown>) {
	const { done, undo } = engine.track(fn);
	await done;
	toasts.show(said, { onundo: () => void undo() });
}

async function copyLink(path: string) {
	if (await copyText(location.origin + path)) toasts.show('Link copied');
}

const PRIORITIES: [Priority, string, CommandId][] = [
	['urgent', 'Urgent', 'task.priority.urgent'],
	['high', 'High', 'task.priority.high'],
	['medium', 'Medium', 'task.priority.medium'],
	['low', 'Low', 'task.priority.low'],
	['none', 'None', 'task.priority.none']
];

/** A task's menu, wherever it shows. */
export function taskMenu(ws: string, taskId: string, anchor: HTMLElement | null): MenuSpec | null {
	const replica = engine.replica(ws);
	const task = replica?.task(taskId);
	if (!replica || !task) return null;
	const title = task.title || 'Untitled task';
	const organize = may(canOrganize);
	const today = localToday();
	const inMyDay = replica.inMyDay(me(), task.id, today);
	const timer = replica.runningTimer(me());
	const timing = timer?.taskId === task.id;
	const selected = selection.has(ws, task.id);
	const items: MenuItem[] = [
		{
			id: 'select',
			label: selected ? 'Take out of the selection' : 'Select',
			command: 'select.toggle',
			run: () => {
				// On a touch screen, taps go on selecting until Done.
				if (!selected && matchMedia('(pointer: coarse)').matches) selection.pick(ws, task.id);
				else selection.toggle(ws, task.id);
			}
		},
		{ id: 'open', label: 'Open', command: 'task.open', run: () => openTask(task.id) },
		{
			id: 'complete',
			label: task.completedAt ? 'Reopen' : 'Complete',
			command: 'task.complete',
			run: () =>
				tracked(`${task.completedAt ? 'Reopened' : 'Completed'} "${title}"`, () =>
					setTaskCompleted(ws, task, !task.completedAt)
				)
		},
		{
			id: 'schedule',
			label: 'Schedule',
			sub: () => [
				...scheduleChoices(today).map((c) => ({
					id: c.id,
					label: c.hint ? `${c.label}, ${c.hint}` : c.label,
					checked: (task.dueDate ?? null) === c.date ? true : undefined,
					run: () => setTaskFields(ws, task.id, { due_date: c.date })
				})),
				sep,
				{ id: 'pick', label: 'Pick a date', run: () => scheduler.open(ws, task.id, anchor) }
			]
		},
		{
			id: 'my-day',
			label: inMyDay ? 'Take out of My Day' : 'Add to My Day',
			command: 'task.my-day',
			run: () => setInMyDay(ws, me(), task.id, !inMyDay)
		},
		{
			id: 'priority',
			label: 'Priority',
			sub: () =>
				PRIORITIES.map(([p, label, command]) => ({
					id: p,
					label,
					command,
					checked: task.priority === p,
					run: () => setTaskFields(ws, task.id, { priority: p })
				}))
		}
	];
	if (task.projectId) {
		const statuses = replica.statuses(task.projectId);
		if (statuses.length)
			items.push({
				id: 'status',
				label: 'Status',
				sub: () =>
					statuses.map((s) => ({
						id: s.id,
						label: s.name,
						checked: task.statusId === s.id,
						run: () => setTaskStatus(ws, task, s.id)
					}))
			});
	}
	const labels = replica.labels();
	if (labels.length)
		items.push({
			id: 'labels',
			label: 'Labels',
			sub: () => {
				const on = new Set((engine.replica(ws)?.taskLabels(task.id) ?? []).map((l) => l.id));
				return labels.map((l) => ({
					id: l.id,
					label: l.name,
					checked: on.has(l.id),
					keep: true,
					run: () => setTaskLabel(ws, task.id, l.id, !on.has(l.id))
				}));
			}
		});
	const people = session.reachable(ws, task.projectId);
	if (people.length > 1 || (people.length === 1 && people[0].userId !== me()) || organize)
		items.push({
			id: 'assign',
			label: 'Assign',
			sub: () => {
				const on = new Set(engine.replica(ws)?.assignees(task.id) ?? []);
				return people.map((p) => ({
					id: p.userId,
					label: p.userId === me() ? `${p.name} (you)` : p.name,
					checked: on.has(p.userId),
					keep: true,
					run: () => setAssignee(ws, task.id, p.userId, !on.has(p.userId))
				}));
			}
		});
	if (organize && !task.parentId)
		items.push({
			id: 'move',
			label: 'Move to',
			sub: () => [
				{
					id: 'inbox',
					label: 'Inbox',
					checked: task.projectId === null,
					run: () => tracked(`Moved "${title}" to the Inbox`, () => moveTask(ws, task, null))
				},
				...replica.projects().map((p) => ({
					id: p.id,
					label: p.name,
					checked: task.projectId === p.id,
					run: () => tracked(`Moved "${title}" to ${p.name}`, () => moveTask(ws, task, p.id))
				}))
			]
		});
	items.push({
		id: 'timer',
		label: timing ? 'Stop the timer' : 'Start a timer',
		run: async () => {
			const time = await import('./sync/time');
			if (timing && timer) await time.stopTimer(ws, timer);
			else await time.startTimer(ws, task.id);
		}
	});
	items.push(sep, {
		id: 'duplicate',
		label: 'Duplicate',
		run: async () => (await import('./templateActions')).duplicateTask(ws, task.id)
	});
	items.push({ id: 'link', label: 'Copy link', run: () => copyLink(`/?task=${task.id}`) });
	if (may(canUseTemplates))
		items.push({
			id: 'template',
			label: 'Save as a template',
			run: () => {
				intents.ask('task', task.id, 'template');
				openTask(task.id);
			}
		});
	items.push(sep, {
		id: 'delete',
		label: 'Delete',
		command: 'task.delete',
		danger: true,
		run: () => tracked(`Deleted "${title}"`, () => setTaskDeleted(ws, task.id, true))
	});
	return { name: title, kind: 'task actions', items };
}

const viewHref = (id: string) => resolve('/(app)/views/[id]', { id });
const projectHref = (id: string) => resolve('/(app)/projects/[id]', { id });

/** A saved view's menu: in the sidebar, and on its own title. */
export function viewMenu(ws: string, viewId: string): MenuSpec | null {
	const view = engine.replica(ws)?.view(viewId);
	if (!view) return null;
	const editable = may(canEditViews) && (view.ownerId === null || view.ownerId === me());
	const items: MenuItem[] = [
		{ id: 'open', label: 'Open', run: () => goto(resolve('/(app)/views/[id]', { id: view.id })) },
		{
			id: 'tab',
			label: 'Open in a new tab',
			run: () => void window.open(viewHref(view.id), '_blank')
		}
	];
	if (editable)
		items.push(
			{
				id: 'rename',
				label: 'Rename',
				run: () => {
					intents.ask('view', view.id, 'rename');
					return goto(resolve('/(app)/views/[id]', { id: view.id }));
				}
			},
			{
				id: 'filters',
				label: 'Filters and display',
				run: () => {
					intents.ask('view', view.id, 'filters');
					return goto(resolve('/(app)/views/[id]', { id: view.id }));
				}
			}
		);
	if (editable && view.ownerId === me())
		items.push({
			id: 'share',
			label: 'Share with the workspace',
			run: async () => {
				const id = await shareView(ws, view);
				toasts.show(`Shared ${view.name} with the workspace`);
				await goto(resolve('/(app)/views/[id]', { id }), { replace: true });
			}
		});
	if (may(canEditViews))
		items.push({
			id: 'duplicate',
			label: 'Duplicate',
			run: async () => {
				const id = await createView(ws, {
					name: `${view.name} (copy)`,
					type: view.type,
					projectId: view.projectId,
					ownerId: view.ownerId,
					filter: view.filter,
					display: view.display
				});
				toasts.show(`Made a copy of ${view.name}`, {
					action: { label: 'Open', run: () => void goto(resolve('/(app)/views/[id]', { id })) }
				});
			}
		});
	items.push({ id: 'link', label: 'Copy link', run: () => copyLink(viewHref(view.id)) });
	if (editable)
		items.push(sep, {
			id: 'delete',
			label: 'Delete',
			danger: true,
			run: () => tracked(`Deleted the view ${view.name}`, () => deleteView(ws, view.id))
		});
	return { name: view.name, kind: 'view actions', items };
}

/** A project's menu: in the sidebar, on the Projects page, and on its own title. */
export function projectMenu(ws: string, projectId: string): MenuSpec | null {
	const replica = engine.replica(ws);
	const project =
		replica?.project(projectId) ?? replica?.archivedProjects().find((p) => p.id === projectId);
	if (!project) return null;
	const organize = may(canOrganize);
	const sharable =
		!session.local &&
		!!session.workspace &&
		!session.workspace.isPersonal &&
		canShare(session.workspace.role);
	const ask = (what: string) => () => {
		intents.ask('project', project.id, what);
		return goto(resolve('/(app)/projects/[id]', { id: project.id }));
	};
	const items: MenuItem[] = [
		{
			id: 'open',
			label: 'Open',
			run: () => goto(resolve('/(app)/projects/[id]', { id: project.id }))
		},
		{
			id: 'tab',
			label: 'Open in a new tab',
			run: () => void window.open(projectHref(project.id), '_blank')
		},
		{
			id: 'wiki',
			label: 'Open its wiki',
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with a query
			run: () => goto(`${resolve('/(app)/projects/[id]', { id: project.id })}?tab=wiki`)
		}
	];
	// Reports leave archived projects out.
	if (!project.archivedAt)
		items.push({
			id: 'reports',
			label: 'Open its reports',
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with a query
			run: () => goto(`${resolve('/(app)/reports')}?project=${project.id}`)
		});
	// Time on the project itself, not on one of its tasks.
	if (!project.archivedAt) {
		const timer = replica?.runningTimer(me());
		const timing = timer?.projectId === project.id;
		items.push(
			{
				id: 'log-time',
				label: 'Log time',
				run: () =>
					// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with a query
					goto(`${resolve('/(app)/projects/[id]', { id: project.id })}?tab=time#log`)
			},
			{
				id: 'timer',
				label: timing ? 'Stop the timer' : 'Start a timer',
				run: async () => {
					const time = await import('./sync/time');
					if (timing && timer) await time.stopTimer(ws, timer);
					else await time.startProjectTimer(ws, project.id);
				}
			}
		);
	}
	if (organize)
		items.push(
			{ id: 'rename', label: 'Rename', run: ask('rename') },
			{
				id: 'color',
				label: 'Color and icon',
				sub: () => [
					...PALETTE.map((c) => ({
						id: c,
						label: cap(c),
						checked: project.color === c,
						run: () => updateProject(ws, project.id, { color: c })
					})),
					sep,
					{ id: 'icon', label: 'Choose an icon', run: ask('style') }
				]
			}
		);
	if (sharable) items.push({ id: 'share', label: 'Share with guests', run: ask('share') });
	items.push({
		id: 'calendar',
		label: 'Subscribe in a calendar app',
		run: () =>
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with a query
			goto(`${resolve('/(app)/settings/w/[ws]/calendar', { ws })}?project=${project.id}`)
	});
	if (may(canUseTemplates))
		items.push({ id: 'template', label: 'Save as a template', run: ask('template') });
	items.push({ id: 'link', label: 'Copy link', run: () => copyLink(projectHref(project.id)) });
	if (organize)
		items.push(
			sep,
			project.archivedAt
				? {
						id: 'unarchive',
						label: 'Unarchive',
						run: async () => {
							await setProjectArchived(ws, project.id, false);
							toasts.show(`${project.name} is back in use`);
						}
					}
				: {
						id: 'archive',
						label: 'Archive',
						run: async () => {
							await setProjectArchived(ws, project.id, true);
							toasts.show(`Archived ${project.name}`, {
								onundo: () => void setProjectArchived(ws, project.id, false)
							});
						}
					},
			{
				id: 'delete',
				label: 'Delete',
				danger: true,
				run: async () => {
					await setProjectDeleted(ws, project.id, true);
					toasts.show(`Deleted ${project.name}`, {
						onundo: () => void setProjectDeleted(ws, project.id, false)
					});
				}
			}
		);
	return { name: project.name, kind: 'project actions', items };
}

const KINDS: [StatusCategory, string][] = [
	['backlog', 'Backlog'],
	['todo', 'To do'],
	['in-progress', 'In progress'],
	['done', 'Done'],
	['canceled', 'Canceled']
];
/** A status column's menu on a board by status: what Edit statuses does, at the column. */
export function statusMenu(
	ws: string,
	projectId: string,
	statusId: string,
	title: string,
	addHere: () => void
): MenuSpec | null {
	const statuses = engine.replica(ws)?.statuses(projectId) ?? [];
	const status = statuses.find((s) => s.id === statusId);
	if (!status) return null;
	const at = statuses.indexOf(status);
	const items: MenuItem[] = [{ id: 'new', label: 'New task here', run: addHere }];
	if (may(canOrganize)) {
		items.push(
			{
				id: 'rename',
				label: 'Rename the status',
				run: async () => {
					document.getElementById('status-editor')?.showPopover();
					await tick();
					const field = document.querySelectorAll<HTMLInputElement>(
						'#status-editor input[aria-label="Status name"]'
					)[at];
					field?.focus();
					field?.select();
				}
			},
			{
				id: 'kind',
				label: 'Its kind',
				sub: () =>
					KINDS.map(([k, label]) => ({
						id: k,
						label,
						checked: status.category === k,
						run: () => updateStatus(ws, statusId, { category: k })
					}))
			}
		);
		if (at > 0)
			items.push({
				id: 'left',
				label: 'Move left',
				run: () => moveStatus(ws, projectId, statusId, -1)
			});
		if (at < statuses.length - 1)
			items.push({
				id: 'right',
				label: 'Move right',
				run: () => moveStatus(ws, projectId, statusId, 1)
			});
		if (statuses.length > 1)
			items.push('separator', {
				id: 'delete',
				label: 'Delete the status',
				danger: true,
				run: async () => {
					const { done, undo } = engine.track(() => deleteStatus(ws, projectId, statusId));
					await done;
					toasts.show(`Deleted the status ${title}; its tasks moved to another`, {
						onundo: () => void undo()
					});
				}
			});
	}
	return { name: title, kind: 'status actions', items };
}

/** A comment's menu: what its buttons do, and copying it. */
export function commentMenu(
	ws: string,
	taskId: string,
	c: Comment,
	edit: () => void,
	removeOthers: () => void
): MenuSpec {
	const mine = c.authorId === me();
	const items: MenuItem[] = [
		{
			id: 'copy',
			label: 'Copy text',
			run: async () => {
				if (await copyText(toDisplay(referencesToDisplay(c.body).text).text))
					toasts.show('Comment copied');
			}
		},
		{ id: 'link', label: 'Copy link to the task', run: () => copyLink(`/?task=${taskId}`) }
	];
	if (mine) items.push({ id: 'edit', label: 'Edit', run: edit });
	if (mine || may(canDeleteOthersComments))
		items.push(sep, {
			id: 'delete',
			label: 'Delete',
			danger: true,
			run: () => (mine ? void deleteComment(ws, c.id) : removeOthers())
		});
	return { name: `${session.nameOf(ws, c.authorId)}'s comment`, kind: 'comment actions', items };
}

/** A file's menu, on a task. */
export function fileMenu(ws: string, a: Attachment): MenuSpec {
	const url = fileUrl(ws, a.id);
	return {
		name: a.filename,
		kind: 'file actions',
		items: [
			{ id: 'open', label: 'Open', run: () => void window.open(url, '_blank', 'noopener') },
			{
				id: 'download',
				label: 'Download',
				run: () => {
					const link = document.createElement('a');
					link.href = url;
					link.download = a.filename;
					link.click();
				}
			},
			{ id: 'link', label: 'Copy link', run: () => copyLink(url) },
			sep,
			{
				id: 'delete',
				label: 'Delete',
				danger: true,
				run: () => tracked(`Removed ${a.filename}`, () => deleteAttachment(ws, a.id))
			}
		]
	};
}

/** A label's menu, on a task's row. */
export function labelMenu(ws: string, taskId: string, l: { id: string; name: string }): MenuSpec {
	return {
		name: l.name,
		kind: 'label actions',
		items: [
			{
				id: 'off',
				label: 'Take it off this task',
				run: () => setTaskLabel(ws, taskId, l.id, false)
			},
			{
				id: 'edit',
				label: 'Edit the label',

				run: () => goto(resolve('/(app)/settings/w/[ws]/labels', { ws }))
			}
		]
	};
}

/** A wiki page's menu, in the tree, doing what Wiki.svelte does. */
export function pageMenu(
	ws: string,
	p: {
		id: string;
		main: boolean;
		title: string;
		href: string;
		templates: boolean;
		open: () => unknown;
		newUnder: () => unknown;
		rename: () => unknown;
		history: () => unknown;
		template: () => unknown;
		remove: () => unknown;
	}
): MenuSpec {
	const items: MenuItem[] = [
		{ id: 'open', label: 'Open', run: p.open },
		{ id: 'tab', label: 'Open in a new tab', run: () => void window.open(p.href, '_blank') },
		{ id: 'new', label: 'New page under it', run: p.newUnder }
	];
	if (!p.main) items.push({ id: 'rename', label: 'Rename', run: p.rename });
	items.push(
		{ id: 'link', label: 'Copy link', run: () => copyLink(p.href) },
		{ id: 'history', label: 'History', run: p.history }
	);
	if (p.templates) items.push({ id: 'template', label: 'Save as a template', run: p.template });
	if (!p.main) items.push(sep, { id: 'delete', label: 'Delete', danger: true, run: p.remove });
	return { name: p.title, kind: 'page actions', items };
}
