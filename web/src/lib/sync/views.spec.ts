import { describe, expect, it } from 'vitest';
import { filterCount, matches, UNREADABLE } from '../filter';
import { createTaskIn, createView, editView, writeColumn } from './data';
import { engine } from './engine.svelte';
import { readOptions } from './replica.svelte';

// A sample workspace keeps edits in memory, so the engine runs without a
// database or a server.
function workspace() {
	const ws = `views-${Math.random()}`;
	const replica = engine.sample(ws, []);
	return { ws, replica };
}

describe('editView', () => {
	it('works a list change out from the row as it stands when it is saved', async () => {
		const { ws, replica } = workspace();
		await engine.mutate(ws, 'saved_view', 'v', {
			name: 'V',
			type: 'list',
			config: { filter: { labels: ['l1'] } },
			display: { later: 1 },
			sort_key: 'a0'
		});
		// Two changes sent together, as two quick clicks are: the second
		// builds on the first, not on what the page showed before it.
		const add = (id: string) => (cs: unknown[] | undefined) => [...(cs ?? []), id];
		await Promise.all([
			editView(ws, 'v', { display: (d) => ({ sort: add('a')(d.sort as unknown[]) }) }),
			editView(ws, 'v', { display: (d) => ({ sort: add('b')(d.sort as unknown[]) }) }),
			editView(ws, 'v', { filter: (f) => ({ labels: [...(f.labels ?? []), 'l2'] }) }),
			editView(ws, 'v', { filter: (f) => ({ labels: [...(f.labels ?? []), 'l3'] }) })
		]);
		const v = replica.view('v')!;
		expect(v.display).toEqual({ later: 1, sort: ['a', 'b'] });
		expect(v.filter.labels).toEqual(['l1', 'l2', 'l3']);
		// A patch value of undefined takes the part away.
		await editView(ws, 'v', { filter: { labels: undefined }, display: { sort: undefined } });
		expect(replica.view('v')!.filter).toEqual({});
		expect(replica.view('v')!.display).toEqual({ later: 1 });
		engine.dropSample(ws);
	});
});

// Rows a broken app or an old REST write left in a shape the app cannot
// read never break the page: the view shows nothing, the panels read the
// parts they can, and the next edit makes the row whole again.
describe('views stored in a broken shape', () => {
	const ctx = {
		me: 'u1',
		today: '2026-10-28',
		category: () => 'todo' as const,
		labels: () => [],
		assignees: () => []
	};

	it('reads a config or display that is not an object, and repairs it on edit', async () => {
		const { ws, replica } = workspace();
		await engine.mutate(ws, 'saved_view', 'v', {
			name: 'V',
			type: 'list',
			config: '{"filter":{}}',
			display: [null],
			sort_key: 'a0'
		});
		const v = replica.view('v')!;
		expect(v.filter).toBe(UNREADABLE);
		expect(v.display).toEqual({});
		expect(filterCount(v.filter)).toBe(1);
		await editView(ws, 'v', { filter: (f) => ({ labels: [...(f.labels ?? []), 'l1'] }) });
		expect(replica.view('v')!.filter).toEqual({ labels: ['l1'] });
		await editView(ws, 'v', { display: { sort: [{ by: 'title', dir: 'asc' }] } });
		expect(replica.view('v')!.display).toEqual({ sort: [{ by: 'title', dir: 'asc' }] });
		engine.dropSample(ws);
	});

	it('shows nothing for a filter part of the wrong type until it is set again', async () => {
		const { ws, replica } = workspace();
		await engine.mutate(ws, 'saved_view', 'v', {
			name: 'V',
			type: 'list',
			config: { filter: { statuses: 'todo', labels: 'l1' } },
			sort_key: 'a0'
		});
		const task = {
			id: 't',
			projectId: null,
			priority: 'none',
			title: 'T',
			notes: '',
			dueDate: null
		};
		expect(matches(task as never, replica.view('v')!.filter, ctx)).toBe(false);
		await editView(ws, 'v', { filter: { statuses: ['todo'], labels: undefined } });
		expect(matches(task as never, replica.view('v')!.filter, ctx)).toBe(true);
		engine.dropSample(ws);
	});

	it('never copies an unreadable filter into a new view', async () => {
		const { ws, replica } = workspace();
		const id = await createView(ws, {
			projectId: null,
			ownerId: null,
			name: 'Copy',
			type: 'list',
			filter: UNREADABLE
		});
		expect(replica.get('saved_view', id)!.values.config).toEqual({ filter: {} });
		engine.dropSample(ws);
	});

	it('reads only the choices it can', () => {
		expect(readOptions({})).toEqual([]);
		expect(readOptions('o1')).toEqual([]);
		expect(
			readOptions([
				null,
				5,
				{ name: 'No id' },
				{ id: 'o1', name: 'One', color: 'red', later: 1 },
				{ id: 'o1', name: 'Again', color: 'blue' },
				{ id: 'o2', name: 7 }
			])
		).toEqual([
			{ id: 'o1', name: 'One', color: 'red', later: 1 },
			{ id: 'o2', name: '', color: 'blue' }
		]);
	});
});

describe('board columns', () => {
	it('gives a task whose status is gone the status of the column it is moved in', async () => {
		const { ws, replica } = workspace();
		await engine.mutate(ws, 'project', 'p', { name: 'P', sort_key: 'a0' });
		await engine.mutate(ws, 'status', 's1', {
			project_id: 'p',
			name: 'To do',
			category: 'todo',
			sort_key: 'a0'
		});
		await engine.mutate(ws, 'task', 't', {
			project_id: 'p',
			status_id: 's-gone',
			title: 'T',
			sort_key: 'a0'
		});
		await writeColumn(ws, replica.task('t')!, { status: 's1' });
		expect(replica.task('t')!.statusId).toBe('s1');
		engine.dropSample(ws);
	});

	it('writes a field column only when the value differs, and makes cards with it', async () => {
		const { ws, replica } = workspace();
		await engine.mutate(ws, 'project', 'p', { name: 'P', sort_key: 'a0' });
		await engine.mutate(ws, 'status', 's1', {
			project_id: 'p',
			name: 'To do',
			category: 'todo',
			sort_key: 'a0'
		});
		const id = await createTaskIn(ws, 'p', 'Card', { field: 'f', value: 'o1' });
		expect(replica.fieldValues(id).f).toBe('o1');
		const clocks = () => JSON.stringify([...replica.rows.values()].map((r) => r.clocks));
		const was = clocks();
		await writeColumn(ws, replica.task(id)!, { field: 'f', value: 'o1' });
		expect(clocks()).toBe(was);
		await writeColumn(ws, replica.task(id)!, { field: 'f', value: null });
		expect(replica.fieldValues(id).f).toBeUndefined();
		const high = await createTaskIn(ws, 'p', 'High', { priority: 'high' });
		expect(replica.task(high)!.priority).toBe('high');
		engine.dropSample(ws);
	});
});
