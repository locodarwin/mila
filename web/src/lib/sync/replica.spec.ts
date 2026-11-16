import { describe, expect, it } from 'vitest';
import { Clock } from './hlc';
import { Replica } from './replica.svelte';

function replica() {
	return new Replica('ws', new Clock('me'));
}

describe('Replica', () => {
	it('creates rows with every default field', () => {
		const r = replica();
		const op = r.local('task', 't1', { title: 'Hello', sort_key: 'a0' });
		const row = r.get('task', 't1')!;
		expect(row.values).toMatchObject({ title: 'Hello', notes: '', completed_at: null });
		expect(row.clocks.title).toBe(op.hlc);
		expect(row.clocks.notes).toBeUndefined();
	});

	it('takes NULs out of text before it is kept or sent', () => {
		const r = replica();
		const op = r.local('task', 't1', { title: 'Hel\0lo', notes: '\0a\0b', sort_key: 'a0' });
		expect(r.get('task', 't1')!.values).toMatchObject({ title: 'Hello', notes: 'ab' });
		expect(op.fields).toMatchObject({ title: 'Hello', notes: 'ab' });
	});

	it('keeps a newer local field when an older one arrives', () => {
		const r = replica();
		r.local('task', 't1', { title: 'Mine', sort_key: 'a0' });
		const changed = r.remote('task', 't1', { title: 'Theirs' }, { title: '0000000000001-0000-x' });
		expect(changed).toBeUndefined();
		expect(r.get('task', 't1')!.values.title).toBe('Mine');
	});

	it('takes a newer remote field and moves its clock past it', () => {
		const r = replica();
		r.local('task', 't1', { title: 'Mine', sort_key: 'a0' });
		const future = '9999999999999-0000-x';
		r.remote('task', 't1', { title: 'Theirs' }, { title: future });
		expect(r.get('task', 't1')!.values.title).toBe('Theirs');
		const next = r.local('task', 't1', { title: 'Again' });
		expect(next.hlc > future).toBe(true);
	});

	it("keeps a task's earliest creator, whichever arrives first", () => {
		// Two devices made the same next task; this one by the later clock.
		const r = replica();
		const mine = r.local('task', 't1', { title: 'Water', created_by: 'me', sort_key: 'a0' });
		const earlier = '0000000000001-0000-x';
		r.remote('task', 't1', { created_by: 'them' }, { created_by: earlier });
		expect(r.get('task', 't1')!.values.created_by).toBe('them');
		// A later creator from elsewhere changes nothing.
		expect(r.remote('task', 't1', { created_by: 'me' }, { created_by: mine.hlc })).toBeUndefined();
		expect(r.get('task', 't1')!.values.created_by).toBe('them');
		// Nor does making it again here, as the server will keep the older.
		r.local('task', 't1', { created_by: 'me', title: 'Water again' });
		expect(r.get('task', 't1')!.values).toMatchObject({ created_by: 'them', title: 'Water again' });
		expect(r.get('task', 't1')!.clocks.created_by).toBe(earlier);
		// A row seen for the first time takes it as it comes.
		r.remote('task', 't2', { created_by: 'me' }, { created_by: mine.hlc });
		expect(r.get('task', 't2')!.values.created_by).toBe('me');
	});

	it('reverts a rejected edit to the server copy', () => {
		const r = replica();
		r.remote(
			'task',
			't1',
			{ title: 'Ok', sort_key: 'a0' },
			{ title: '0000000000001-0000-s', sort_key: '0000000000001-0000-s' }
		);
		const op = r.local('task', 't1', { title: '' });
		r.revert(op, {
			entity: 'task',
			id: 't1',
			fields: { title: 'Ok' },
			clocks: { title: '0000000000001-0000-s' }
		});
		expect(r.get('task', 't1')!.values.title).toBe('Ok');
	});

	it('drops a rejected new row the server never had', () => {
		const r = replica();
		const op = r.local('task', 't1', { title: 'Nope', sort_key: 'a0' });
		expect(r.revert(op, undefined)).toBeNull();
		expect(r.get('task', 't1')).toBeUndefined();
	});

	it('hides deleted and archived rows from its views', () => {
		const r = replica();
		r.local('project', 'p1', { name: 'One', sort_key: 'a0' });
		r.local('project', 'p2', { name: 'Two', sort_key: 'a1', archived_at: '2026-10-28T00:00:00Z' });
		r.local('task', 't1', { title: 'Inbox', sort_key: 'a0' });
		r.local('task', 't2', { title: 'Gone', sort_key: 'a1', deleted_at: '2026-10-28T00:00:00Z' });
		expect(r.projects().map((p) => p.name)).toEqual(['One']);
		expect(r.tasks(null).map((t) => t.title)).toEqual(['Inbox']);
	});

	it('calls a project finished only when every task in it, subtasks too, is done', () => {
		const r = replica();
		const done = '2026-10-29T00:00:00Z';
		expect(r.projectFinished('p1')).toBe(false);
		r.local('task', 't1', { title: 'Top', project_id: 'p1', sort_key: 'a0', completed_at: done });
		r.local('task', 's1', { title: 'Sub', project_id: 'p1', parent_id: 't1', sort_key: 'a0' });
		r.local('task', 'x1', { title: 'Elsewhere', project_id: 'p2', sort_key: 'a0' });
		// Its top-level task is done, but a subtask is still open.
		expect(r.projectFinished('p1')).toBe(false);
		r.local('task', 's1', { completed_at: done });
		expect(r.projectFinished('p1')).toBe(true);
		// An open subtask of a deleted task does not hold it back.
		r.local('task', 't2', { title: 'Gone', project_id: 'p1', sort_key: 'a1', deleted_at: done });
		r.local('task', 's2', { title: 'Orphan', project_id: 'p1', parent_id: 't2', sort_key: 'a0' });
		expect(r.projectFinished('p1')).toBe(true);
		r.local('task', 't3', { title: 'New', project_id: 'p1', sort_key: 'a2' });
		expect(r.projectFinished('p1')).toBe(false);
	});

	it('keeps its indexes in step as rows move, go and come back', () => {
		const r = replica();
		r.local('task', 'a', { title: 'A', project_id: 'p1', sort_key: 'a0' });
		r.local('task', 'b', { title: 'B', project_id: 'p1', parent_id: 'a', sort_key: 'a1' });
		r.local('checklist_item', 'c', { task_id: 'a', text: 'Step', sort_key: 'a0' });
		r.local('task_label', 'l', { task_id: 'a', label_id: 'x' });
		r.local('label', 'x', { name: 'red', sort_key: 'a0' });
		const ids = (ts: { id: string }[]) => ts.map((t) => t.id);
		expect(ids(r.tasks('p1'))).toEqual(['a']);
		expect(ids(r.subtasks('a'))).toEqual(['b']);
		expect(ids(r.checklist('a'))).toEqual(['c']);
		expect(ids(r.taskLabels('a'))).toEqual(['x']);

		// Moved to another project and out from under its parent.
		r.remote(
			'task',
			'b',
			{ project_id: 'p2', parent_id: null },
			{ project_id: '9999999999999-0000-x', parent_id: '9999999999999-0000-x' }
		);
		expect(ids(r.tasks('p1'))).toEqual(['a']);
		expect(ids(r.tasks('p2'))).toEqual(['b']);
		expect(r.subtasks('a')).toEqual([]);

		// Deleted, then restored; the inbox is the null project.
		r.local('checklist_item', 'c', { deleted_at: '2026-11-03T00:00:00Z' });
		expect(r.checklist('a')).toEqual([]);
		r.local('checklist_item', 'c', { deleted_at: null });
		expect(ids(r.checklist('a'))).toEqual(['c']);
		r.local('task', 'a', { project_id: null });
		expect(ids(r.tasks(null))).toEqual(['a']);
		expect(r.tasks('p1')).toEqual([]);

		// A bootstrap replaces everything, indexes too.
		r.replaceAll([
			{
				entity: 'task',
				id: 'z',
				values: { title: 'Z', project_id: 'p1', sort_key: 'a0' },
				clocks: {}
			}
		]);
		expect(ids(r.tasks('p1'))).toEqual(['z']);
		expect(r.tasks(null)).toEqual([]);
		expect(r.checklist('a')).toEqual([]);
	});
});
