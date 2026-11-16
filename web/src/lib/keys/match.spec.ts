import { beforeEach, describe, expect, it } from 'vitest';
import { effective } from './bindings';
import type { KeyLike } from './chord';
import type { Scope } from './commands';
import { buildIndex, Matcher, SEQUENCE_MS, type Index } from './match';

function press(key: string, rest: Partial<KeyLike> = {}): KeyLike {
	const code = /^[a-z]$/i.test(key)
		? `Key${key.toUpperCase()}`
		: /^[0-9]$/.test(key)
			? `Digit${key}`
			: key;
	return { key, code, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...rest };
}

let now = 0;
let matcher: Matcher;
let index: Index;
const defaults = (singleKeys = true) => buildIndex(effective({}), singleKeys);
const match = (scope: Scope, e: KeyLike, ctx?: { field?: boolean; handled?: boolean }) =>
	matcher.match(index, scope, e, ctx);

beforeEach(() => {
	now = 1000;
	matcher = new Matcher(() => now);
	index = defaults();
});

describe('parity with the keys before they could be changed', () => {
	// One row per row of the old shortcut sheet and per branch of the old
	// handlers in the app layout, taskKeys, the board and the calendar.
	it.each([
		['anywhere', press('q'), 'quick-add'],
		['anywhere', press('Q', { shiftKey: true }), 'quick-add'],
		['anywhere', press('Q'), 'quick-add'],
		['anywhere', press('k', { ctrlKey: true }), 'palette'],
		['anywhere', press('k', { metaKey: true }), 'palette'],
		['anywhere', press('/'), 'search'],
		['anywhere', press('?', { code: 'Slash', shiftKey: true }), 'help'],
		['anywhere', press('/', { code: 'Slash', ctrlKey: true }), 'help'],
		['anywhere', press('z', { ctrlKey: true }), 'undo'],
		['anywhere', press('Z', { ctrlKey: true, shiftKey: true }), null],
		['anywhere', press('k', { ctrlKey: true, shiftKey: true }), null],
		['anywhere', press('x'), null],
		['list', press('j'), 'task.next'],
		['list', press('ArrowDown'), 'task.next'],
		['list', press('ArrowDown', { shiftKey: true }), 'select.down'],
		['list', press('k'), 'task.prev'],
		['list', press('ArrowUp'), 'task.prev'],
		['list', press('Enter'), 'task.open'],
		['list', press('x'), 'task.complete'],
		['list', press('X', { shiftKey: true }), 'task.complete'],
		['list', press('t'), 'task.my-day'],
		['list', press('1'), 'task.priority.urgent'],
		['list', press('2'), 'task.priority.high'],
		['list', press('3'), 'task.priority.medium'],
		['list', press('4'), 'task.priority.low'],
		['list', press('0'), 'task.priority.none'],
		['list', press('!', { code: 'Digit1', shiftKey: true }), null],
		['list', press('ArrowUp', { altKey: true }), 'task.move-up'],
		['list', press('ArrowDown', { altKey: true }), 'task.move-down'],
		['list', press('ArrowUp', { altKey: true, shiftKey: true }), 'task.move-up'],
		['list', press('j', { altKey: true }), null],
		['list', press('j', { ctrlKey: true }), null],
		['list', press('Delete'), 'task.delete'],
		['list', press('Backspace'), 'task.delete'],
		['list', press('Delete', { shiftKey: true }), 'task.delete'],
		['list', press('q'), null],
		['board', press('ArrowLeft', { altKey: true }), 'board.move-left'],
		['board', press('ArrowRight', { altKey: true }), 'board.move-right'],
		['board', press('ArrowUp', { altKey: true }), 'board.move-up'],
		['board', press('ArrowDown', { altKey: true }), 'board.move-down'],
		['board', press('ArrowDown'), 'board.walk'],
		['board', press('Enter'), 'board.open'],
		['calendar', press('ArrowLeft', { altKey: true }), 'calendar.earlier-day'],
		['calendar', press('ArrowRight', { altKey: true }), 'calendar.later-day'],
		['calendar', press('ArrowUp', { altKey: true }), 'calendar.earlier-week'],
		['calendar', press('ArrowDown', { altKey: true }), 'calendar.later-week'],
		['calendar', press('ArrowDown'), null]
	] as const)('%s: %o gives %s', (scope, e, want) => {
		expect(match(scope, e)).toBe(want);
	});

	it('goes with G then a letter within the window, and not after it', () => {
		expect(match('anywhere', press('g'))).toBeNull();
		now += 500;
		expect(match('anywhere', press('i'))).toBe('go.inbox');
		expect(match('anywhere', press('g'))).toBeNull();
		now += SEQUENCE_MS + 100;
		expect(match('anywhere', press('i'))).toBeNull();
		for (const [k, id] of [
			['d', 'go.today'],
			['u', 'go.upcoming'],
			['p', 'go.projects'],
			['h', 'go.help'],
			['I', 'go.inbox']
		]) {
			match('anywhere', press('g'));
			expect(match('anywhere', press(k))).toBe(id);
		}
	});

	it('takes G then Q as quick add', () => {
		match('anywhere', press('g'));
		expect(match('anywhere', press('q'))).toBe('quick-add');
	});

	it('lets only the palette through in a field, and nothing a row took', () => {
		expect(match('anywhere', press('q'), { field: true })).toBeNull();
		expect(match('anywhere', press('k', { ctrlKey: true }), { field: true })).toBe('palette');
		expect(match('anywhere', press('z', { ctrlKey: true }), { field: true })).toBeNull();
		expect(match('anywhere', press('q'), { handled: true })).toBeNull();
		expect(match('anywhere', press('k', { ctrlKey: true }), { handled: true })).toBe('palette');
	});

	it('keeps the first key of a sequence while typing, and drops it once a row takes a key', () => {
		match('anywhere', press('g'));
		match('anywhere', press('a'), { field: true });
		expect(match('anywhere', press('i'))).toBe('go.inbox');
		match('anywhere', press('g'));
		match('anywhere', press('x'), { handled: true });
		expect(match('anywhere', press('i'))).toBeNull();
	});

	it('leaves the second key of a sequence to the window on a row', () => {
		match('anywhere', press('g'));
		expect(match('list', press('i'))).toBeNull();
		expect(match('anywhere', press('i'))).toBe('go.inbox');
		// Not part of a sequence: the row's.
		match('anywhere', press('g'));
		expect(match('list', press('x'))).toBe('task.complete');
	});
});

describe('single-key shortcuts off', () => {
	beforeEach(() => {
		index = defaults(false);
	});

	it('turns off character keys anywhere', () => {
		expect(match('anywhere', press('q'))).toBeNull();
		expect(match('anywhere', press('/'))).toBeNull();
		match('anywhere', press('g'));
		expect(match('anywhere', press('i'))).toBeNull();
	});

	it('leaves the shortcut list on Ctrl or Cmd and /, outside fields', () => {
		index = defaults(false);
		expect(match('anywhere', press('?', { code: 'Slash', shiftKey: true }))).toBeNull();
		expect(match('anywhere', press('/', { code: 'Slash', ctrlKey: true }))).toBe('help');
		expect(match('anywhere', press('/', { code: 'Slash', metaKey: true }))).toBe('help');
		expect(match('anywhere', press('/', { code: 'Slash', ctrlKey: true }), { field: true })).toBe(
			null
		);
	});

	it('leaves keys with a modifier, named keys and focused keys working', () => {
		expect(match('list', press('x'))).toBe('task.complete');
		expect(match('anywhere', press('k', { ctrlKey: true }))).toBe('palette');
		index = buildIndex(effective({ 'quick-add': ['F2'], 'go.inbox': ['Alt+i'] }), false);
		expect(match('anywhere', press('F2'))).toBe('quick-add');
		expect(match('anywhere', press('i', { altKey: true }))).toBe('go.inbox');
	});
});

describe('held keys', () => {
	it('keep a sequence waiting while its first key is held', () => {
		match('anywhere', press('g'));
		now += 900;
		expect(match('anywhere', press('g', { repeat: true }))).toBeNull();
		now += 900;
		expect(match('anywhere', press('g', { repeat: true }))).toBeNull();
		now += 300;
		expect(match('anywhere', press('i'))).toBe('go.inbox');
	});

	it('repeat moves only', () => {
		expect(match('list', press('x', { repeat: true }))).toBeNull();
		expect(match('list', press('Delete', { repeat: true }))).toBeNull();
		expect(match('list', press('t', { repeat: true }))).toBeNull();
		expect(match('list', press('j', { repeat: true }))).toBe('task.next');
		expect(match('list', press('ArrowUp', { altKey: true, repeat: true }))).toBe('task.move-up');
		expect(match('anywhere', press('q', { repeat: true }))).toBeNull();
	});
});

describe('changed keys', () => {
	it('match what the person chose', () => {
		index = buildIndex(effective({ 'quick-add': ['n'], 'go.inbox': ['g n'] }), true);
		expect(match('anywhere', press('q'))).toBeNull();
		expect(match('anywhere', press('n'))).toBe('quick-add');
		match('anywhere', press('g'));
		expect(match('anywhere', press('n'))).toBe('go.inbox');
	});

	it('let Shift and a sequence start be a key of its own', () => {
		index = buildIndex(effective({ 'go.trash': ['Shift+g'] }), true);
		expect(match('anywhere', press('G', { shiftKey: true }))).toBe('go.trash');
		// Plain G still starts the sequences.
		expect(match('anywhere', press('g'))).toBeNull();
		expect(match('anywhere', press('i'))).toBe('go.inbox');
	});

	it('never match a plain key for the palette, whatever the map holds', () => {
		const map = effective({});
		map.set('palette', ['q']);
		index = buildIndex(map, true);
		expect(match('anywhere', press('q'), { field: true })).toBeNull();
	});

	it('let the narrower scope win on a row', () => {
		index = buildIndex(effective({ 'task.complete': ['q'] }), true);
		expect(match('list', press('q'))).toBe('task.complete');
		expect(match('anywhere', press('q'))).toBe('quick-add');
	});
});
