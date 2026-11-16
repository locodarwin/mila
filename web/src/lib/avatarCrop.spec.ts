import { describe, expect, it } from 'vitest';
import {
	clampPan,
	coverScale,
	cropRect,
	initialView,
	MAX_OUTPUT,
	maxScale,
	MIN_SIDE,
	outputSide,
	zoomAbout,
	type Size,
	type View
} from './avatarCrop';

const STAGE = 280;

describe('coverScale', () => {
	it('makes the shorter side span the stage', () => {
		expect(coverScale(560, 1120, STAGE)).toBe(0.5);
		expect(coverScale(1000, 140, STAGE)).toBe(2);
		expect(coverScale(280, 280, STAGE)).toBe(1);
	});
});

describe('clampPan', () => {
	const img: Size = { w: 1000, h: 500 };
	const cover = coverScale(img.w, img.h, STAGE);

	it('never zooms out past covering, nor in past the most', () => {
		expect(clampPan({ scale: cover / 2, x: 0, y: 0 }, img, STAGE).scale).toBe(cover);
		expect(clampPan({ scale: 1000, x: 0, y: 0 }, img, STAGE).scale).toBe(maxScale(img, STAGE));
	});

	it('keeps the stage covered at every edge', () => {
		const at = (x: number, y: number) => clampPan({ scale: cover, x, y }, img, STAGE);
		expect(at(50, 0).x).toBe(0); // a gap on the left
		expect(at(0, 50).y).toBe(0); // a gap at the top
		expect(at(-5000, 0).x).toBe(STAGE - img.w * cover); // a gap on the right
		expect(at(0, -5000).y).toBe(STAGE - img.h * cover); // a gap at the bottom
		expect(at(-100, 0)).toEqual({ scale: cover, x: -100, y: 0 }); // within bounds
	});

	it('never lets a crop span fewer than the smallest side', () => {
		const tiny: Size = { w: 20, h: 20 };
		const v = clampPan({ scale: 1e6, x: 0, y: 0 }, tiny, STAGE);
		expect(cropRect(v, tiny, STAGE).side).toBeGreaterThanOrEqual(MIN_SIDE - 1e-9);
	});
});

describe('zoomAbout', () => {
	it('keeps the point zoomed about where it is', () => {
		const view: View = { scale: 0.7, x: -40, y: -15 };
		const point = { x: 120, y: 200 };
		const before = { x: (point.x - view.x) / view.scale, y: (point.y - view.y) / view.scale };
		for (const f of [1.15, 1 / 1.15, 3, 0.25]) {
			const z = zoomAbout(view, f, point);
			expect(z.scale).toBeCloseTo(view.scale * f);
			expect((point.x - z.x) / z.scale).toBeCloseTo(before.x);
			expect((point.y - z.y) / z.scale).toBeCloseTo(before.y);
		}
	});
});

describe('cropRect', () => {
	it('is the middle square at first', () => {
		const img = { w: 1000, h: 500 };
		const r = cropRect(initialView(img, STAGE), img, STAGE);
		expect(r.sx).toBeCloseTo(250);
		expect(r.sy).toBeCloseTo(0);
		expect(r.side).toBeCloseTo(500);
	});

	it('stays inside the picture across many zooms and pans', () => {
		let seed = 7;
		const rand = () => ((seed = (seed * 16807) % 2147483647) / 2147483647) * 2 - 1;
		for (const img of [
			{ w: 1000, h: 500 },
			{ w: 300, h: 2000 },
			{ w: 17, h: 17 },
			{ w: 4000, h: 3000 }
		]) {
			let view = initialView(img, STAGE);
			for (let i = 0; i < 500; i++) {
				view = clampPan(
					zoomAbout(
						{ ...view, x: view.x + rand() * 200, y: view.y + rand() * 200 },
						Math.exp(rand()),
						{ x: (rand() + 1) * 140, y: (rand() + 1) * 140 }
					),
					img,
					STAGE
				);
				const r = cropRect(view, img, STAGE);
				expect(r.sx).toBeGreaterThanOrEqual(0);
				expect(r.sy).toBeGreaterThanOrEqual(0);
				expect(r.sx + r.side).toBeLessThanOrEqual(img.w + 1e-9);
				expect(r.sy + r.side).toBeLessThanOrEqual(img.h + 1e-9);
				expect(r.side).toBeGreaterThan(0);
			}
		}
	});
});

describe('outputSide', () => {
	it('sends the crop at its own size, up to the most', () => {
		expect(outputSide(100.4)).toBe(100);
		expect(outputSide(2000)).toBe(MAX_OUTPUT);
		expect(outputSide(512)).toBe(512);
	});
});
