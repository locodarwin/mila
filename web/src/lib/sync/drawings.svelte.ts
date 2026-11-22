// Drawings in wiki pages: making one, and changing its shapes. Loaded only
// with a drawing. Each change in the editor is a step of its own history
// (undo and redo there), written through engine.mutate a row at a time, so
// two people editing different shapes keep both.
import { readBox, shapeOf, inOrder, type DrawingStarter, type Shape } from '../drawing';
import { uuid } from '../uuid';
import { engine } from './engine.svelte';
import type { Values } from './entities';
import { keyBetween } from './sortkey';

const now = () => new Date().toISOString();

/** A drawing's shapes as they are on this device. */
export function shapesOf(ws: string, pageId: string, drawingId: string): Shape[] {
	const r = engine.replica(ws);
	if (!r) return [];
	return inOrder(r.drawingRows(pageId, drawingId).map((row) => shapeOf(row.id, row.values)));
}

/** A new drawing on a page, with a starter's shapes; its ID. */
export async function makeDrawing(
	ws: string,
	pageId: string,
	me: string,
	starter: DrawingStarter
): Promise<string> {
	const id = uuid();
	// eslint-disable-next-line svelte/prefer-svelte-reactivity -- plain: local to making the rows
	const ids = new Map(starter.shapes.map((s) => [s.key, uuid()]));
	let key: string | null = null;
	const rows: { entity: 'page_drawing' | 'drawing_shape'; id: string; values: Values }[] = [
		{
			entity: 'page_drawing',
			id,
			values: { page_id: pageId, name: starter.name, grid: true, snap: true, made_by: me }
		}
	];
	// Lines go under the shapes they join, but are written after them, so
	// the shapes they name are there first.
	const lines = (s: DrawingStarter['shapes'][number]) => !!s.from || !!s.to;
	// eslint-disable-next-line svelte/prefer-svelte-reactivity -- plain: local to making the rows
	const keys = new Map<string, string>();
	for (const s of [...starter.shapes.filter(lines), ...starter.shapes.filter((x) => !lines(x))]) {
		key = keyBetween(key, null);
		keys.set(s.key, key);
	}
	for (const s of [...starter.shapes.filter((x) => !lines(x)), ...starter.shapes.filter(lines)]) {
		rows.push({
			entity: 'drawing_shape',
			id: ids.get(s.key)!,
			values: {
				page_id: pageId,
				drawing_id: id,
				kind: s.kind,
				box: readBox(s.box),
				text: s.text ?? '',
				style: s.style ?? {},
				from_id: s.from ? ids.get(s.from) : null,
				to_id: s.to ? ids.get(s.to) : null,
				sort_key: keys.get(s.key)!
			}
		});
	}
	await engine.insert(ws, rows);
	return id;
}

export function renameDrawing(ws: string, id: string, name: string) {
	return engine.mutate(ws, 'page_drawing', id, { name: name.trim().slice(0, 200) });
}

export function setDrawingOption(ws: string, id: string, option: 'grid' | 'snap', on: boolean) {
	return engine.mutate(ws, 'page_drawing', id, { [option]: on });
}

/** One change to one shape: its fields before (null when it is new) and after. */
export interface Change {
	id: string;
	before: Values | null;
	after: Values;
}

/** A shape's fields as a row's, for what a change writes. */
export function rowOf(s: Shape, pageId: string): Values {
	return {
		page_id: pageId,
		drawing_id: s.drawingId,
		kind: s.kind,
		box: { x: round(s.box.x), y: round(s.box.y), w: round(s.box.w), h: round(s.box.h) },
		points: s.points,
		text: s.text,
		style: s.style,
		from_id: s.fromId,
		to_id: s.toId,
		image_id: s.imageId,
		sort_key: s.sortKey
	};
}

const round = (n: number) => Math.round(n * 10) / 10;

/** The fields that differ between two versions of a shape, as a row's. */
export function diff(before: Shape, after: Shape, pageId: string): Change | null {
	const a = rowOf(before, pageId);
	const b = rowOf(after, pageId);
	const was: Values = {};
	const now: Values = {};
	for (const k of Object.keys(b)) {
		if (k === 'page_id' || k === 'drawing_id' || k === 'kind') continue;
		if (JSON.stringify(a[k]) !== JSON.stringify(b[k])) {
			was[k] = a[k];
			now[k] = b[k];
		}
	}
	return Object.keys(now).length ? { id: after.id, before: was, after: now } : null;
}

async function write(ws: string, changes: Change[], which: 'before' | 'after') {
	for (const c of changes) {
		const values = which === 'after' ? c.after : (c.before ?? { deleted_at: now() });
		// A new shape undone is deleted; done again, it comes back.
		const fields =
			which === 'after' && c.before === null ? { ...values, deleted_at: null } : values;
		await engine.mutate(ws, 'drawing_shape', c.id, fields, { undoable: false });
	}
}

/**
 * A drawing's history while it is edited: each step one action (a move of
 * several shapes, a new arrow), undone and done again as a whole.
 */
export class DrawingHistory {
	#done: Change[][] = [];
	#undone: Change[][] = [];
	canUndo = $state(false);
	canRedo = $state(false);
	/** Whether anything was changed since the editor opened. */
	changed = $state(false);

	constructor(private ws: string) {}

	async apply(changes: Change[]) {
		if (!changes.length) return;
		this.#done.push(changes);
		if (this.#done.length > 100) this.#done.shift();
		this.#undone = [];
		this.#update();
		this.changed = true;
		await write(this.ws, changes, 'after');
	}

	async undo(): Promise<boolean> {
		const step = this.#done.pop();
		if (!step) return false;
		this.#undone.push(step);
		this.#update();
		await write(this.ws, [...step].reverse(), 'before');
		this.changed = true;
		return true;
	}

	async redo(): Promise<boolean> {
		const step = this.#undone.pop();
		if (!step) return false;
		this.#done.push(step);
		this.#update();
		await write(this.ws, step, 'after');
		this.changed = true;
		return true;
	}

	#update() {
		this.canUndo = this.#done.length > 0;
		this.canRedo = this.#undone.length > 0;
	}
}
