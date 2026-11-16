import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { assign, changed, clash, effective, reset } from './bindings';
import { COMMANDS, isCommand, type CommandId, type Scope } from './commands';

describe('effective', () => {
	it('gives the defaults when nothing changed', () => {
		const map = effective({});
		for (const c of COMMANDS) expect(map.get(c.id), c.id).toEqual('remappable' in c ? [] : c.keys);
		expect([...map.keys()]).toEqual(COMMANDS.map((c) => c.id));
	});

	it('lays the changes over the defaults', () => {
		const map = effective({ 'quick-add': ['n'], 'task.complete': [] });
		expect(map.get('quick-add')).toEqual(['n']);
		expect(map.get('task.complete')).toEqual([]);
		expect(map.get('search')).toEqual(['/']);
	});

	it('passes over unknown ids and bindings that are not bindings', () => {
		const map = effective({ 'from.the-future': ['F9'], 'quick-add': ['Ctrl+n', 'n'] });
		expect(map.get('quick-add')).toEqual(['n']);
		expect(map.has('from.the-future' as CommandId)).toBe(false);
	});

	it('never lets a default take a key the person chose', () => {
		// The same key in the same scope: the default goes.
		expect(effective({ 'quick-add': ['/'] }).get('search')).toEqual([]);
		// A default sequence started by a chosen key goes too.
		expect(effective({ search: ['g'] }).get('go.inbox')).toEqual([]);
		// And a chosen sequence makes a default single key that starts it go.
		expect(effective({ 'go.trash': ['q t'] }).get('quick-add')).toEqual([]);
		// The same key in an overlapping scope stays: the row wins on a row.
		expect(effective({ 'task.complete': ['q'] }).get('quick-add')).toEqual(['q']);
	});

	it('gives a key two commands were given to the one first in COMMANDS', () => {
		const map = effective({ 'go.trash': ['F2'], 'go.settings': ['F2'] });
		expect(map.get('go.trash')).toEqual(['F2']);
		expect(map.get('go.settings')).toEqual([]);
	});

	it("passes over a command's own fixed keys and keys that cannot work for it", () => {
		expect(effective({ 'task.next': ['j', 'ArrowDown'] }).get('task.next')).toEqual(['j']);
		expect(effective({ palette: ['q', 'Mod+p'] }).get('palette')).toEqual(['Mod+p']);
		expect(effective({ 'task.complete': ['g x', 'c'] }).get('task.complete')).toEqual(['c']);
	});

	it('leaves help-only commands with their fixed keys alone', () => {
		expect(effective({ 'panel.close': ['q'] }).get('panel.close')).toEqual([]);
	});
});

describe('assign', () => {
	it('drops an entry equal to the defaults', () => {
		expect(assign({ 'quick-add': ['n'] }, 'quick-add', ['q'])).toEqual({});
	});

	it('sorts ids and keeps the order of keys', () => {
		const out = assign({ undo: ['Mod+Shift+u'] }, 'quick-add', ['n', 'Alt+n']);
		expect(Object.keys(out)).toEqual(['quick-add', 'undo']);
		expect(out['quick-add']).toEqual(['n', 'Alt+n']);
		expect(JSON.stringify(assign(out, 'go.trash', ['F2']))).toBe(
			'{"go.trash":["F2"],"quick-add":["n","Alt+n"],"undo":["Mod+Shift+u"]}'
		);
	});

	it('says which commands changed', () => {
		const map = effective({ 'quick-add': ['n'] });
		expect(changed(map, 'quick-add')).toBe(true);
		expect(changed(map, 'search')).toBe(false);
	});
});

describe('reset', () => {
	it('brings a command back to its defaults', () => {
		expect(reset({ 'quick-add': ['n'], undo: ['F2'] }, 'quick-add')).toEqual({
			next: { undo: ['F2'] },
			also: []
		});
	});

	it('takes back a default another command holds after a swap, resetting that one too', () => {
		const swapped = { 'task.complete': ['t'], 'task.my-day': ['x'] };
		// Only removing the entry would leave Complete with no key at all.
		expect(effective(assign(swapped, 'task.complete', ['x'])).get('task.complete')).toEqual([]);
		const { next, also } = reset(swapped, 'task.complete');
		expect(also).toEqual(['task.my-day']);
		expect(next).toEqual({});
		expect(effective(next).get('task.complete')).toEqual(['x']);
	});

	it('goes on down the line of holders', () => {
		const chain = { 'task.complete': ['t'], 'task.my-day': ['1'], 'task.priority.urgent': ['x'] };
		const { next, also } = reset(chain, 'task.complete');
		// Complete's X is with Urgent, whose 1 is with My Day.
		expect(also).toEqual(['task.priority.urgent', 'task.my-day']);
		expect(next).toEqual({});
		// Something else the person changed is left alone.
		expect(reset({ ...chain, undo: ['F2'] }, 'task.complete').next).toEqual({ undo: ['F2'] });
	});
});

describe('the defaults', () => {
	it('have unique ids, in the shape the server takes', () => {
		const ids = COMMANDS.map((c) => c.id);
		expect(new Set(ids).size).toBe(ids.length);
		for (const id of ids) expect(id).toMatch(/^[a-z][a-z0-9.-]{0,47}$/);
	});

	it('never clash with each other', () => {
		const all = COMMANDS.flatMap((c) =>
			[...c.keys, ...('fixed' in c ? c.fixed : [])].map((b) => ({
				id: c.id,
				scope: c.scope as Scope,
				b,
				widget: c.scope === 'elsewhere' && 'remappable' in c
			}))
		);
		// Help-only rows elsewhere are each heard by one widget of their own
		// (the cropper, the panel edges, a menu), so they may share keys.
		for (const [i, a] of all.entries())
			for (const b of all.slice(i + 1))
				if (a.id !== b.id && !(a.widget && b.widget))
					expect(clash(a, b), `${a.id} ${a.b} / ${b.id} ${b.b}`).toBeNull();
	});

	it('name a registry command for every palette item that has a key', () => {
		const palette = readFileSync(
			new URL('../components/CommandPalette.svelte', import.meta.url),
			'utf8'
		);
		const named = [...palette.matchAll(/registry\('([^']+)'/g)].map((m) => m[1]);
		expect(named.length).toBeGreaterThan(10);
		for (const id of named) expect(isCommand(id), id).toBe(true);
	});
});
