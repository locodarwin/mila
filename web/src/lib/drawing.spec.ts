import { describe, expect, it } from 'vitest';
import {
	alignTo,
	boundsOf,
	DRAWING_STARTERS,
	drawingSvg,
	endsOf,
	fillToken,
	heightFor,
	roughMeasure,
	routeOf,
	shapeLabel,
	shapeOf,
	strokeToken,
	wrapText,
	type Paint,
	type Shape
} from './drawing';

const shape = (over: Partial<Shape>): Shape => ({
	id: 's',
	drawingId: 'd',
	kind: 'rect',
	box: { x: 0, y: 0, w: 100, h: 50 },
	points: null,
	text: '',
	style: {},
	fromId: null,
	toId: null,
	imageId: null,
	sortKey: 'a0',
	...over
});

describe('drawings', () => {
	it('read rows, whatever they hold', () => {
		const s = shapeOf('x', { kind: 'nonsense', box: { x: 'a', y: 2, w: -4 }, style: null });
		expect(s.kind).toBe('rect');
		expect(s.box).toEqual({ x: 0, y: 2, w: 0, h: 0 });
		expect(s.style).toEqual({});
	});

	it('color shapes by tokens, never colors', () => {
		expect(fillToken(shape({}))).toBe('diagram-node-bg');
		expect(fillToken(shape({ kind: 'sticky' }))).toBe('diagram-note-bg');
		expect(fillToken(shape({ style: { fill: 'green' } }))).toBe('highlight-green');
		expect(fillToken(shape({ style: { fill: 'none' } }))).toBeNull();
		expect(strokeToken(shape({ kind: 'arrow' }))).toBe('diagram-line');
		expect(strokeToken(shape({ style: { stroke: '3' } }))).toBe('chart-series-3');
	});

	it('join lines to the sides of their shapes that face each other', () => {
		const a = shape({ id: 'a', box: { x: 0, y: 0, w: 100, h: 50 } });
		const b = shape({ id: 'b', box: { x: 300, y: 0, w: 100, h: 50 } });
		const line = shape({ id: 'l', kind: 'arrow', fromId: 'a', toId: 'b' });
		const byId = new Map([a, b, line].map((s) => [s.id, s]));
		const ends = endsOf(line, byId);
		expect(ends.a).toEqual([100, 25]);
		expect(ends.b).toEqual([300, 25]);
		expect(ends.aSide).toBe('right');
		expect(ends.bSide).toBe('left');
		// An arrow runs in elbows unless told otherwise; here it is straight anyway.
		expect(routeOf(line, ends).d).toBe('M100 25L200 25L200 25L300 25');
		expect(routeOf({ ...line, style: { route: 'straight' } }, ends).d).toBe('M100 25L300 25');
	});

	it('keep a line where it was when its shape is gone', () => {
		const line = shape({
			kind: 'line',
			fromId: 'gone',
			box: { x: 10, y: 10, w: 0, h: 0 },
			points: [
				[0, 0],
				[50, 20]
			]
		});
		expect(endsOf(line, new Map()).b).toEqual([60, 30]);
	});

	it('wrap words to fit, and grow a shape for them', () => {
		expect(wrapText('one two three four', 50, 10, false, roughMeasure)).toEqual([
			'one two',
			'three',
			'four'
		]);
		expect(wrapText('a\n\nb', 100, 10, false, roughMeasure)).toEqual(['a', '', 'b']);
		const tall = shape({ text: 'many words that will not fit on one line of this box at all' });
		expect(heightFor(tall, roughMeasure)).toBeGreaterThan(50);
		expect(heightFor(shape({ text: 'Hi' }), roughMeasure)).toBe(50);
	});

	it('line up a box with another nearby, within reach', () => {
		const a = alignTo({ x: 103, y: 300, w: 50, h: 50 }, [{ x: 100, y: 0, w: 80, h: 40 }], 6);
		expect(a.dx).toBe(-3);
		expect(a.dy).toBe(0);
		expect(a.guides).toEqual([{ x: 100 }]);
	});

	it('name shapes for screen readers', () => {
		const a = shape({ id: 'a', text: 'Web' });
		const b = shape({ id: 'b', kind: 'cylinder', text: 'Database' });
		const line = shape({ id: 'l', kind: 'arrow', fromId: 'a', toId: 'b' });
		const byId = new Map([a, b, line].map((s) => [s.id, s]));
		expect(shapeLabel(a, byId)).toBe('Box: Web');
		expect(shapeLabel(line, byId)).toBe('Arrow from Web to Database');
	});

	it('write a picture in the colors given, with nothing to run', () => {
		const paint: Paint = {
			color: (t) => `paint-${t}`,
			measure: roughMeasure,
			font: 'Inter',
			image: () => null
		};
		const svg = drawingSvg([shape({ text: 'A <b> & c' })], paint, 'Flow', 'paint-page');
		expect(svg).toContain('<title>Flow</title>');
		expect(svg).toContain('A &lt;b&gt; &amp; c');
		expect(svg).toContain('fill="paint-diagram-node-bg"');
		expect(svg).not.toContain('var(');
		expect(svg).not.toContain('<script');
	});

	it('start from shapes that join what they name', () => {
		for (const s of DRAWING_STARTERS) {
			const keys = new Set(s.shapes.map((x) => x.key));
			expect(keys.size, s.id).toBe(s.shapes.length);
			for (const x of s.shapes) {
				if (x.from) expect(keys.has(x.from), `${s.id} ${x.key}`).toBe(true);
				if (x.to) expect(keys.has(x.to), `${s.id} ${x.key}`).toBe(true);
			}
		}
		expect(boundsOf([]).w).toBeGreaterThan(0);
	});
});
