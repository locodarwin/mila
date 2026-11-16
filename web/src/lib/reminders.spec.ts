import { beforeAll, describe, expect, it } from 'vitest';
import { preferences } from './preferences.svelte';
import {
	describeReminder,
	dueMoment,
	duration,
	firesAt,
	localMoment,
	presets,
	remindAtFor
} from './reminders';
import type { Reminder, Task } from './sync/entities';

type Due = Pick<Task, 'dueDate' | 'dueTime'>;

// Times and dates as a 24-hour, month-first reader has them; dates.spec.ts
// covers the other ways.
beforeAll(() => preferences.set({ clock: '24', dateFormat: 'month-day' }));

// Wednesday, 28 October 2026, 08:00 where the tests run.
const now = new Date(2026, 9, 28, 8, 0);
const timed: Due = { dueDate: '2026-10-30', dueTime: '14:30' };
const allDay: Due = { dueDate: '2026-10-30', dueTime: null };
const undated: Due = { dueDate: null, dueTime: null };

const relative = (offset: number, task: Due = timed): Reminder => ({
	id: 'r',
	taskId: 't',
	offsetMinutes: offset,
	remindAt: remindAtFor(task, offset)!
});
const fixed = (at: Date): Reminder => ({
	id: 'r',
	taskId: 't',
	offsetMinutes: null,
	remindAt: at.toISOString()
});

describe('reminders', () => {
	it('falls due at the due time, or at 9:00 for an all-day task', () => {
		expect(dueMoment(timed)).toEqual(new Date(2026, 9, 30, 14, 30));
		expect(dueMoment(allDay)).toEqual(new Date(2026, 9, 30, 9, 0));
		expect(dueMoment(undated)).toBeNull();
	});

	it('works out a relative reminder from the due moment', () => {
		expect(remindAtFor(timed, 60)).toBe(new Date(2026, 9, 30, 13, 30).toISOString());
		expect(remindAtFor(allDay, 24 * 60)).toBe(new Date(2026, 9, 29, 9, 0).toISOString());
		expect(remindAtFor(undated, 15)).toBeNull();
		// It follows the task, whatever it last worked out to.
		const r = relative(15);
		expect(firesAt(r, { dueDate: '2026-11-02', dueTime: '10:00' })).toEqual(
			new Date(2026, 10, 2, 9, 45)
		);
		// With no due date it keeps that time.
		expect(firesAt(r, undated)).toEqual(new Date(2026, 9, 30, 14, 15));
	});

	it('counts whole days before as calendar days', () => {
		// 1 November 2026 is the day after daylight saving time ends in the
		// US; wherever the tests run, the day before at 9:00 is at 9:00.
		const sunday = { dueDate: '2026-11-01', dueTime: null };
		expect(remindAtFor(sunday, 1440)).toBe(new Date(2026, 9, 31, 9, 0).toISOString());
		expect(remindAtFor({ dueDate: '2026-03-08', dueTime: null }, 1440)).toBe(
			new Date(2026, 2, 7, 9, 0).toISOString()
		);
		expect(remindAtFor({ dueDate: '2026-11-01', dueTime: '18:00' }, 1440 + 30)).toBe(
			new Date(2026, 9, 31, 17, 30).toISOString()
		);
		expect(describeReminder(relative(1440, sunday), sunday, now)).toBe('The day before at 9:00');
	});

	it('offers presets that suit the task and are still to come', () => {
		expect(presets(timed, now).map((p) => [p.id, p.offset])).toEqual([
			['due', 0],
			['15m', 15],
			['1h', 60],
			['1d', 1440],
			// The morning of is relative too, so it moves and repeats with the task.
			['morning', 330],
			['tomorrow', undefined]
		]);
		// At 10:00 the morning of is an hour before, which is offered already.
		expect(presets({ dueDate: '2026-10-30', dueTime: '10:00' }, now).map((p) => p.id)).toEqual([
			'due',
			'15m',
			'1h',
			'1d',
			'tomorrow'
		]);
		expect(presets(allDay, now).map((p) => [p.id, p.offset])).toEqual([
			['morning', 0],
			['1d', 1440],
			['tomorrow', undefined]
		]);
		expect(presets(undated, now).map((p) => p.id)).toEqual(['tomorrow']);
		expect(presets(undated, now)[0].at).toBe(new Date(2026, 9, 29, 9, 0).toISOString());
		// An hour before a task due in half an hour has passed.
		const soon = { dueDate: '2026-10-28', dueTime: '08:30' };
		expect(presets(soon, now).map((p) => p.id)).toEqual(['due', '15m', 'tomorrow']);
		// The morning of is offered only when it comes before the due time.
		expect(presets({ dueDate: '2026-10-30', dueTime: '08:00' }, now).map((p) => p.id)).toEqual([
			'due',
			'15m',
			'1h',
			'1d',
			'tomorrow'
		]);
	});

	it('reads as people say it', () => {
		expect(describeReminder(relative(0), timed, now)).toBe('At the due time');
		expect(describeReminder(relative(60), timed, now)).toBe('1 hour before due');
		expect(describeReminder(relative(90), timed, now)).toBe('1 hour 30 minutes before due');
		expect(describeReminder(relative(330), timed, now)).toBe('The morning of the due date, 9:00');
		// Moved to another day, it is that day's morning.
		expect(firesAt(relative(330), { dueDate: '2026-11-02', dueTime: '14:30' })).toEqual(
			new Date(2026, 10, 2, 9, 0)
		);
		expect(describeReminder(relative(0, allDay), allDay, now)).toBe(
			'The morning of the due date, 9:00'
		);
		expect(describeReminder(relative(1440, allDay), allDay, now)).toBe('The day before at 9:00');
		expect(describeReminder(relative(2880, allDay), allDay, now)).toBe('2 days before at 9:00');
		expect(describeReminder(fixed(localMoment('2026-10-29', '09:00')), timed, now)).toBe(
			'Tomorrow at 9:00'
		);
		expect(describeReminder(fixed(localMoment('2026-10-28', '17:05')), timed, now)).toBe(
			'Today at 17:05'
		);
		expect(describeReminder(fixed(localMoment('2026-11-02', '09:00')), timed, now)).toBe(
			'Mon, Nov 2 at 9:00'
		);
		expect(describeReminder(fixed(localMoment('2027-01-04', '09:00')), timed, now)).toBe(
			'Mon, Jan 4, 2027 at 9:00'
		);
		// A relative reminder on a task that lost its due date shows its time.
		expect(describeReminder(relative(60), undated, now)).toBe('Fri, Oct 30 at 13:30');
	});

	it('says how long', () => {
		expect(duration(15)).toBe('15 minutes');
		expect(duration(60)).toBe('1 hour');
		expect(duration(1440 + 180)).toBe('1 day 3 hours');
		expect(duration(2 * 1440 + 1)).toBe('2 days 1 minute');
	});
});
