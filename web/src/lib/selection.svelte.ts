// Selecting several tasks (docs/PLAN.md section 3): which tasks are selected,
// in the lists, boards and tables that show tasks. This is the small part every
// list loads; what a selection can be changed with (manyActions.ts) and the bar
// (components/SelectionBar.svelte) load once something is selected.
//
// A list, board or table registers what it shows, in order, while it is on the
// page. The selection is the IDs picked, and what counts is those the lists
// still show: a task moved, filtered out or deleted elsewhere leaves it, and
// comes back with an undo.
/* eslint-disable svelte/prefer-svelte-reactivity -- each set is made afresh and replaces the old one whole, which is what is reactive; none is changed in place once stored */
import { untrack } from 'svelte';
import { isApple } from './device';
import type { CommandId } from './keys/commands';

/** A list of tasks to select in: the task IDs it shows, in screen order. */
export type Order = () => string[];

/** The commands that select (keys/commands.ts). */
export const SELECT_COMMANDS: readonly CommandId[] = [
	'select.toggle',
	'select.down',
	'select.up',
	'select.all',
	'select.clear'
];

class Selection {
	/** The workspace the selection is in. */
	ws = $state<string | null>(null);
	#ids = $state.raw<ReadonlySet<string>>(new Set());
	/** On a phone, after the menu's Select: a tap selects, until Done. */
	picking = $state(false);
	/** Pick a date was chosen: the bar asks for one date for all. */
	askDate = $state(false);
	/** Where Shift-click and Shift with an arrow stretch from. */
	#anchor: string | null = null;
	/** What was selected before the stretch from the anchor began. */
	#base: ReadonlySet<string> = new Set();
	#lists = $state.raw<ReadonlySet<Order>>(new Set());

	/** The IDs picked, shown or not. */
	get ids(): ReadonlySet<string> {
		return this.#ids;
	}

	/** Whether a task is selected, in a workspace. */
	has(ws: string, id: string): boolean {
		return this.ws === ws && this.#ids.has(id);
	}

	/** Whether there is a selection the page shows, in this workspace (or a phone is picking). */
	on(ws: string): boolean {
		return this.ws === ws && (this.picking || this.#shown.length > 0);
	}

	/** The selected tasks the page shows, in its order, each once; worked out once per change. */
	#shown = $derived.by(() => {
		const ids = this.#ids;
		if (!ids.size) return [];
		const seen = new Set<string>();
		for (const order of this.#lists) for (const id of order()) if (ids.has(id)) seen.add(id);
		return [...seen];
	});

	/** The selected tasks the page shows, in its order, each once. */
	shown(): string[] {
		return this.#shown;
	}

	/** Every task the page's lists show, each once: what All on a phone selects. */
	everything(): string[] {
		const seen = new Set<string>();
		for (const order of this.#lists) for (const id of order()) seen.add(id);
		return [...seen];
	}

	/** A list, board or table showing tasks; returns what takes it off. */
	register(order: Order): () => void {
		// Untracked, so an effect that registers never reruns on the list it writes.
		untrack(() => (this.#lists = new Set([...this.#lists, order])));
		return () =>
			untrack(() => {
				const next = new Set(this.#lists);
				next.delete(order);
				// Going to another page clears the selection (the app layout); a
				// list that empties and comes back with an undo keeps it.
				this.#lists = next;
			});
	}

	#set(ws: string, ids: ReadonlySet<string>) {
		if (this.ws !== ws) this.picking = false;
		this.ws = ws;
		this.#ids = ids;
	}

	/** Adds a task, or takes it out. */
	toggle(ws: string, id: string) {
		const next = new Set(this.ws === ws ? this.#ids : []);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		this.#set(ws, next);
		this.#anchor = id;
		this.#base = next;
	}

	/** Adds a task (the menu's Select). */
	add(ws: string, id: string) {
		if (!this.has(ws, id)) this.toggle(ws, id);
	}

	/**
	 * Selects everything from the anchor to a task, as the list shows them,
	 * beside what was selected before; going back takes off what was passed.
	 */
	extend(ws: string, order: string[], to: string) {
		if (this.ws !== ws || !this.#anchor || !order.includes(this.#anchor)) {
			this.#anchor = to;
			this.#base = this.ws === ws ? this.#ids : new Set();
		}
		const a = order.indexOf(this.#anchor!);
		const b = order.indexOf(to);
		const next = new Set(this.#base);
		for (const id of order.slice(Math.min(a, b), Math.max(a, b) + 1)) next.add(id);
		this.#set(ws, next);
	}

	/** Starts a stretch at a task, keeping what is selected. */
	from(ws: string, id: string) {
		this.#base = this.ws === ws ? this.#ids : new Set();
		this.#anchor = id;
		if (this.ws !== ws) this.#set(ws, new Set());
	}

	/** Selects every task a list holds. */
	all(ws: string, order: string[]) {
		const next = new Set([...(this.ws === ws ? this.#ids : []), ...order]);
		this.#set(ws, next);
		this.#base = next;
	}

	clear() {
		this.#ids = new Set();
		this.#anchor = null;
		this.#base = new Set();
		this.picking = false;
		this.askDate = false;
	}

	/** Starts picking on a phone, from a task's menu. */
	pick(ws: string, id: string) {
		this.add(ws, id);
		this.picking = true;
	}
}

export const selection = new Selection();

/** Whether a click adds to the selection: Ctrl, or Command on Apple devices (where Ctrl-click is a right-click). */
export function addsToSelection(e: MouseEvent): boolean {
	return isApple() ? e.metaKey : e.ctrlKey;
}

/**
 * A click on a task in a list: with Ctrl or Command it adds or takes out, with
 * Shift it stretches the selection, and on a phone that is picking a plain tap
 * adds or takes out. Returns whether it selected, and then the click does nothing
 * else.
 */
export function clickSelects(e: MouseEvent, ws: string, id: string, order: Order): boolean {
	if (e.button !== 0) return false;
	if (e.shiftKey && !e.altKey && !addsToSelection(e)) {
		selection.extend(ws, order(), id);
	} else if (addsToSelection(e) && !e.shiftKey && !e.altKey) {
		selection.toggle(ws, id);
	} else if (selection.picking && selection.ws === ws && !e.shiftKey) {
		selection.toggle(ws, id);
	} else return false;
	e.preventDefault();
	e.stopPropagation();
	return true;
}

/**
 * A select command on a task. Returns the task focus moves to (Shift and an
 * arrow), the task itself, or null when the command did nothing here (Escape
 * with nothing selected), so the key does what it otherwise would.
 */
export function commandSelects(
	cmd: CommandId,
	ws: string,
	id: string,
	order: string[]
): string | null {
	switch (cmd) {
		case 'select.toggle':
			selection.toggle(ws, id);
			return id;
		case 'select.down':
		case 'select.up': {
			const at = order.indexOf(id);
			const to =
				order[Math.max(0, Math.min(order.length - 1, at + (cmd === 'select.down' ? 1 : -1)))];
			if (!to) return null;
			// From the focused task, when the stretch isn't already running through it.
			if (!selection.has(ws, id)) selection.from(ws, id);
			selection.extend(ws, order, to);
			return to;
		}
		case 'select.all':
			selection.all(ws, order);
			return id;
		case 'select.clear':
			if (selection.ws !== ws || (!selection.ids.size && !selection.picking)) return null;
			selection.clear();
			return id;
	}
	return null;
}

/** The task keys that act on the whole selection while there is one (manyActions.ts). */
export const ACTS_ON_MANY: readonly CommandId[] = [
	'task.complete',
	'task.my-day',
	'task.delete',
	'task.priority.urgent',
	'task.priority.high',
	'task.priority.medium',
	'task.priority.low',
	'task.priority.none'
];

/** What the selection is, said after it changes. */
export function sayCount(): string {
	const n = selection.shown().length;
	return n ? `${n} selected` : 'Selection cleared';
}
