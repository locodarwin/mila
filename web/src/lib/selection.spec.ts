// Selecting several tasks (docs/PLAN.md section 3): what Ctrl-click, Shift-click
// and the select keys do to the selection.
import { afterEach, describe, expect, it } from 'vitest';
import { commandSelects, selection } from './selection.svelte';

const WS = 'ws';
const ORDER = ['a', 'b', 'c', 'd', 'e'];
const picked = () => [...selection.ids].sort();

describe('selection', () => {
	afterEach(() => selection.clear());

	it('adds and takes out one at a time', () => {
		selection.toggle(WS, 'b');
		selection.toggle(WS, 'd');
		expect(picked()).toEqual(['b', 'd']);
		selection.toggle(WS, 'b');
		expect(picked()).toEqual(['d']);
		expect(selection.has(WS, 'd')).toBe(true);
		expect(selection.has('other', 'd')).toBe(false);
	});

	it('stretches from the last one picked, keeping what was picked before', () => {
		selection.toggle(WS, 'a');
		selection.toggle(WS, 'c');
		selection.extend(WS, ORDER, 'e');
		expect(picked()).toEqual(['a', 'c', 'd', 'e']);
		// Stretching back takes off what it passed, but not what was picked before.
		selection.extend(WS, ORDER, 'd');
		expect(picked()).toEqual(['a', 'c', 'd']);
		selection.extend(WS, ORDER, 'b');
		expect(picked()).toEqual(['a', 'b', 'c']);
	});

	it('starts a stretch at the task itself when nothing was picked', () => {
		selection.extend(WS, ORDER, 'c');
		expect(picked()).toEqual(['c']);
	});

	it('Shift and an arrow stretch from the focused task and move focus', () => {
		expect(commandSelects('select.down', WS, 'b', ORDER)).toBe('c');
		expect(picked()).toEqual(['b', 'c']);
		expect(commandSelects('select.down', WS, 'c', ORDER)).toBe('d');
		expect(commandSelects('select.up', WS, 'd', ORDER)).toBe('c');
		expect(picked()).toEqual(['b', 'c']);
	});

	it('selects every task a list holds, and Escape clears only a selection', () => {
		expect(commandSelects('select.clear', WS, 'a', ORDER)).toBeNull();
		commandSelects('select.all', WS, 'a', ORDER);
		expect(picked()).toEqual(ORDER);
		expect(commandSelects('select.clear', WS, 'a', ORDER)).toBe('a');
		expect(picked()).toEqual([]);
	});

	it('a selection in another workspace starts afresh', () => {
		selection.toggle(WS, 'a');
		selection.toggle('other', 'b');
		expect(selection.ws).toBe('other');
		expect(picked()).toEqual(['b']);
	});
});
