import { describe, expect, it } from 'vitest';
import { placeNear } from './popover';

const view = { width: 400, height: 800 };
const menu = { width: 220, height: 200 };
const button = (left: number, top: number, w = 30, h = 30) => ({
	left,
	top,
	right: left + w,
	bottom: top + h
});

describe('placeNear', () => {
	it('puts a menu under its button, lined up with its left edge', () => {
		expect(placeNear(button(50, 100), menu, view)).toEqual({ top: 134, left: 50 });
	});
	it('opens above when there is no room below', () => {
		expect(placeNear(button(50, 700), menu, view)).toEqual({ top: 496, left: 50 });
	});
	it('lines up with the right edge near the right side', () => {
		expect(placeNear(button(340, 100), menu, view).left).toBe(150);
	});
	it('stays on the screen whatever happens', () => {
		const huge = { width: 600, height: 900 };
		const at = placeNear(button(10, 400), huge, view);
		expect(at.left).toBe(8);
		expect(at.top).toBe(8);
	});
});
