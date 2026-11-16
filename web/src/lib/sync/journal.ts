// The edits on their way into IndexedDB, kept in localStorage first.
// Writing to IndexedDB takes a moment, and a page that reloads or closes
// in that moment aborts the write: an edit that already showed (a
// delete with its Undo toast) would be gone. localStorage is written at
// once, so each edit is kept here before it shows, cleared once
// IndexedDB has it, and put into IndexedDB the next time the engine
// starts if it never got there. An edit kept twice is harmless: the
// server takes an op once by its opId, and a row keeps each field's
// newest write (merge).
import type { Op, Row } from './replica.svelte';

const KEY = 'mila.journal';

export interface Entry {
	/** This entry, so the edit that wrote it can clear it. */
	key: string;
	ws: string;
	ops: Op[];
	rows: Row[];
	schema: number;
}

function read(): Entry[] {
	try {
		const v = JSON.parse(localStorage.getItem(KEY) ?? '[]');
		return Array.isArray(v)
			? v.filter(
					(e): e is Entry =>
						!!e &&
						typeof e.key === 'string' &&
						typeof e.ws === 'string' &&
						Array.isArray(e.ops) &&
						Array.isArray(e.rows)
				)
			: [];
	} catch {
		return [];
	}
}

function write(list: Entry[]) {
	if (list.length) localStorage.setItem(KEY, JSON.stringify(list));
	else localStorage.removeItem(KEY);
}

/**
 * Keeps an edit until IndexedDB has it. False when it could not be kept
 * (storage full or blocked): the edit then goes on as it always did.
 */
export function keep(entry: Entry): boolean {
	try {
		write([...read(), entry]);
		return true;
	} catch {
		return false;
	}
}

/** IndexedDB has the edit. */
export function done(key: string) {
	try {
		const list = read();
		const rest = list.filter((e) => e.key !== key);
		if (rest.length !== list.length) write(rest);
	} catch {
		// storage blocked: nothing was kept
	}
}

/** The edits kept and never cleared, oldest first. */
export function unfinished(): Entry[] {
	return read();
}

/** Forgets every edit kept: signing out, or once they are in IndexedDB. */
export function clear(keys?: string[]) {
	try {
		if (!keys) localStorage.removeItem(KEY);
		else write(read().filter((e) => !keys.includes(e.key)));
	} catch {
		// storage blocked
	}
}

/**
 * A kept row merged into the one IndexedDB holds: each field takes the
 * newer write, by its clock, so a kept edit never undoes a later one.
 */
export function merge(stored: Row | undefined, kept: Row): Row {
	if (!stored) return kept;
	const values = { ...stored.values };
	const clocks = { ...stored.clocks };
	for (const [name, clock] of Object.entries(kept.clocks)) {
		if (clocks[name] === undefined || clock > clocks[name]) {
			values[name] = kept.values[name];
			clocks[name] = clock;
		}
	}
	return { ...stored, values, clocks };
}
