// Keyboard control for a list of task rows: rows carry data-task-id, and
// this action on the list's container runs the commands in COMMANDS
// (keys/commands.ts) whose scope is 'list', on the keys the person has.
//
// The list is one stop for Tab (a roving tabindex): only the current row,
// and the controls in it, can be tabbed to; arrows and J/K move between
// rows. The current row is the one last focused, else the first.
//
// A task can be in a list twice (a saved view grouped by label shows it
// under each of its labels), so a row is known by its data-row-key
// ("group/task") where it has one, and by its task ID otherwise; its
// data-group names the group it moves within.
import type { Task } from './sync/entities';
import {
	placeTask,
	setInMyDay,
	setTaskCompleted,
	setTaskDeleted,
	setTaskFields
} from './sync/data';
import { session } from './session.svelte';
import { engine } from './sync/engine.svelte';
import { openTask } from './taskLink';
import { toasts } from './toasts.svelte';
import { localToday } from './sync/data';
import type { Priority } from './sync/entities';
import { announce } from './announce.svelte';
import type { CommandId } from './keys/commands';
import { keymap } from './keys/keymap.svelte';
import {
	ACTS_ON_MANY,
	SELECT_COMMANDS,
	clickSelects,
	commandSelects,
	sayCount,
	selection
} from './selection.svelte';

export interface TaskKeysOptions {
	ws: string;
	/** The rows in screen order, in groups; reordering stays in a group. */
	groups: () => { id: string; tasks: Task[] }[];
	/**
	 * What the move commands say instead of moving, when the list has no
	 * order of its own to change (a sorted list, or one mixing projects).
	 */
	fixed?: string;
}

const ROW = '[data-task-id]';

/** What identifies a row: its group and task, or just its task. */
const rowKey = (el: HTMLElement) => el.dataset.rowKey ?? el.dataset.taskId ?? '';

const PRIORITY_NAMES: Record<Priority, string> = {
	urgent: 'Urgent',
	high: 'High',
	medium: 'Medium',
	low: 'Low',
	none: 'No priority'
};

/** What a row holds that Tab could otherwise stop on. */
const INNER = 'a[href], button, input, select, textarea, [tabindex]';

const PRIORITY_OF: Partial<Record<CommandId, Priority>> = {
	'task.priority.urgent': 'urgent',
	'task.priority.high': 'high',
	'task.priority.medium': 'medium',
	'task.priority.low': 'low',
	'task.priority.none': 'none'
};

/**
 * The row to focus when the one at `at` goes away: the next, else the
 * previous, that is another task. A task can be in a list twice, and its
 * other copy goes away with it.
 */
function besideOf(all: HTMLElement[], at: number): HTMLElement | undefined {
	const id = all[at]?.dataset.taskId;
	const other = (r: HTMLElement) => r.dataset.taskId !== id;
	return all.slice(at + 1).find(other) ?? all.slice(0, at).findLast(other);
}

export function taskKeys(node: HTMLElement, options: TaskKeysOptions) {
	let opts = options;
	let current: string | null = null;

	function rows(): HTMLElement[] {
		return [...node.querySelectorAll<HTMLElement>(ROW)];
	}

	/** The row with a key (see rowKey), if it is drawn. */
	function rowFor(key: string): HTMLElement | null {
		const k = CSS.escape(key);
		return node.querySelector<HTMLElement>(`[data-row-key="${k}"], [data-task-id="${k}"]`);
	}

	// The row last given the tab stop, so a change deals only the rows it
	// touches: a list of thousands is not walked on every edit.
	let dealt: HTMLElement | null = null;

	/** Puts a row and its controls in the tab order, or takes them out. */
	function deal(r: HTMLElement, on: boolean) {
		r.tabIndex = on ? 0 : -1;
		for (const c of r.querySelectorAll<HTMLElement>(INNER)) {
			if (on && c.dataset.roved !== undefined) {
				c.removeAttribute('tabindex');
				delete c.dataset.roved;
			} else if (!on && c.tabIndex >= 0) {
				c.tabIndex = -1;
				c.dataset.roved = '';
			}
		}
	}

	/**
	 * Leaves the current row, and only it, in the tab order. With the
	 * changes that prompted it, only the rows they touched (and the old and
	 * new current row) are dealt again; with none, every row is.
	 */
	function rove(changes?: MutationRecord[]) {
		const active = (current && rowFor(current)) || node.querySelector<HTMLElement>(ROW);
		let touched: Iterable<HTMLElement> = rows();
		if (changes) {
			const set = new Set<HTMLElement>();
			for (const m of changes) {
				const inRow = (m.target as Element).closest?.<HTMLElement>(ROW);
				if (inRow) set.add(inRow);
				for (const n of m.addedNodes) {
					if (!(n instanceof HTMLElement)) continue;
					if (n.matches(ROW)) set.add(n);
					for (const r of n.querySelectorAll<HTMLElement>(ROW)) set.add(r);
				}
			}
			if (dealt) set.add(dealt);
			touched = set;
		}
		for (const r of touched) if (r !== active && node.contains(r)) deal(r, false);
		if (active) deal(active, true);
		dealt = active;
	}

	function onfocusin(e: FocusEvent) {
		const row = (e.target as HTMLElement).closest<HTMLElement>(ROW);
		if (!row || rowKey(row) === current) return;
		current = rowKey(row);
		rove([]);
	}

	function focusRow(key: string) {
		requestAnimationFrame(() => rowFor(key)?.focus());
	}

	/** The tasks the list shows, in order, each once: what a selection is made in. */
	function order(): string[] {
		return [...new Set(opts.groups().flatMap((g) => g.tasks.map((t) => t.id)))];
	}

	// Ctrl-click (Command-click on a Mac) and Shift-click select; so does a tap
	// while a phone is picking. Before the row's own buttons hear the click.
	function onclick(e: MouseEvent) {
		const row = (e.target as HTMLElement).closest<HTMLElement>(ROW);
		if (!row || (e.target as Element).closest('input, textarea, [contenteditable="true"]')) return;
		if (clickSelects(e, opts.ws, row.dataset.taskId!, order)) announce(sayCount());
	}
	// Shift and a press would select the row's text.
	function onmousedown(e: MouseEvent) {
		if (e.shiftKey && (e.target as HTMLElement).closest(ROW)) e.preventDefault();
	}

	function onkeydown(e: KeyboardEvent) {
		const row = (e.target as HTMLElement).closest<HTMLElement>(ROW);
		if (!row || e.target !== row) return;
		const cmd = keymap.match('list', e);
		if (!cmd) return;
		if (SELECT_COMMANDS.includes(cmd)) {
			const to = commandSelects(cmd, opts.ws, row.dataset.taskId!, order());
			// Escape with nothing selected is left to what else hears it.
			if (to === null) return;
			e.preventDefault();
			if (to !== row.dataset.taskId) rowFor(to)?.focus();
			announce(sayCount());
			return;
		}
		// The row's, even when it does nothing here (the first row's
		// previous), so a key kept on both a row and anywhere never does both.
		e.preventDefault();
		const id = row.dataset.taskId!;
		// While anything is selected, the task keys act on all of it.
		if (selection.on(opts.ws) && selection.shown().length && ACTS_ON_MANY.includes(cmd)) {
			void import('./manyActions').then((m) => m.runCommand(opts.ws, cmd));
			return;
		}
		const all = rows();
		const at = all.indexOf(row);
		const groups = opts.groups();
		const task = groups.flatMap((g) => g.tasks).find((t) => t.id === id);
		const priority = PRIORITY_OF[cmd];

		if (cmd === 'task.move-up' || cmd === 'task.move-down') {
			if (opts.fixed) {
				announce(opts.fixed);
				return;
			}
			const group = (
				row.dataset.group !== undefined
					? groups.find((g) => g.id === row.dataset.group)
					: groups.find((g) => g.tasks.some((t) => t.id === id))
			)?.tasks;
			if (!group) return;
			const i = group.findIndex((t) => t.id === id);
			const up = cmd === 'task.move-up';
			const to = up ? i - 1 : i + 1;
			if (to < 0 || to >= group.length) return;
			if (up) void placeTask(opts.ws, id, group[i - 2]?.sortKey ?? null, group[i - 1].sortKey);
			else void placeTask(opts.ws, id, group[i + 1].sortKey, group[i + 2]?.sortKey ?? null);
			announce(`Moved ${task?.title ?? 'the task'} to ${to + 1} of ${group.length}`);
			focusRow(rowKey(row));
		} else if (cmd === 'task.next') {
			all[Math.min(at + 1, all.length - 1)]?.focus();
		} else if (cmd === 'task.prev') {
			all[Math.max(at - 1, 0)]?.focus();
		} else if (cmd === 'task.open') {
			void openTask(id);
		} else if (cmd === 'task.complete' && task) {
			const completing = task.completedAt === null;
			const { undo } = engine.track(() => setTaskCompleted(opts.ws, task, completing));
			if (completing) toasts.show(`Completed "${task.title}"`, { onundo: () => void undo() });
			besideOf(all, at)?.focus();
		} else if (cmd === 'task.my-day' && session.user) {
			const replica = engine.replica(opts.ws);
			const on = !replica?.inMyDay(session.user.id, id, localToday());
			void setInMyDay(opts.ws, session.user.id, id, on);
			toasts.show(on ? 'Added to My Day' : 'Taken out of My Day');
		} else if (priority) {
			void setTaskFields(opts.ws, id, { priority });
			announce(`Priority: ${PRIORITY_NAMES[priority]}`);
		} else if (cmd === 'task.delete' && task) {
			void setTaskDeleted(opts.ws, id, true);
			toasts.show(`Deleted "${task.title}"`, {
				onundo: () => void setTaskDeleted(opts.ws, id, false)
			});
			besideOf(all, at)?.focus();
		}
	}

	// Rows come and go as the list changes; each change re-deals the stops.
	const watch = new MutationObserver((changes) => rove(changes));
	watch.observe(node, { childList: true, subtree: true });
	rove();
	const unregister = selection.register(order);
	node.addEventListener('keydown', onkeydown);
	node.addEventListener('focusin', onfocusin);
	node.addEventListener('click', onclick, { capture: true });
	node.addEventListener('mousedown', onmousedown);
	return {
		update(next: TaskKeysOptions) {
			opts = next;
		},
		destroy() {
			watch.disconnect();
			unregister();
			node.removeEventListener('keydown', onkeydown);
			node.removeEventListener('focusin', onfocusin);
			node.removeEventListener('click', onclick, { capture: true });
			node.removeEventListener('mousedown', onmousedown);
		}
	};
}
