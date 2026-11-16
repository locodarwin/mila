// The unsent edits of a session that ended while the app was open. Everything
// else on the device is deleted then (signout.ts), but these exist nowhere
// else: they wait here for the same person to sign in again, and go when
// anyone else does. Only what the person typed, and only until then.
import { local, type Pending } from './db';

const KEY = 'mila.unsent';

/** Keeps these edits for this person; false if the browser would not hold them. */
export function hold(user: string, edits: Pending[]): boolean {
	try {
		localStorage.setItem(KEY, JSON.stringify({ user, edits }));
		return true;
	} catch {
		return false;
	}
}

export function dropHeld() {
	try {
		localStorage.removeItem(KEY);
	} catch {
		// Without storage nothing was held.
	}
}

/**
 * Puts what was held for this person back among the unsent edits, for the
 * engine's start to send, and says how many. A workspace they have since
 * left takes its edits with it.
 */
export async function release(user: string, workspaces: string[]): Promise<number> {
	let held: { user?: string; edits?: Pending[] } | null = null;
	try {
		held = JSON.parse(localStorage.getItem(KEY) ?? 'null');
	} catch {
		// Unreadable is the same as none.
	}
	dropHeld();
	if (!held || held.user !== user || !Array.isArray(held.edits)) return 0;
	const edits = held.edits
		.filter((p) => workspaces.includes(p.ws))
		.map(({ ws, op, schema }) => ({ ws, op, schema }));
	if (edits.length) await local.outbox.bulkAdd(edits);
	return edits.length;
}
