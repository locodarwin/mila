import { describe, expect, it } from 'vitest';
import { effective } from './bindings';
import { conflicts, swap, swapConflicts } from './conflicts';

const map = effective({});
const kinds = (id: Parameters<typeof conflicts>[1], b: string) =>
	conflicts(map, id, b).map((c) => c.kind);

describe('conflicts', () => {
	it('keeps reserved keys for the browser and the system', () => {
		const [c] = conflicts(map, 'quick-add', 'Mod+w');
		expect(c.kind).toBe('reserved');
		expect(c.message).toBe("Ctrl+W closes the browser tab; Mila can't use it.");
		expect(kinds('quick-add', 'Mod+l')).toEqual(['reserved']);
		expect(kinds('quick-add', 'Tab')).toEqual(['reserved']);
		expect(kinds('go.trash', 'g Escape')).toContain('reserved');
	});

	it('reserves Alt and the side arrows anywhere, but not on the board', () => {
		expect(kinds('go.trash', 'Alt+ArrowLeft')[0]).toBe('reserved');
		expect(kinds('board.move-left', 'Alt+ArrowLeft')).toEqual([]);
		expect(kinds('board.move-left', 'Space')).toEqual([]);
		expect(kinds('task.complete', 'Space')).toEqual(['same-scope']);
		expect(kinds('go.trash', 'Space')).toEqual(['reserved', 'overlap']);
	});

	it("finds a command's own fixed key, which it always has", () => {
		const [c] = conflicts(map, 'task.next', 'ArrowDown');
		expect(c).toMatchObject({ kind: 'fixed', other: 'task.next' });
		expect(c.message).toBe("Down arrow is always one of “Next task”'s keys already.");
	});

	it('refuses a sequence on a focused task', () => {
		expect(kinds('task.complete', 'g x')).toEqual(['sequence']);
	});

	it("keeps another command's fixed keys", () => {
		const [c] = conflicts(map, 'task.complete', 'ArrowDown');
		expect(c.kind).toBe('fixed');
		expect(c.other).toBe('task.next');
	});

	it("keeps the theme editor's undo keys from the shell's commands", () => {
		const [c] = conflicts(map, 'go.trash', 'Mod+y');
		expect(c).toMatchObject({ kind: 'fixed', other: 'theme.redo' });
		expect(c.message).toBe(
			"Ctrl+Y is always Redo in the theme editor, so “Open the trash” would not work there; that can't change."
		);
		expect(conflicts(map, 'palette', 'Mod+Shift+z')[0]).toMatchObject({ other: 'theme.redo' });
		expect(conflicts(map, 'search', 'Mod+z')[0]).toMatchObject({ other: 'theme.undo' });
		// The app's own undo is what the editor's stands in for.
		const undo = new Map(map);
		undo.set('undo', []);
		expect(conflicts(undo, 'undo', 'Mod+z')).toEqual([]);
		// A focused task is not heard while the editor is open.
		expect(kinds('task.complete', 'Mod+y')).toEqual([]);
	});

	it('asks for a modifier where the command works while typing', () => {
		expect(kinds('palette', 'p')).toEqual(['field']);
		expect(kinds('palette', 'Mod+Alt+p')).toEqual([]);
		expect(kinds('palette', 'F8')).toEqual([]);
	});

	it('finds the same key in the same scope', () => {
		const [c] = conflicts(map, 'task.my-day', 'x');
		expect(c).toMatchObject({ kind: 'same-scope', other: 'task.complete' });
		expect(c.message).toBe('X already does “Complete or reopen” on a task in a list.');
	});

	it('finds the same key in an overlapping scope', () => {
		const [c] = conflicts(map, 'task.complete', 'q');
		expect(c).toMatchObject({ kind: 'overlap', other: 'quick-add' });
		expect(c.message).toBe(
			'On a task in a list, Q would do “Complete or reopen” instead of “Add a task”.'
		);
	});

	it('leaves the list and the board to themselves', () => {
		expect(kinds('board.move-left', 'Alt+ArrowUp')).toEqual(['same-scope']);
		expect(kinds('task.complete', 'Alt+ArrowLeft')).toEqual([]);
	});

	it('finds a key that starts a sequence, and a sequence a key starts', () => {
		const [c] = conflicts(map, 'go.trash', 'g');
		expect(c.kind).toBe('prefix');
		expect(c.message).toBe('G starts “Go to Inbox” (G then I) and 7 more.');
		expect(kinds('go.trash', 'q t')).toEqual(['prefix']);
		expect(kinds('go.trash', 'g t')).toEqual([]);
	});
});

describe('swapConflicts', () => {
	const after = (
		id: Parameters<typeof conflicts>[1],
		old: string,
		other: Parameters<typeof conflicts>[1],
		key: string
	) => swapConflicts(map, { id, old }, { id: other, key }).map((c) => c.kind);

	it('allows a swap the other command can take', () => {
		expect(after('task.complete', 'x', 'task.my-day', 't')).toEqual([]);
	});

	it('never hands the palette a plain key', () => {
		// Add a task's Q for the palette's Ctrl+K: every Q typed would open it.
		expect(after('quick-add', 'q', 'palette', 'Mod+k')).toContain('field');
	});

	it('never hands a focused scope a sequence', () => {
		// Go to Inbox's G I for Complete's X: G I never fires on a row.
		expect(after('go.inbox', 'g i', 'task.complete', 'x')).toContain('sequence');
	});

	it('never hands a command a reserved key', () => {
		// The board's Alt+Left for Add a task's Q: the browser goes back.
		expect(after('board.move-left', 'Alt+ArrowLeft', 'quick-add', 'q')).toContain('reserved');
	});
});

describe('swap', () => {
	it('moves rather than swaps when asked to', () => {
		const out = swap({}, { id: 'quick-add', old: 'q' }, { id: 'palette', key: 'Mod+k' }, false);
		expect(out).toEqual({ palette: [], 'quick-add': ['Mod+k'] });
	});

	it('gives the other command the key being replaced', () => {
		const out = swap({}, { id: 'task.complete', old: 'x' }, { id: 'task.my-day', key: 't' });
		expect(out).toEqual({ 'task.complete': ['t'], 'task.my-day': ['x'] });
	});

	it('moves a key that is being added', () => {
		const out = swap({}, { id: 'task.complete' }, { id: 'task.my-day', key: 't' });
		expect(out).toEqual({ 'task.complete': ['x', 't'], 'task.my-day': [] });
	});

	it('drops entries that end up as the defaults', () => {
		const once = swap({}, { id: 'task.complete', old: 'x' }, { id: 'task.my-day', key: 't' });
		expect(swap(once, { id: 'task.complete', old: 't' }, { id: 'task.my-day', key: 'x' })).toEqual(
			{}
		);
	});
});
