import { describe, expect, it } from 'vitest';
import { blank, type ThemeDoc } from './document';
import { withValue } from './edit';
import { ThemeHistory } from './history.svelte';

const start = (): ThemeDoc => blank('standard', 'Test');
const px = (d: ThemeDoc, n: number) => withValue(d, 'radius-md', 'light', { px: n });

describe("the editor's history", () => {
	it('makes one step of a slider moved and let go, and saves only that', () => {
		const saved: ThemeDoc[] = [];
		const h = new ThemeHistory(start(), (d) => saved.push(d));
		h.change(px(h.doc, 4), false);
		h.change(px(h.doc, 6), false);
		expect(h.live).toBe(true);
		expect(saved).toEqual([]);
		h.change(px(h.doc, 8));
		expect(saved).toHaveLength(1);
		expect(h.undo()).toBe(true);
		expect(h.doc.tokens).toEqual({});
		expect(h.redo()).toBe(true);
		expect(h.doc.tokens).toEqual({ 'radius-md': { px: 8 } });
		expect(saved).toHaveLength(3);
	});

	it('keeps nothing for a change put back as it was, and cancel drops a live one', () => {
		const saved: ThemeDoc[] = [];
		const h = new ThemeHistory(start(), (d) => saved.push(d));
		const first = h.doc;
		h.change(px(h.doc, 4), false);
		h.change(first);
		expect(h.canUndo).toBe(false);
		h.change({ ...h.doc, name: '' }, false);
		h.cancel();
		expect(h.doc.name).toBe('Test');
		expect(saved).toEqual([]);
	});

	it('reverts to the version it opened with, as a step undo takes back', () => {
		const h = new ThemeHistory(start(), () => {});
		h.change(px(h.doc, 4));
		h.change(px(h.doc, 5));
		expect(h.changed).toBe(true);
		h.revert();
		expect(h.changed).toBe(false);
		h.undo();
		expect(h.doc.tokens).toEqual({ 'radius-md': { px: 5 } });
	});

	it('takes a version from elsewhere without saving it, unless a change is under way', () => {
		const saved: ThemeDoc[] = [];
		const h = new ThemeHistory(start(), (d) => saved.push(d));
		h.external(px(h.doc, 9));
		expect(h.doc.tokens).toEqual({ 'radius-md': { px: 9 } });
		expect(saved).toEqual([]);
		h.change(px(h.doc, 3), false);
		h.external(px(h.doc, 12));
		expect(h.doc.tokens).toEqual({ 'radius-md': { px: 3 } });
		h.undo();
		expect(h.doc.tokens).toEqual({ 'radius-md': { px: 9 } });
	});
});
