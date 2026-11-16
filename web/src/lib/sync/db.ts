// This device's copy of every workspace, its unsent edits and how far it
// has pulled, in IndexedDB. Signing out deletes it all, and the edits
// journal.ts keeps on their way in.
import Dexie, { type Table } from 'dexie';
import { clear as clearJournal } from './journal';
import type { Op, Row } from './replica.svelte';

export interface StoredRow extends Row {
	ws: string;
}

export interface Pending {
	n?: number; // order of edits
	ws: string;
	op: Op;
	// The SYNC_SCHEMA it was made under; missing on edits saved before it
	// was kept (see outbox.ts). Not indexed, so no new schema version.
	schema?: number;
}

class LocalDB extends Dexie {
	rows!: Table<StoredRow, [string, string, string]>;
	// epoch is not indexed, so adding it needed no new schema version: a
	// copy saved before it simply has none, and bootstraps again.
	meta!: Table<{ ws: string; seq: number; epoch?: string }, string>;
	outbox!: Table<Pending, number>;

	constructor() {
		super('mila');
		this.version(1).stores({
			rows: '[ws+entity+id], ws',
			meta: 'ws',
			outbox: '++n, ws'
		});
	}
}

export const local = new LocalDB();

/** Deletes everything this device holds. */
export async function wipeLocal() {
	clearJournal();
	local.close();
	await Dexie.delete('mila');
	await local.open();
}
