// Scheduling a task in a tap: one chooser for the whole app (SchedulePicker
// in the app layout), opened for a task by its row's schedule button or by
// swiping the row left.
import { formatDate, weekStart, weekdayName } from './dates';
import { addDaysTo, weekdayOfDate } from './recurrence';

export interface ScheduleChoice {
	id: 'today' | 'tomorrow' | 'next-week' | 'none';
	label: string;
	/** The day it means, to show beside the label. */
	hint: string;
	date: string | null;
}

/** JavaScript's day of the week (0 for Sunday) of a YYYY-MM-DD date. */
function dayOfWeek(iso: string): number {
	return (weekdayOfDate(iso) + 1) % 7;
}

/**
 * The quick choices: today, tomorrow, the first day of next week (the
 * user's week, as quick add's "next week"), and no date.
 */
export function scheduleChoices(today: string, firstDay = weekStart()): ScheduleChoice[] {
	const tomorrow = addDaysTo(today, 1);
	const nextWeek = addDaysTo(today, (firstDay - dayOfWeek(today) + 7) % 7 || 7);
	const short = (d: string) => weekdayName(dayOfWeek(d));
	return [
		{ id: 'today', label: 'Today', hint: short(today), date: today },
		{ id: 'tomorrow', label: 'Tomorrow', hint: short(tomorrow), date: tomorrow },
		{
			id: 'next-week',
			label: 'Next week',
			hint: formatDate(nextWeek, { weekday: 'short' }),
			date: nextWeek
		},
		{ id: 'none', label: 'No date', hint: '', date: null }
	];
}

export interface ScheduleTarget {
	ws: string;
	taskId: string;
	/** What it opened from, to sit beside on a wide screen and return focus to. */
	anchor: HTMLElement | null;
}

class Scheduler {
	target = $state<ScheduleTarget | null>(null);

	open(ws: string, taskId: string, anchor: HTMLElement | null = null) {
		this.target = { ws, taskId, anchor };
	}

	close() {
		this.target = null;
	}
}

export const scheduler = new Scheduler();
