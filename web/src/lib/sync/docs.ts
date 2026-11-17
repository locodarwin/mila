// Live documents on the device: a task's notes as
// a Yjs document, built from the task's doc_update rows. Edits made here go
// out as new rows, at most one every half second while someone types and
// at once when the editor closes; rows that arrive from elsewhere are
// applied as they come. The server renders the document into the task's
// notes, which everything else reads.
import * as Y from 'yjs';
import {
	applyAwarenessUpdate,
	Awareness,
	encodeAwarenessUpdate,
	removeAwarenessStates
} from 'y-protocols/awareness';
import { uuid } from '../uuid';
import { engine } from './engine.svelte';
import { FRAGMENT } from './liveNotes';
import type { Row } from './replica.svelte';

export { editable, FRAGMENT } from './liveNotes';

/** How long typing gathers before it is sent, in milliseconds. */
export const SEND_EVERY = 500;

/** Marks updates that came from rows, so they are not sent back. */
const FROM_ROWS = Symbol('rows');

export function toBase64(bytes: Uint8Array): string {
	let s = '';
	for (let i = 0; i < bytes.length; i += 0x8000)
		s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
	return btoa(s);
}

export function fromBase64(text: string): Uint8Array {
	const s = atob(text);
	const out = new Uint8Array(s.length);
	for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
	return out;
}

/** What a live document belongs to: a task's notes, or a page's body. */
export interface DocTarget {
	kind: 'task' | 'page';
	id: string;
}

/** The entity of a document's rows, and the field naming what they belong to. */
export const ROWS = {
	task: { entity: 'doc_update', field: 'task_id' },
	page: { entity: 'page_update', field: 'page_id' }
} as const;

export class LiveDoc {
	readonly ydoc = new Y.Doc();
	#applied = new Set<string>();
	#pending: Uint8Array[] = [];
	#timer: ReturnType<typeof setTimeout> | undefined;
	#closed = false;
	// A page going away (a reload, a closed tab, a phone putting it aside) never
	// reaches the next send, so what was typed goes now: engine.mutate keeps it on
	// the device before it returns.
	#leaving = () => {
		if (document.visibilityState === 'hidden') void this.flush();
	};

	constructor(
		readonly ws: string,
		readonly target: DocTarget,
		/** Sends a row; engine.mutate by default, left out of the app's undo. */
		readonly send: (id: string, data: string) => Promise<unknown> = (id, data) =>
			engine.mutate(
				ws,
				ROWS[target.kind].entity,
				id,
				{ [ROWS[target.kind].field]: target.id, data, deleted_at: null },
				{ undoable: false }
			)
	) {
		this.ydoc.on('update', (update: Uint8Array, origin: unknown) => {
			if (origin === FROM_ROWS || this.#closed) return;
			this.#pending.push(update);
			clearTimeout(this.#timer);
			this.#timer = setTimeout(() => void this.flush(), SEND_EVERY);
		});
		if (typeof window !== 'undefined') {
			window.addEventListener('pagehide', this.#leaving);
			document.addEventListener('visibilitychange', this.#leaving);
		}
	}

	get fragment(): Y.XmlFragment {
		return this.ydoc.getXmlFragment(FRAGMENT);
	}

	/** Applies the rows not applied yet: this device's own are skipped. */
	sync(rows: Row[]) {
		Y.transact(
			this.ydoc,
			() => {
				for (const r of rows) {
					if (this.#applied.has(r.id)) continue;
					this.#applied.add(r.id);
					const data = r.values.data;
					if (typeof data !== 'string' || !data) continue;
					try {
						Y.applyUpdate(this.ydoc, fromBase64(data), FROM_ROWS);
					} catch (err) {
						console.warn('Mila: a row of a document could not be read', r.id, err);
					}
				}
			},
			FROM_ROWS
		);
	}

	/** Sends what has been typed since the last row, as one row. */
	async flush() {
		clearTimeout(this.#timer);
		this.#timer = undefined;
		if (!this.#pending.length) return;
		const update = this.#pending.length === 1 ? this.#pending[0] : Y.mergeUpdates(this.#pending);
		this.#pending = [];
		const id = uuid();
		this.#applied.add(id);
		await this.send(id, toBase64(update));
	}

	/** Sends what is left and lets the document go. */
	async close() {
		if (typeof window !== 'undefined') {
			window.removeEventListener('pagehide', this.#leaving);
			document.removeEventListener('visibilitychange', this.#leaving);
		}
		await this.flush();
		this.#closed = true;
		this.ydoc.destroy();
	}
}

/** How often this device says where its cursor is, at most, in milliseconds. */
const SAY_EVERY = 100;

/**
 * Who has a live document open, and where their cursors are: a Yjs
 * awareness, told to the other devices over the socket. The server stamps
 * each state with its sender's user ID (state.user.id), so that is what
 * says who someone is, not anything a device claims.
 */
export class DocPresence {
	readonly awareness: Awareness;
	#channel: { say: (update: string) => void; leave: () => void };
	#timer: ReturnType<typeof setTimeout> | undefined;

	constructor(ws: string, doc: LiveDoc) {
		this.awareness = new Awareness(doc.ydoc);
		this.#channel = engine.presence(
			ws,
			`${doc.target.kind}:${doc.target.id}`,
			(update) => applyAwarenessUpdate(this.awareness, fromBase64(update), 'remote'),
			() => this.#say()
		);
		this.awareness.on('update', (_: unknown, origin: unknown) => {
			if (origin === 'remote' || this.#timer) return;
			this.#timer = setTimeout(() => {
				this.#timer = undefined;
				this.#say();
			}, SAY_EVERY);
		});
		this.#say();
	}

	#say() {
		const me = this.awareness.clientID;
		if (this.awareness.getStates().has(me))
			this.#channel.say(toBase64(encodeAwarenessUpdate(this.awareness, [me])));
	}

	/** The user IDs of everyone else who has the document open. */
	others(): string[] {
		const out = new Set<string>();
		for (const [client, state] of this.awareness.getStates()) {
			const id = (state.user as { id?: unknown } | undefined)?.id;
			if (client !== this.awareness.clientID && typeof id === 'string') out.add(id);
		}
		return [...out];
	}

	/** Says this device has left, and stops. */
	close() {
		clearTimeout(this.#timer);
		const me = this.awareness.clientID;
		removeAwarenessStates(this.awareness, [me], 'local');
		this.#channel.say(toBase64(encodeAwarenessUpdate(this.awareness, [me])));
		this.#channel.leave();
		this.awareness.destroy();
	}
}
