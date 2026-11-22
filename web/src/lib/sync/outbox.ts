// Unsent edits made by an older version of the app. Each is kept with the
// SYNC_SCHEMA it was made under, and a newer app rewrites it into its own
// shape before sending: the edits held back by "update required" are sent
// after the reload as the server now expects them, not as they were made.
import type { Pending } from './db';
import { PAIRS, pairId, SYNC_SCHEMA, xorId } from './entities';
import type { Op } from './replica.svelte';

/** The version of edits saved before the version was kept with them. */
export const FIRST_SCHEMA = 1;

/**
 * UPGRADES[n] rewrites an op made under version n into version n + 1 (a
 * renamed field moved, a new required one filled). Raising SYNC_SCHEMA
 * means adding the step from the version before it; outbox.spec.ts fails
 * until there is one.
 */
export const UPGRADES: Record<number, (op: Op) => Op> = {
	// 2 added doc_update, the rows of live documents. An edit kept from
	// version 1 is sent as it is: a write to a task's notes is turned into
	// a row of its document by the server.
	1: (op) => op,
	// 3 is the wiki's formatting beyond Markdown:
	// columns, foldable sections, views and the rest, which an app of 2
	// can't hold and would delete from any document it opened. Reads now
	// name the version too, so an app of 2 stops before it sees one, and
	// what it kept from before then is sent as it is.
	2: (op) => op,
	// 4 gave a view in a page its own layout (a board or a table), which an app
	// of 3 would take off it when it edited the page. What it kept is sent as it is.
	3: (op) => op,
	// 5 made a pair's ID a hash of the pair rather than the two IDs XORed. An
	// edit that names both is renamed here; rekeyPairs (rekey.ts) renames the
	// rest at start from the rows on the device, and the server knows each
	// old ID of a row it had, so an edit it never renamed still lands.
	4: (op) => {
		const pair = PAIRS[op.entity];
		const a = pair && op.fields[pair[0]];
		const b = pair && op.fields[pair[1]];
		if (typeof a !== 'string' || typeof b !== 'string' || op.id !== xorId(a, b)) return op;
		return { ...op, id: pairId(a, b) };
	},
	// 6 added the report view and a report in a page, which an app of 5 can't
	// show and would drop from a page it edited. What it kept is sent as it is.
	5: (op) => op,
	// 7 added drawings in pages (page_drawing and drawing_shape), and the block
	// naming one, which an app of 6 would drop from a page it edited. What it
	// kept is sent as it is.
	6: (op) => op
};

/**
 * The op as version `to` expects it, or null when a newer app made it (in
 * another tab, say) and this one cannot know its shape.
 */
export function upgrade(
	p: Pending,
	to: number = SYNC_SCHEMA,
	steps: Record<number, (op: Op) => Op> = UPGRADES
): Op | null {
	let version = p.schema ?? FIRST_SCHEMA;
	if (version > to) return null;
	let op = p.op;
	for (; version < to; version++) {
		const step = steps[version];
		if (!step) throw new Error(`Mila cannot send edits made under sync schema ${version}.`);
		op = step(op);
	}
	return op;
}
