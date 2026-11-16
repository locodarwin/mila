import { describe, expect, it } from 'vitest';
import { scheduleChoices } from './schedule.svelte';

describe('scheduleChoices', () => {
	// 2026-10-29 is a Thursday.
	const dates = (firstDay: number) =>
		Object.fromEntries(scheduleChoices('2026-10-29', firstDay).map((c) => [c.id, c.date]));

	it('offers today, tomorrow, next week and no date', () => {
		expect(dates(1)).toEqual({
			today: '2026-10-29',
			tomorrow: '2026-10-30',
			'next-week': '2026-11-02',
			none: null
		});
	});
	it('starts next week on the user’s first day', () => {
		expect(dates(0)['next-week']).toBe('2026-11-01');
		expect(dates(6)['next-week']).toBe('2026-10-31');
	});
	it('means the coming week even on its first day', () => {
		const monday = scheduleChoices('2026-11-02', 1).find((c) => c.id === 'next-week');
		expect(monday?.date).toBe('2026-11-09');
	});
});
