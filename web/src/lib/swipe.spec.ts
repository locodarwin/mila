import { describe, expect, it } from 'vitest';
import { pullCloses, rubberBand, swipeOutcome } from './swipe';

describe('swipeOutcome', () => {
	it('counts a swipe a third of the way across, or 120px on a wide row', () => {
		expect(swipeOutcome(130, 0, 390)).toBe('right');
		expect(swipeOutcome(-130, 0, 390)).toBe('left');
		expect(swipeOutcome(110, 0, 390)).toBeNull();
		expect(swipeOutcome(121, 0, 1200)).toBe('right');
	});
	it('counts a quick flick past 40px', () => {
		expect(swipeOutcome(50, 0.8, 390)).toBe('right');
		expect(swipeOutcome(-50, -0.8, 390)).toBe('left');
		expect(swipeOutcome(30, 2, 390)).toBeNull();
	});
	it('ignores a flick back the other way', () => {
		expect(swipeOutcome(60, -0.8, 390)).toBeNull();
	});
});

describe('rubberBand', () => {
	it('follows the finger, then resists', () => {
		expect(rubberBand(100, 400)).toBe(100);
		expect(rubberBand(-100, 400)).toBe(-100);
		const far = rubberBand(400, 400);
		expect(far).toBeGreaterThan(180);
		expect(far).toBeLessThan(400);
		expect(rubberBand(-400, 400)).toBe(-far);
		expect(rubberBand(500, 400)).toBeGreaterThan(far);
	});
});

describe('pullCloses', () => {
	it('closes past a quarter of the screen, or on a flick', () => {
		expect(pullCloses(220, 0, 800)).toBe(true);
		expect(pullCloses(180, 0, 800)).toBe(false);
		expect(pullCloses(60, 0.9, 800)).toBe(true);
		expect(pullCloses(30, 2, 800)).toBe(false);
	});
});
