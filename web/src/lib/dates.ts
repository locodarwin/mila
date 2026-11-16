// Every date and time the app shows goes through here, so the date
// preferences (first day of the week, date format, 12- or 24-hour clock)
// hold everywhere. "auto" follows the device's language and region.
import { preferences, type Preferences } from './preferences.svelte';

type Style = Pick<Preferences, 'weekStart' | 'dateFormat' | 'clock'>;

const current = (): Style => preferences.current;

/** How this device writes times when left to itself. */
export function deviceClock(): '12' | '24' {
	try {
		const cycle = new Intl.DateTimeFormat(undefined, { hour: 'numeric' }).resolvedOptions()
			.hourCycle;
		return cycle === 'h11' || cycle === 'h12' ? '12' : '24';
	} catch {
		return '24';
	}
}

/** Whether this device writes the day before the month, or the month first. */
export function deviceDateOrder(): 'month-day' | 'day-month' {
	try {
		const parts = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' })
			.formatToParts(new Date(2026, 10, 3))
			.map((p) => p.type);
		return parts.indexOf('day') < parts.indexOf('month') ? 'day-month' : 'month-day';
	} catch {
		return 'month-day';
	}
}

/** The first day of this device's week, 0 for Sunday to 6 for Saturday. */
export function deviceWeekStart(): number {
	try {
		const locale = new Intl.Locale(navigator.language) as Intl.Locale & {
			getWeekInfo?: () => { firstDay: number };
			weekInfo?: { firstDay: number };
		};
		const info = locale.getWeekInfo?.() ?? locale.weekInfo;
		// Intl counts from Monday as 1 to Sunday as 7.
		if (info) return info.firstDay % 7;
	} catch {
		// no week information here
	}
	return 1;
}

/** The first day of the week, 0 for Sunday to 6 for Saturday. */
export function weekStart(style: Style = current()): number {
	switch (style.weekStart) {
		case 'sunday':
			return 0;
		case 'monday':
			return 1;
		case 'saturday':
			return 6;
	}
	return deviceWeekStart();
}

function hour12(style: Style): boolean {
	return (style.clock === 'auto' ? deviceClock() : style.clock) === '12';
}

/** The local time an all-day task falls due, as the server counts it (reminders.ts). */
export const ALL_DAY_AT = '09:00';

/** A clock time from "14:30" (as tasks keep them): "2:30 PM" or "14:30". */
export function formatTime(clock: string, style: Style = current()): string {
	const m = /^(\d{1,2}):(\d{2})/.exec(clock);
	if (!m) return clock;
	const h = Number(m[1]);
	if (!hour12(style)) return `${h}:${m[2]}`;
	return `${h % 12 || 12}:${m[2]} ${h < 12 ? 'AM' : 'PM'}`;
}

/** The clock time of a moment, where the user is. */
export function formatClock(d: Date, style: Style = current()): string {
	return formatTime(`${d.getHours()}:${String(d.getMinutes()).padStart(2, '0')}`, style);
}

export interface DateOptions {
	weekday?: 'short' | 'long';
	month?: 'short' | 'long';
	/** Add the year: always, never, or (the default) when it is not this year. */
	year?: boolean;
}

/** The locale that writes dates in the chosen order; undefined is the device's. */
function localeOf(style: Style): string | undefined {
	switch (style.dateFormat) {
		case 'month-day':
			return 'en-US';
		case 'day-month':
			return 'en-GB';
	}
	return undefined;
}

/**
 * A date, from "2026-11-03" or a Date: "Nov 3", "3 Nov" or "2026-11-03",
 * with a weekday if asked ("Tue, Nov 3").
 */
export function formatDate(
	day: string | Date,
	opts: DateOptions = {},
	style: Style = current(),
	now = new Date()
): string {
	const d = typeof day === 'string' ? new Date(day + 'T00:00') : day;
	if (Number.isNaN(d.getTime())) return String(day);
	const year = opts.year ?? d.getFullYear() !== now.getFullYear();
	if (style.dateFormat === 'iso') {
		const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
		return opts.weekday ? `${weekdayName(d.getDay(), opts.weekday, style)}, ${iso}` : iso;
	}
	return d.toLocaleDateString(localeOf(style), {
		...(opts.weekday ? { weekday: opts.weekday } : {}),
		month: opts.month ?? 'short',
		day: 'numeric',
		...(year ? { year: 'numeric' } : {})
	});
}

/** A month and its year, for a calendar's title: "November 2026". */
export function formatMonth(month: string, style: Style = current()): string {
	return new Date(month + '-01T00:00').toLocaleDateString(
		style.dateFormat === 'auto' ? undefined : 'en-US',
		{ month: 'long', year: 'numeric' }
	);
}

/** A month's short name, for a timeline's scale: "Nov". */
export function formatMonthShort(month: string, style: Style = current()): string {
	return new Date(month + '-01T00:00').toLocaleDateString(
		style.dateFormat === 'auto' ? undefined : 'en-US',
		{ month: 'short' }
	);
}

/** A weekday's name, 0 for Sunday. */
export function weekdayName(
	day: number,
	width: 'short' | 'long' = 'short',
	style: Style = current()
): string {
	// 2026-11-01 was a Sunday.
	return new Date(2026, 10, 1 + day).toLocaleDateString(
		style.dateFormat === 'auto' ? undefined : 'en-US',
		{ weekday: width }
	);
}

/** The week's day numbers (0 for Sunday) from its first day. */
export function weekDays(style: Style = current()): number[] {
	const first = weekStart(style);
	return Array.from({ length: 7 }, (_, i) => (first + i) % 7);
}

/** How many days a date is past the first day of its week. */
export function daysIntoWeek(d: Date, style: Style = current()): number {
	return (d.getDay() - weekStart(style) + 7) % 7;
}
