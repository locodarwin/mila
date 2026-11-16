// Sync schema 5 made a pair's ID (a label on a task, an assignee) a hash of
// the pair rather than the two IDs XORed, and the server moved every row to
// its new ID. An unsent edit from before names the old one; most name the
// pair as well, and the outbox's upgrade renames those, but one that only
// changes a row (taking a label off) names neither. The rows this device
// holds from before still do, so at start, before a fresh copy replaces
// them, each such edit is renamed from its row, or from an earlier edit of
// the same row. The server renames whatever is left.
import { local, type Pending } from './db';
import { PAIRS, pairId, xorId, type Values } from './entities';
import { FIRST_SCHEMA } from './outbox';
import type { Op } from './replica.svelte';

/** What rekeyPairs reads and writes: the device's database, or a test's. */
export interface KeptEdits {
	/** The unsent edits, in the order they were made. */
	pending(): Promise<Pending[]>;
	/** A row as this device holds it. */
	values(ws: string, entity: string, id: string): Promise<Values | undefined>;
	/** Gives a kept edit its op as renamed. */
	rename(n: number, op: Op): Promise<void>;
}

const onDevice: KeptEdits = {
	pending: () => local.outbox.toArray(),
	values: async (ws, entity, id) => (await local.rows.get([ws, entity, id]))?.values,
	rename: async (n, op) => {
		await local.outbox.update(n, { op });
	}
};

export async function rekeyPairs(kept: KeptEdits = onDevice) {
	const moved = new Map<string, string>();
	for (const p of await kept.pending()) {
		const pair = PAIRS[p.op.entity];
		if (!pair || p.n === undefined || (p.schema ?? FIRST_SCHEMA) >= 5) continue;
		const key = `${p.ws}\n${p.op.entity}\n${p.op.id}`;
		let id = moved.get(key);
		if (!id) {
			let a = p.op.fields[pair[0]];
			let b = p.op.fields[pair[1]];
			if (typeof a !== 'string' || typeof b !== 'string') {
				const values = await kept.values(p.ws, p.op.entity, p.op.id);
				a = values?.[pair[0]];
				b = values?.[pair[1]];
			}
			if (typeof a !== 'string' || typeof b !== 'string' || p.op.id !== xorId(a, b)) continue;
			id = pairId(a, b);
			moved.set(key, id);
		}
		await kept.rename(p.n, { ...p.op, id });
	}
}
