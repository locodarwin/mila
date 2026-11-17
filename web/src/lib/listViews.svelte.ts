// How Inbox, My Day and Upcoming are set up on this device: the view (list,
// board, calendar, table or timeline), its filter and its display, per workspace,
// as a saved view would hold them. Kept in
// localStorage, like a project's table layout; signing out clears it.
import { applyPaths, type DisplayPath } from './displayPaths';
import type { ViewEdit } from './sync/data';
import type { ViewDisplay, ViewFilter } from './sync/entities';
import { onSignOut } from './forgetting';

export type FixedList = 'inbox' | 'myday' | 'upcoming';
export type ListType = 'list' | 'board' | 'calendar' | 'table' | 'timeline';
export const LIST_TYPES: ListType[] = ['list', 'board', 'calendar', 'table', 'timeline'];

export interface ListSetup {
	type: ListType;
	filter: ViewFilter;
	display: ViewDisplay;
}

export const freshSetup = (): ListSetup => ({ type: 'list', filter: {}, display: {} });

const keyOf = (ws: string, list: FixedList) => `mila.list.${ws}.${list}`;

/** A stored setup as far as it can be read; anything else is the list as it comes. */
function read(raw: string | null): ListSetup {
	const fresh = freshSetup();
	if (!raw) return fresh;
	try {
		const v = JSON.parse(raw) as Partial<ListSetup>;
		const isObject = (x: unknown): x is Record<string, unknown> =>
			typeof x === 'object' && x !== null && !Array.isArray(x);
		return {
			type: LIST_TYPES.includes(v.type as ListType) ? (v.type as ListType) : 'list',
			filter: isObject(v.filter) ? (v.filter as ViewFilter) : {},
			display: isObject(v.display) ? (v.display as ViewDisplay) : {}
		};
	} catch {
		return fresh;
	}
}

class ListViews {
	/** Forgets every setup held in memory, at sign-out. */
	forget() {
		this.#setups = {};
	}

	#setups = $state<Record<string, ListSetup>>({});

	get(ws: string, list: FixedList): ListSetup {
		const key = keyOf(ws, list);
		const known = this.#setups[key];
		if (known) return known;
		let raw: string | null = null;
		try {
			raw = localStorage.getItem(key);
		} catch {
			// Without storage each list comes as it is.
		}
		return read(raw);
	}

	set(ws: string, list: FixedList, setup: ListSetup) {
		const key = keyOf(ws, list);
		this.#setups[key] = setup;
		try {
			const plain = setup.type === 'list' && isEmpty(setup.filter) && isEmpty(setup.display);
			if (plain) localStorage.removeItem(key);
			else localStorage.setItem(key, JSON.stringify(setup));
		} catch {
			// Kept for this visit only.
		}
	}

	setType(ws: string, list: FixedList, type: ListType) {
		this.set(ws, list, { ...this.get(ws, list), type });
	}

	/** A filter change, as a saved view's: the parts set, undefined taking one away. */
	editFilter(ws: string, list: FixedList, edit: ViewEdit<ViewFilter, Partial<ViewFilter>>) {
		const setup = this.get(ws, list);
		const patch = typeof edit === 'function' ? edit(setup.filter) : edit;
		const filter: Record<string, unknown> = { ...setup.filter };
		for (const [k, v] of Object.entries(patch)) {
			if (v === undefined) delete filter[k];
			else filter[k] = v;
		}
		this.set(ws, list, { ...setup, filter: filter as ViewFilter });
	}

	editDisplay(
		ws: string,
		list: FixedList,
		edit: ViewEdit<ViewDisplay, Partial<Record<DisplayPath, unknown>>>
	): ViewDisplay {
		const setup = this.get(ws, list);
		const patch = typeof edit === 'function' ? edit(setup.display) : edit;
		const display = applyPaths(setup.display, patch);
		this.set(ws, list, { ...setup, display });
		return display;
	}
}

function isEmpty(o: object): boolean {
	return Object.keys(o).length === 0;
}

/** Whether a list is set up any other way than as it comes. */
export function changed(setup: ListSetup): boolean {
	return setup.type !== 'list' || !isEmpty(setup.filter) || !isEmpty(setup.display);
}

export const listViews = new ListViews();
onSignOut(() => listViews.forget());
