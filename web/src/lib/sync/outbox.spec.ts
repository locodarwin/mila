import { describe, expect, it } from 'vitest';
import type { Pending } from './db';
import { SYNC_SCHEMA } from './entities';
import { FIRST_SCHEMA, upgrade, UPGRADES } from './outbox';
import type { Op } from './replica.svelte';

const op: Op = {
	opId: 'op1',
	entity: 'task',
	id: 't1',
	hlc: '0000000001000-0000-me',
	fields: { title: 'Old shape' }
};

// A pretend version 2 that renamed a task's title to name.
const steps = {
	1: (o: Op): Op => {
		const { title, ...rest } = o.fields;
		return { ...o, fields: { ...rest, name: title } };
	}
};

describe('outbox upgrades', () => {
	it('has a step from every older version to SYNC_SCHEMA', () => {
		for (let v = FIRST_SCHEMA; v < SYNC_SCHEMA; v++)
			expect(UPGRADES[v], `from ${v}`).toBeTypeOf('function');
		expect(upgrade({ ws: 'w', op, schema: FIRST_SCHEMA })).not.toBeNull();
	});

	it('rewrites an edit made under an older version instead of sending it as it was', () => {
		const sent = upgrade({ ws: 'w', op, schema: 1 }, 2, steps);
		expect(sent).not.toEqual(op);
		expect(sent?.fields).toEqual({ name: 'Old shape' });
		expect(sent?.opId).toBe('op1');
	});

	it('treats an edit saved before versions were kept as the first version', () => {
		const old: Pending = { ws: 'w', op };
		expect(upgrade(old, 2, steps)?.fields).toEqual({ name: 'Old shape' });
	});

	it('sends an edit of the current version unchanged', () => {
		expect(upgrade({ ws: 'w', op, schema: 2 }, 2, steps)).toBe(op);
	});

	it('will not send an edit made by a newer app', () => {
		expect(upgrade({ ws: 'w', op, schema: 3 }, 2, steps)).toBeNull();
	});

	it('refuses an edit it has no step for', () => {
		expect(() => upgrade({ ws: 'w', op, schema: 1 }, 2, {})).toThrow(/schema 1/);
	});
});
