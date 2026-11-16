import { describe, expect, it } from 'vitest';
import {
	formatDuration,
	minutesBetween,
	parseDuration,
	remaining,
	sayDuration,
	stopwatch,
	timeBadge
} from './duration';

describe('durations', () => {
	it('reads what people type', () => {
		const cases: [string, number | null][] = [
			['1h 30m', 90],
			['1h30m', 90],
			['90m', 90],
			['90 min', 90],
			['1.5h', 90],
			['1.5 hours', 90],
			['1 hr 5 min', 65],
			['1:30', 90],
			['0:05', 5],
			['2', 120],
			['0.25', 15],
			['.5', 30],
			['  2H ', 120],
			['45M', 45],
			['0', 0],
			['', null],
			['soon', null],
			['1:75', null],
			['h', null],
			['-1h', null],
			['1h 30', null]
		];
		for (const [text, want] of cases) expect(parseDuration(text), text).toBe(want);
	});

	it('writes minutes the short way and the spoken way', () => {
		expect(formatDuration(90)).toBe('1h 30m');
		expect(formatDuration(120)).toBe('2h');
		expect(formatDuration(45)).toBe('45m');
		expect(formatDuration(0)).toBe('0m');
		expect(formatDuration(-90)).toBe('1h 30m');
		expect(sayDuration(61)).toBe('1 hour 1 minute');
		expect(sayDuration(150)).toBe('2 hours 30 minutes');
		expect(sayDuration(0)).toBe('0 minutes');
	});

	it('counts a running timer', () => {
		expect(stopwatch(42 * 60 + 15)).toBe('0:42:15');
		expect(stopwatch(10 * 3600 + 5)).toBe('10:00:05');
		expect(stopwatch(-3)).toBe('0:00:00');
		expect(minutesBetween('2026-11-01T10:00:00Z', '2026-11-01T11:29:59Z')).toBe(89);
		expect(minutesBetween('2026-11-01T10:00:00Z', '2026-11-01T09:00:00Z')).toBe(0);
	});

	it('says what is left of an estimate, or how far past it', () => {
		expect(remaining(360, 200)).toEqual({ left: 160, over: 0 });
		expect(remaining(120, 210)).toEqual({ left: 0, over: 90 });
		expect(remaining(60, 60)).toEqual({ left: 0, over: 0 });
		expect(remaining(null, 30)).toBeNull();
	});
});

describe('a time badge', () => {
	const all = { estimate: true, logged: true, remaining: false };
	it('says the time logged against the estimate, and either alone', () => {
		expect(timeBadge(360, 200, all)).toEqual({
			text: '3h 20m / 6h',
			said: '3 hours 20 minutes logged of 6 hours estimated',
			over: false
		});
		expect(timeBadge(120, 210, all)?.over).toBe(true);
		expect(timeBadge(120, 0, all)?.text).toBe('2h');
		expect(timeBadge(null, 45, all)).toMatchObject({ text: '45m', over: false });
		expect(timeBadge(null, 0, all)).toBeNull();
		expect(timeBadge(120, 30, { estimate: false, logged: false, remaining: false })).toBeNull();
	});

	it('adds what is left, or how far past', () => {
		const left = { estimate: false, logged: false, remaining: true };
		expect(timeBadge(120, 30, left)).toMatchObject({ text: '1h 30m left', over: false });
		expect(timeBadge(120, 210, left)).toMatchObject({ text: '1h 30m over', over: true });
		expect(timeBadge(120, 30, { ...all, remaining: true })?.text).toBe('30m / 2h · 1h 30m left');
	});
});
