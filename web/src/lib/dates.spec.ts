import { describe, expect, it } from 'vitest';
import {
	daysIntoWeek,
	formatClock,
	formatDate,
	formatMonth,
	formatTime,
	weekDays,
	weekStart,
	weekdayName
} from './dates';

const style = (
	dateFormat: 'month-day' | 'day-month' | 'iso',
	clock: '12' | '24' = '24',
	weekStart: 'sunday' | 'monday' | 'saturday' = 'monday'
) => ({ dateFormat, clock, weekStart });

// Thursday, October 29, 2026.
const now = new Date(2026, 9, 29, 12);

describe('formatTime', () => {
	it('writes 12- and 24-hour times', () => {
		expect(formatTime('14:30', style('iso', '24'))).toBe('14:30');
		expect(formatTime('09:05', style('iso', '24'))).toBe('9:05');
		expect(formatTime('14:30', style('iso', '12'))).toBe('2:30 PM');
		expect(formatTime('00:15', style('iso', '12'))).toBe('12:15 AM');
		expect(formatTime('12:00', style('iso', '12'))).toBe('12:00 PM');
	});

	it('leaves what is not a time alone', () => {
		expect(formatTime('soon', style('iso', '12'))).toBe('soon');
	});

	it('writes a moment as a clock time', () => {
		expect(formatClock(new Date(2026, 9, 29, 17, 5), style('iso', '12'))).toBe('5:05 PM');
	});
});

describe('formatDate', () => {
	it('puts the month or the day first, or writes ISO', () => {
		expect(formatDate('2026-11-03', {}, style('month-day'), now)).toBe('Nov 3');
		expect(formatDate('2026-11-03', {}, style('day-month'), now)).toBe('3 Nov');
		expect(formatDate('2026-11-03', {}, style('iso'), now)).toBe('2026-11-03');
	});

	it('adds the weekday when asked', () => {
		expect(formatDate('2026-11-03', { weekday: 'short' }, style('month-day'), now)).toBe(
			'Tue, Nov 3'
		);
		expect(formatDate('2026-11-03', { weekday: 'long' }, style('day-month'), now)).toBe(
			'Tuesday 3 Nov'
		);
		expect(formatDate('2026-11-03', { weekday: 'short' }, style('iso'), now)).toBe(
			'Tue, 2026-11-03'
		);
	});

	it('adds the year for another year, or when asked', () => {
		expect(formatDate('2027-01-04', {}, style('month-day'), now)).toBe('Jan 4, 2027');
		expect(formatDate('2027-01-04', {}, style('day-month'), now)).toBe('4 Jan 2027');
		expect(formatDate('2027-01-04', { year: false }, style('month-day'), now)).toBe('Jan 4');
		expect(formatDate('2026-11-03', { month: 'long', year: true }, style('month-day'), now)).toBe(
			'November 3, 2026'
		);
	});

	it('writes a month for a calendar title', () => {
		expect(formatMonth('2026-11', style('day-month'))).toBe('November 2026');
	});
});

describe('weeks', () => {
	it('start on the chosen day', () => {
		expect(weekStart(style('iso', '24', 'sunday'))).toBe(0);
		expect(weekStart(style('iso', '24', 'saturday'))).toBe(6);
		expect(weekDays(style('iso', '24', 'monday'))).toEqual([1, 2, 3, 4, 5, 6, 0]);
		expect(weekDays(style('iso', '24', 'saturday'))).toEqual([6, 0, 1, 2, 3, 4, 5]);
		expect(weekdayName(0, 'short', style('iso'))).toBe('Sun');
	});

	it('count the days since the week began', () => {
		// November 1, 2026 is a Sunday.
		const sunday = new Date(2026, 10, 1);
		expect(daysIntoWeek(sunday, style('iso', '24', 'sunday'))).toBe(0);
		expect(daysIntoWeek(sunday, style('iso', '24', 'monday'))).toBe(6);
		expect(daysIntoWeek(sunday, style('iso', '24', 'saturday'))).toBe(1);
	});
});
