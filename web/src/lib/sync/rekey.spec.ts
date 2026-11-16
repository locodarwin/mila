import { describe, expect, it } from 'vitest';
import type { Pending } from './db';
import { pairId, xorId, type Values } from './entities';
import { upgrade } from './outbox';
import { rekeyPairs, type KeptEdits } from './rekey';
import type { Op } from './replica.svelte';

const task = '0190a4c2-6f1e-7b3a-9c4d-2e5f6a7b8c9d';
const label = '4f8e2a1c-3b5d-4e6f-8a9b-0c1d2e3f4a5b';
const other = '7c2d9e4f-1a3b-4c5d-8e6f-9a0b1c2d3e4f';
const old = xorId(task, label);

const edit = (n: number, id: string, fields: Values, schema = 4): Pending => ({
	n,
	ws: 'w',
	schema,
	op: { opId: `op${n}`, entity: 'task_label', id, hlc: '0000000001000-0000-me', fields }
});

function kept(edits: Pending[], rows: Record<string, Values>): KeptEdits & { edits: Pending[] } {
	return {
		edits,
		pending: async () => edits,
		values: async (_ws, entity, id) => rows[`${entity}/${id}`],
		rename: async (n: number, op: Op) => {
			const p = edits.find((e) => e.n === n)!;
			p.op = op;
		}
	};
}

describe('pair rows of sync schema 5', () => {
	it('renames an edit that names its pair when it is sent', () => {
		const sent = upgrade(edit(1, old, { task_id: task, label_id: label, deleted_at: null }));
		expect(sent?.id).toBe(pairId(task, label));
		// A pair's row that was never its XOR, and other entities, stay.
		expect(upgrade(edit(2, other, { task_id: task, label_id: label }))?.id).toBe(other);
	});

	it('renames, at start, edits that name only the row, from the row on the device', async () => {
		const store = kept(
			[
				edit(1, old, { deleted_at: '2026-11-05T10:00:00Z' }),
				edit(2, xorId(task, other), { task_id: task, label_id: other, deleted_at: null }),
				edit(3, xorId(task, other), { deleted_at: '2026-11-05T10:01:00Z' }),
				edit(4, pairId(task, label), { deleted_at: null }, 5),
				edit(5, '0aaaaaaa-0000-0000-0000-000000000000', { deleted_at: null })
			],
			{ [`task_label/${old}`]: { task_id: task, label_id: label } }
		);
		await rekeyPairs(store);
		expect(store.edits.map((e) => e.op.id)).toEqual([
			pairId(task, label), // from its row
			pairId(task, other), // from its own fields
			pairId(task, other), // from the edit before it, the row being new
			pairId(task, label), // made by this version already
			'0aaaaaaa-0000-0000-0000-000000000000' // nothing to go by: the server's to rename
		]);
	});
});
