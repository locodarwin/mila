// The sync engine: keeps every workspace's replica in IndexedDB, sends
// local edits from the outbox, pulls changes when the server pokes, and
// carries on offline. docs/PLAN.md section 7 describes the protocol.
import { uuid } from '../uuid';
import { SvelteMap } from 'svelte/reactivity';
import { api, ApiError, call } from '../api/client';
import { local, type Pending } from './db';
import * as journal from './journal';
import { PAIRS, SYNC_SCHEMA, type Entity, type Values } from './entities';
import { Clock } from './hlc';
import { upgrade } from './outbox';
import { rekeyPairs } from './rekey';
import { forgetFiles } from '../fileCache';
import { markSynced } from '../perf';
import { toasts } from '../toasts.svelte';
import { Replica, type Op, type OpResult, type Row } from './replica.svelte';

const DEVICE_KEY = 'mila.device';
const PUSH_BATCH = 200;
/** How many undo steps are kept: a step is one edit, or a group of them. */
const UNDO_STEPS = 100;

function deviceId(): string {
	try {
		let id = localStorage.getItem(DEVICE_KEY);
		if (!id) {
			id = uuid().replaceAll('-', '').slice(0, 12);
			localStorage.setItem(DEVICE_KEY, id);
		}
		return id;
	} catch {
		return uuid().replaceAll('-', '').slice(0, 12);
	}
}

// paused: the server wants a newer app, and nothing is sent until a reload.
export type SyncState = 'synced' | 'syncing' | 'offline' | 'paused';

class SyncEngine {
	replicas = new SvelteMap<string, Replica>();
	pending = $state(0);
	/** True once the server has asked for a newer app; nothing syncs until a reload. */
	updateRequired = $state(false);
	#busy = $state(0);
	#online = $state(true);
	state: SyncState = $derived(
		this.updateRequired
			? 'paused'
			: !this.#online
				? 'offline'
				: this.#busy > 0
					? 'syncing'
					: 'synced'
	);

	/** Called when the server says this device is no longer signed in. */
	onSignedOut: () => void = () => {};

	/**
	 * Called when the server refuses to sync with this version of the app.
	 * Sync stops, keeping every unsent edit, until the page reloads.
	 */
	onUpdateRequired: () => void = () => {};
	/** The person the edits are made as, sent with each push. */
	user: string | null = null;
	/** The browser signed in as someone else since: the page must start again. */
	onOtherUser: () => void = () => {};

	#clock = new Clock(deviceId());
	#socket: WebSocket | null = null;
	#retry = 0;
	/** The server put off a push (too much written, or its disk low). */
	#held = false;
	#retryTimer: ReturnType<typeof setTimeout> | undefined;
	#flushTimer: ReturnType<typeof setTimeout> | undefined;
	#running = false;
	/** Whether it syncs: started, and not stopped since (signing out stops it). */
	get running() {
		return this.#running;
	}
	#seq = new Map<string, number>();
	// The sync epoch each workspace's copy came with (see pull).
	#epoch = new Map<string, string>();
	#onlineListener = () => this.#reconnect(0);

	/** Loads and syncs the given workspaces, and stays connected. */
	async start(workspaceIds: string[]) {
		this.#running = true;
		window.addEventListener('online', this.#onlineListener);
		await this.#recover().catch((err) => console.warn('Mila: kept edits not recovered:', err));
		await rekeyPairs().catch((err) => console.warn('Mila: kept edits not renamed:', err));
		await this.setWorkspaces(workspaceIds);
		this.pending = await local.outbox.count();
		this.#connect();
		void this.flush();
	}

	/** Disconnects and forgets everything in memory. */
	stop() {
		this.#running = false;
		window.removeEventListener('online', this.#onlineListener);
		clearTimeout(this.#retryTimer);
		clearTimeout(this.#flushTimer);
		this.#socket?.close();
		this.#socket = null;
		this.replicas.clear();
		this.#samples.clear();
		this.#seq.clear();
		this.#epoch.clear();
		this.pending = 0;
	}

	/**
	 * Makes the replicas match the user's workspaces: loads new ones, drops
	 * (and deletes locally) ones they no longer belong to, including any this
	 * device still holds from before they were removed while it was closed.
	 */
	async setWorkspaces(ids: string[]) {
		const held = await local.meta.toCollection().primaryKeys();
		for (const ws of new Set([...this.replicas.keys(), ...held])) {
			if (!ids.includes(ws) && !this.#samples.has(ws)) {
				this.replicas.delete(ws);
				this.#seq.delete(ws);
				this.#epoch.delete(ws);
				await local.transaction('rw', local.rows, local.meta, local.outbox, async () => {
					await local.rows.where('ws').equals(ws).delete();
					await local.meta.delete(ws);
					await local.outbox.where('ws').equals(ws).delete();
				});
				void forgetFiles(ws);
			}
		}
		await Promise.all(ids.filter((ws) => !this.replicas.has(ws)).map((ws) => this.#load(ws)));
	}

	replica(ws: string | undefined): Replica | undefined {
		return ws ? this.replicas.get(ws) : undefined;
	}

	/**
	 * Whether this device holds a copy of the workspace from the server:
	 * one kept from an earlier visit, or a first sync done. A replica is
	 * empty until then.
	 */
	hasCopy(ws: string): boolean {
		return this.#seq.has(ws) || this.#samples.has(ws);
	}

	// Sample workspaces, for the style guide: held in memory only, so the
	// components that show them work as usual while nothing done to them is
	// saved on the device or sent.
	#samples = new Set<string>();

	/** Makes a sample workspace from rows; see #samples. */
	sample(ws: string, rows: Row[]): Replica {
		const replica = new Replica(ws, this.#clock);
		replica.replaceAll(rows);
		this.#samples.add(ws);
		this.replicas.set(ws, replica);
		return replica;
	}

	dropSample(ws: string) {
		if (this.#samples.delete(ws)) this.replicas.delete(ws);
	}

	async #load(ws: string) {
		const replica = new Replica(ws, this.#clock);
		const meta = await local.meta.get(ws);
		const rows = await local.rows.where('ws').equals(ws).toArray();
		replica.replaceAll(rows);
		this.replicas.set(ws, replica);
		if (meta) {
			this.#seq.set(ws, meta.seq);
			if (meta.epoch) this.#epoch.set(ws, meta.epoch);
			void this.pull(ws);
		} else {
			await this.#bootstrap(ws).catch((err) => this.#failed(err));
		}
	}

	#edits: Promise<unknown> = Promise.resolve();

	// Undo: each edit records the values it replaced. Edits made inside
	// group() undo together.
	#undo: { ws: string; entity: Entity; id: string; before: Values; group: number }[] = [];
	#group = 0;
	#nextGroup = 1;
	canUndo = $state(false);

	/** Runs several edits that one undo should reverse together. */
	group<T>(fn: () => Promise<T>): Promise<T> {
		return this.#inGroup(this.#group || this.#nextGroup++, fn);
	}

	async #inGroup<T>(group: number, fn: () => Promise<T>): Promise<T> {
		const outer = this.#group;
		this.#group = group;
		try {
			return await fn();
		} finally {
			this.#group = outer;
		}
	}

	/**
	 * Runs edits as one group, like group(), and returns at once with the
	 * work and what reverses exactly those edits: the undo on a toast,
	 * which must not take back whatever was edited after it.
	 */
	track(fn: () => Promise<unknown>): { done: Promise<unknown>; undo: () => Promise<boolean> } {
		const group = this.#group || this.#nextGroup++;
		const done = this.#inGroup(group, fn);
		const undo = async () => {
			// An undo pressed before the edit has landed waits for it.
			await done.catch(() => {});
			return this.#undoGroup(group);
		};
		return { done, undo };
	}

	/**
	 * Keeps the last UNDO_STEPS steps, each whole: a change to many tasks is
	 * one step however many rows it touched, and is never cut in half.
	 */
	#trimUndo() {
		const groups = new Set(this.#undo.map((s) => s.group));
		if (groups.size <= UNDO_STEPS) return;
		const drop = new Set([...groups].slice(0, groups.size - UNDO_STEPS));
		this.#undo = this.#undo.filter((s) => !drop.has(s.group));
	}

	/** Reverses the last edit, or group of edits. */
	async undo(): Promise<boolean> {
		const last = this.#undo.at(-1);
		return last ? this.#undoGroup(last.group) : false;
	}

	async #undoGroup(group: number): Promise<boolean> {
		const steps = this.#undo.filter((s) => s.group === group).reverse();
		if (!steps.length) return false;
		this.#undo = this.#undo.filter((s) => s.group !== group);
		this.canUndo = this.#undo.length > 0;
		for (const step of steps) {
			await this.mutate(step.ws, step.entity, step.id, step.before, { undoable: false });
		}
		return true;
	}

	/**
	 * Records a local edit. It is kept on this device before it shows (in
	 * the journal at once, then in IndexedDB), so a reload or a closed tab
	 * can never lose it, and sent when possible. Edits apply in the order
	 * made, so two quick edits to one row both count, and fields worked
	 * out from other rows see every earlier edit. The promise settles once
	 * the edit shows and is kept; if the journal could not keep it, once
	 * IndexedDB has it, as flush() waits for.
	 */
	mutate(
		ws: string,
		entity: Entity,
		id: string,
		fields: Values | ((replica: Replica) => Values),
		opts: { undoable?: boolean } = {}
	): Promise<void> {
		const group = this.#group || this.#nextGroup++;
		let staged: { op: Op; row: Row };
		let kept: boolean;
		try {
			const replica = this.replicas.get(ws);
			if (!replica) throw new Error('That workspace is not loaded.');
			// Fields that depend on other rows, like a sort key after the last
			// task, are worked out from the replica, which holds every earlier edit.
			const values = typeof fields === 'function' ? fields(replica) : fields;
			const old = replica.get(entity, id);
			staged = replica.prepare(entity, id, values);
			if (opts.undoable !== false) {
				// A new row is undone by deleting it.
				const before: Values = old
					? Object.fromEntries(Object.keys(values).map((k) => [k, old.values[k] ?? null]))
					: { deleted_at: new Date().toISOString() };
				this.#undo.push({ ws, entity, id, before, group });
				this.#trimUndo();
				this.canUndo = true;
			}
			if (this.#samples.has(ws)) {
				replica.apply(staged.row);
				return Promise.resolve();
			}
			kept = journal.keep({
				key: staged.op.opId,
				ws,
				ops: [staged.op],
				rows: [staged.row],
				schema: SYNC_SCHEMA
			});
			replica.apply(staged.row);
		} catch (err) {
			return Promise.reject(err);
		}
		const { op, row } = staged;
		return this.#settle(kept, async () => {
			await local.transaction('rw', local.rows, local.outbox, async () => {
				await local.rows.put({ ws, ...row });
				await local.outbox.add({ ws, op, schema: SYNC_SCHEMA });
			});
			journal.done(op.opId);
			this.pending++;
			this.#scheduleFlush();
		});
	}

	/**
	 * Writes an edit to IndexedDB after every earlier one. An edit the
	 * journal kept is settled already; one it could not keep settles when
	 * the write does, and fails with it.
	 */
	#settle(kept: boolean, write: () => Promise<void>): Promise<void> {
		const next = this.#edits.then(write, write);
		this.#edits = next.catch(() => {});
		if (!kept) return next;
		next.catch((err) =>
			console.warn('Mila: an edit waits in the journal for the next start:', err)
		);
		return Promise.resolve();
	}

	/**
	 * Runs one change to many tasks (a selection's) as one undo step whose
	 * ops share a batch ID: rules still run for each task but count it once,
	 * and a person it assigns several tasks gets one push. Returns the work
	 * and what reverses exactly it, as track() does; the undo is a batch too.
	 */
	batch(
		ws: string,
		fn: () => Promise<unknown>
	): { done: Promise<unknown>; undo: () => Promise<boolean> } {
		const inBatch = async <T>(work: () => Promise<T>): Promise<T> => {
			const replica = this.replicas.get(ws);
			const outer = replica?.batch ?? null;
			if (replica) replica.batch = outer ?? uuid();
			try {
				return await work();
			} finally {
				if (replica) replica.batch = outer;
			}
		};
		const { done, undo } = this.track(() => inBatch(fn));
		return { done, undo: () => inBatch(undo) };
	}

	/**
	 * Runs many edits as one piece of bulk work, an import's, so they start no
	 * automation.
	 */
	async bulkly<T>(ws: string, work: () => Promise<T>): Promise<T> {
		const replica = this.replicas.get(ws);
		if (replica) replica.bulk = true;
		try {
			return await work();
		} finally {
			if (replica) replica.bulk = false;
		}
	}

	/**
	 * Saves many new rows at once, in one transaction on this device, in
	 * the order given (a project before its tasks): what a template makes.
	 * They are not undone one by one; the caller undoes them as a whole
	 * (deleting the project or task made).
	 */
	insert(ws: string, rows: { entity: Entity; id: string; values: Values }[]): Promise<void> {
		let prepared: { op: Op; row: Row }[];
		let key = '';
		let kept = false;
		try {
			const replica = this.replicas.get(ws);
			if (!replica) throw new Error('That workspace is not loaded.');
			replica.bulk = true;
			try {
				prepared = rows.map((r) => replica.prepare(r.entity, r.id, r.values));
			} finally {
				replica.bulk = false;
			}
			if (!this.#samples.has(ws) && prepared.length) {
				key = prepared[0].op.opId;
				kept = journal.keep({
					key,
					ws,
					ops: prepared.map((p) => p.op),
					rows: prepared.map((p) => p.row),
					schema: SYNC_SCHEMA
				});
			}
			for (const { row } of prepared) replica.apply(row);
		} catch (err) {
			return Promise.reject(err);
		}
		if (this.#samples.has(ws)) return Promise.resolve();
		return this.#settle(kept, async () => {
			await local.transaction('rw', local.rows, local.outbox, async () => {
				await local.rows.bulkPut(prepared.map(({ row }) => ({ ws, ...row })));
				await local.outbox.bulkAdd(prepared.map(({ op }) => ({ ws, op, schema: SYNC_SCHEMA })));
			});
			if (key) journal.done(key);
			this.pending += prepared.length;
			this.#scheduleFlush();
		});
	}

	/**
	 * Puts edits the journal kept, and IndexedDB never got (the page went
	 * before it could), into IndexedDB: before the replicas load from it.
	 */
	async #recover() {
		const entries = journal.unfinished();
		if (!entries.length) return;
		await local.transaction('rw', local.rows, local.outbox, async () => {
			for (const e of entries) {
				for (const row of e.rows) {
					const stored = await local.rows.get([e.ws, row.entity, row.id]);
					await local.rows.put({ ...journal.merge(stored, row), ws: e.ws });
				}
				await local.outbox.bulkAdd(e.ops.map((op) => ({ ws: e.ws, op, schema: e.schema })));
			}
		});
		journal.clear(entries.map((e) => e.key));
	}

	#scheduleFlush() {
		clearTimeout(this.#flushTimer);
		this.#flushTimer = setTimeout(() => void this.flush(), 50);
	}

	/** The edits not yet sent, once those on their way into IndexedDB are in. */
	async unsent(): Promise<Pending[]> {
		await this.#edits;
		return local.outbox.orderBy('n').toArray();
	}

	/** Sends unsent edits, oldest first. One tab at a time does this. */
	async flush() {
		if (!this.#running || this.updateRequired) return;
		// Edits on their way into IndexedDB go too.
		await this.#edits;
		const run = async () => {
			for (;;) {
				const batch = await local.outbox.orderBy('n').limit(PUSH_BATCH).toArray();
				if (batch.length === 0) return;
				const ws = batch[0].ws;
				const same = batch.filter((p) => p.ws === ws);
				await this.#push(ws, same);
			}
		};
		this.#busy++;
		try {
			if (navigator.locks) await navigator.locks.request('mila-flush', run);
			else await run();
			this.#online = true;
			this.#held = false;
		} catch (err) {
			this.#failed(err);
		} finally {
			this.#busy--;
			this.pending = await local.outbox.count();
		}
	}

	async #push(ws: string, batch: Pending[]) {
		// Edits made by an older app are sent in this one's shape; one made
		// by a newer app (in another tab) means this one is out of date.
		const ops = batch.map((p) => upgrade(p));
		if (ops.includes(null)) {
			throw new ApiError('update_required', 'A newer version of Mila made these edits.', 426);
		}
		const sent = ops as Op[];
		const { results } = await call(
			api.POST('/workspaces/{workspaceId}/sync/push', {
				params: { path: { workspaceId: ws } },
				body: { schema: SYNC_SCHEMA, ...(this.user ? { user: this.user } : {}), ops: sent }
			})
		);
		const replica = this.replicas.get(ws);
		const refused: string[] = [];
		await local.transaction('rw', local.rows, local.outbox, async () => {
			for (let i = 0; i < batch.length; i++) {
				const result = results[i] as OpResult;
				if (result.status === 'rejected') refused.push(result.message ?? '');
				if (result.status === 'rejected' && replica) {
					const row = replica.revert(sent[i], result.row);
					if (row === null) await local.rows.delete([ws, sent[i].entity, sent[i].id]);
					else if (row) await local.rows.put({ ws, ...$state.snapshot(row) });
					console.warn('Mila: an edit was refused:', result.message);
				}
			}
			await local.outbox.bulkDelete(batch.map((p) => p.n!));
		});
		// The edit showed at once and has just been taken back: say so.
		if (refused.length) {
			const what = refused.length === 1 ? 'A change' : `${refused.length} changes`;
			toasts.error(`${what} could not be saved. ${refused[0]}`.trim());
		}
	}

	/** Pulls everything after this device's last change number. */
	async pull(ws: string) {
		const replica = this.replicas.get(ws);
		if (!replica || !this.#running || this.updateRequired || this.#samples.has(ws)) return;
		this.#busy++;
		try {
			for (;;) {
				const after = this.#seq.get(ws) ?? 0;
				// The epoch changes when the server is restored from a backup;
				// a copy from before then, or from before epochs, starts again.
				const epoch = this.#epoch.get(ws);
				const page = await call(
					api.GET('/workspaces/{workspaceId}/sync/changes', {
						params: { path: { workspaceId: ws }, query: { after, epoch, schema: SYNC_SCHEMA } }
					})
				);
				const changed: Row[] = [];
				const gone: [string, Entity, string][] = [];
				for (const c of page.changes) {
					const row = replica.remote(c.entity, c.id, c.fields, c.clocks);
					// A document's rows the server merged into one are no use kept.
					const docRows = c.entity === 'doc_update' || c.entity === 'page_update';
					if (row && docRows && row.values.deleted_at != null) {
						replica.forget(c.entity, c.id);
						gone.push([ws, c.entity, c.id]);
					} else if (row) changed.push(row);
				}
				await local.transaction('rw', local.rows, local.meta, async () => {
					await local.rows.bulkPut(changed.map((r) => ({ ws, ...$state.snapshot(r) })));
					if (gone.length) await local.rows.bulkDelete(gone);
					await local.meta.put({ ws, seq: page.seq, epoch });
				});
				this.#seq.set(ws, page.seq);
				if (!page.more) break;
			}
			this.#online = true;
		} catch (err) {
			if (err instanceof ApiError && err.status === 410) {
				await this.#bootstrap(ws).catch((e) => this.#failed(e));
			} else {
				this.#failed(err);
			}
		} finally {
			this.#busy--;
		}
	}

	/**
	 * Replaces the replica with the server's copy, then replays this
	 * device's unsent edits on top so none are lost.
	 */
	async #bootstrap(ws: string) {
		const replica = this.replicas.get(ws);
		if (!replica) return;
		this.#busy++;
		try {
			const snap = await call(
				api.GET('/workspaces/{workspaceId}/sync/bootstrap', {
					params: { path: { workspaceId: ws }, query: { schema: SYNC_SCHEMA } }
				})
			);
			replica.replaceAll(
				snap.rows.map((r) => ({ entity: r.entity, id: r.id, values: r.fields, clocks: r.clocks }))
			);
			const unsent = await local.outbox.where('ws').equals(ws).sortBy('n');
			for (const p of unsent) {
				const op = upgrade(p);
				if (!op) continue;
				// An edit of a pair's row by an ID the copy has not got (one from
				// before pair IDs were hashes, which the server renames) would
				// make a row of it with neither half of its pair: it waits for
				// the server's answer instead.
				const pair = PAIRS[op.entity];
				if (pair && !(pair[0] in op.fields) && !replica.get(op.entity, op.id)) continue;
				replica.remote(op.entity, op.id, op.fields, clocksOf(op));
			}
			await local.transaction('rw', local.rows, local.meta, async () => {
				await local.rows.where('ws').equals(ws).delete();
				await local.rows.bulkPut(
					// Every row here was just made from the server's JSON or the outbox's
					// plain copies, so none holds reactive state: the deep copy a snapshot
					// makes of 20,000 rows was a fifth of a first sync, and IndexedDB copies
					// what it stores anyway.
					[...replica.rows.values()].map((r) => ({ ws, ...r }))
				);
				await local.meta.put({ ws, seq: snap.seq, epoch: snap.epoch });
			});
			this.#seq.set(ws, snap.seq);
			this.#epoch.set(ws, snap.epoch);
			this.#online = true;
			markSynced(ws, snap.rows.length);
		} finally {
			this.#busy--;
		}
	}

	#failed(err: unknown) {
		if (err instanceof ApiError && err.status === 401) {
			this.onSignedOut();
			return;
		}
		if (err instanceof ApiError && err.code === 'other_user') {
			// Refused whole: these edits are not the new person's to send.
			this.stop();
			this.onOtherUser();
			return;
		}
		if (err instanceof ApiError && err.code === 'update_required') {
			// The push was refused whole, so the outbox still holds it.
			if (!this.updateRequired) {
				this.updateRequired = true;
				this.onUpdateRequired();
			}
			return;
		}
		if (err instanceof ApiError && (err.status === 429 || err.status === 503)) {
			// Refused whole, so the outbox keeps it; sent again when the
			// server said, and said once.
			if (!this.#held) {
				toasts.show(
					err.status === 429
						? 'Mila has saved a great deal in the last hour. Your other changes are kept on this device and go in a few minutes.'
						: `${err.message} Your changes are kept on this device until then.`
				);
			}
			this.#held = true;
			this.#reconnect(Math.min(3600, Math.max(5, err.retryAfter ?? 60)) * 1000);
			return;
		}
		if (err instanceof ApiError && err.code !== 'network') {
			console.warn('Mila sync:', err.message);
			return;
		}
		this.#online = false;
		this.#reconnect();
	}

	// The socket only says when to pull; everything else is HTTP.

	#connect() {
		if (!this.#running || this.#socket) return;
		const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/v1/sync/ws`;
		const socket = new WebSocket(url);
		this.#socket = socket;
		let opened = false;
		socket.onopen = () => {
			opened = true;
			this.#retry = 0;
			this.#online = true;
			// Catch up on anything missed while disconnected.
			void this.flush();
			for (const ws of this.replicas.keys()) void this.pull(ws);
			this.onNotify();
			this.onSettings();
			this.onThemes(null);
			this.onPeopleChanged();
			// Open documents say again who is in them.
			for (const p of this.#presence.values()) p.rejoin();
		};
		socket.onmessage = (e) => {
			const msg = JSON.parse(e.data) as {
				type: string;
				workspace?: string;
				seq?: number;
				id?: string;
				doc?: string;
				update?: string;
			};
			if (
				msg.type === 'poke' &&
				msg.workspace &&
				(msg.seq ?? 0) > (this.#seq.get(msg.workspace) ?? 0)
			) {
				void this.pull(msg.workspace);
			} else if (msg.type === 'reset' && msg.workspace && this.replicas.has(msg.workspace)) {
				// What this user may see there changed: the pull answers 410
				// and the workspace loads afresh, rows given and taken alike.
				void this.pull(msg.workspace);
			} else if (msg.type === 'workspaces') {
				this.onWorkspacesChanged();
			} else if (msg.type === 'notify') {
				this.onNotify();
			} else if (msg.type === 'settings') {
				this.onSettings();
			} else if (msg.type === 'themes' && typeof msg.id === 'string') {
				this.onThemes(msg.id);
			} else if (msg.type === 'people') {
				this.onPeopleChanged();
			} else if (msg.type === 'presence' && typeof msg.update === 'string') {
				this.#presence.get(`${msg.workspace}/${msg.doc}`)?.heard(msg.update);
			}
		};
		socket.onclose = () => {
			if (this.#socket === socket) this.#socket = null;
			if (!this.#running) return;
			this.#reconnect();
			// A socket refused says nothing of why, and nothing else may be
			// asking: only an answer can tell that the session has ended.
			if (!opened)
				void call(api.GET('/me')).catch((err) => {
					if (err instanceof ApiError && err.status === 401) this.#failed(err);
				});
		};
	}

	// Presence (docs/PLAN.md section 7): devices say over the socket who
	// has a live document open and where their cursor is. One listener per
	// open document, by workspace/doc.
	#presence = new Map<string, { heard: (update: string) => void; rejoin: () => void }>();

	/**
	 * Opens a document's presence: `heard` gets what other devices say (a
	 * Yjs awareness update, as base64), `rejoin` is asked to say this
	 * device's state again after a reconnect. Returns how to say something
	 * and how to leave.
	 */
	presence(
		ws: string,
		doc: string,
		heard: (update: string) => void,
		rejoin: () => void
	): { say: (update: string) => void; leave: () => void } {
		const key = `${ws}/${doc}`;
		const entry = { heard, rejoin };
		this.#presence.set(key, entry);
		const send = (m: object) => {
			if (this.#socket?.readyState === WebSocket.OPEN) this.#socket.send(JSON.stringify(m));
		};
		return {
			say: (update) => send({ type: 'presence', workspace: ws, doc, update }),
			leave: () => {
				if (this.#presence.get(key) !== entry) return;
				this.#presence.delete(key);
				send({ type: 'leave', workspace: ws, doc });
			}
		};
	}

	/** Called when the user's memberships changed on the server. */
	onWorkspacesChanged: () => void = () => {};

	/**
	 * Called when the user's notifications changed (a new one, or one read
	 * elsewhere), and on every reconnect in case one was missed.
	 */
	onNotify: () => void = () => {};

	/**
	 * Called when the user's preferences changed on another device, and on
	 * every reconnect, which is also when ones changed offline are sent.
	 */
	onSettings: () => void = () => {};

	/**
	 * Called with a theme's id when the user saved or deleted one of their
	 * custom themes on another device, and with null on every reconnect.
	 */
	onThemes: (id: string | null) => void = () => {};

	/**
	 * Called when someone the user shares a workspace with changed their
	 * name or picture, and on every reconnect in case that was missed.
	 */
	onPeopleChanged: () => void = () => {};

	#reconnect(delay?: number) {
		clearTimeout(this.#retryTimer);
		if (!this.#running) return;
		const wait = delay ?? Math.min(30_000, 1000 * 2 ** this.#retry++);
		this.#retryTimer = setTimeout(() => {
			if (!this.#socket) this.#connect();
			else void this.flush();
		}, wait);
	}
}

function clocksOf(op: Op): Record<string, string> {
	return Object.fromEntries(Object.keys(op.fields).map((k) => [k, op.hlc]));
}

export const engine = new SyncEngine();
