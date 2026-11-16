import { describe, expect, it } from 'vitest';
import { parseQuickAdd, type QuickAddContext } from './quickadd';

// Wednesday, 28 October 2026.
const ctx: QuickAddContext = {
	today: new Date(2026, 9, 28),
	projects: [
		{ id: 'p-work', name: 'Work' },
		{ id: 'p-kitchen', name: 'Kitchen remodel' }
	],
	labels: [
		{ id: 'l-phone', name: 'Phone' },
		{ id: 'l-errand', name: 'Errand' }
	],
	members: [
		{ id: 'u-shawn', name: 'Shawn Olson' },
		{ id: 'u-dana', name: 'Dana' }
	]
};

describe('parseQuickAdd', () => {
	it.each([
		['Buy milk', { title: 'Buy milk' }],
		[
			'Call Dana tomorrow 3pm #work !high @shawn +phone',
			{
				title: 'Call Dana',
				dueDate: '2026-10-29',
				dueTime: '15:00',
				projectId: 'p-work',
				priority: 'high',
				assigneeIds: ['u-shawn'],
				labelIds: ['l-phone']
			}
		],
		['Pay rent today', { title: 'Pay rent', dueDate: '2026-10-28' }],
		['Pay rent tod', { title: 'Pay rent', dueDate: '2026-10-28' }],
		['Dentist fri at 9:30am', { title: 'Dentist', dueDate: '2026-10-30', dueTime: '09:30' }],
		['Standup wed', { title: 'Standup', dueDate: '2026-11-04' }], // the next Wednesday, not today
		['Plan sprint next week', { title: 'Plan sprint', dueDate: '2026-11-02' }],
		['Review next mon', { title: 'Review', dueDate: '2026-11-09' }],
		['Renew passport in 3 days', { title: 'Renew passport', dueDate: '2026-10-31' }],
		['Renew passport in 2 weeks', { title: 'Renew passport', dueDate: '2026-11-11' }],
		['Taxes due nov 14', { title: 'Taxes', dueDate: '2026-11-14' }],
		['Taxes 14 november', { title: 'Taxes', dueDate: '2026-11-14' }],
		['Taxes 11/14', { title: 'Taxes', dueDate: '2026-11-14' }],
		['Party on 2026-12-31', { title: 'Party', dueDate: '2026-12-31' }],
		['Birthday jan 3', { title: 'Birthday', dueDate: '2027-01-03' }], // already passed this year
		['Lunch at noon', { title: 'Lunch', dueDate: '2026-10-28', dueTime: '12:00' }],
		['Call at 3 pm', { title: 'Call', dueDate: '2026-10-28', dueTime: '15:00' }],
		['Deploy 17:45 tomorrow', { title: 'Deploy', dueDate: '2026-10-29', dueTime: '17:45' }],
		['Tiles #kitchen-remodel', { title: 'Tiles', projectId: 'p-kitchen' }],
		['Tiles #kit', { title: 'Tiles', projectId: 'p-kitchen' }],
		['Fix it !1', { title: 'Fix it', priority: 'urgent' }],
		['Fix it !4', { title: 'Fix it', priority: 'low' }],
		[
			'Pick up +errand @dana',
			{ title: 'Pick up', labelIds: ['l-errand'], assigneeIds: ['u-dana'] }
		],
		// Unknown sigils and lookalikes stay in the title.
		['Fix #nope and !maybe', { title: 'Fix #nope and !maybe' }],
		['Read 3 chapters', { title: 'Read 3 chapters' }],
		['Email the 5 people', { title: 'Email the 5 people' }],
		['Watch Monday Night Football', { title: 'Watch Night Football', dueDate: '2026-11-02' }],
		['Feb 30 party', { title: 'Feb 30 party' }],
		// Repeats; with no date, the first one is the due date.
		['Stretch every day', { title: 'Stretch', recurrence: 'FREQ=DAILY', dueDate: '2026-10-28' }],
		[
			'Standup every weekday 9:30am',
			{
				title: 'Standup',
				recurrence: 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR',
				dueDate: '2026-10-28',
				dueTime: '09:30'
			}
		],
		['Review every week', { title: 'Review', recurrence: 'FREQ=WEEKLY', dueDate: '2026-10-28' }],
		[
			'Pay rent every month nov 1',
			{ title: 'Pay rent', recurrence: 'FREQ=MONTHLY', dueDate: '2026-11-01' }
		],
		['Renew every year', { title: 'Renew', recurrence: 'FREQ=YEARLY', dueDate: '2026-10-28' }],
		[
			'Water plants every 3 days',
			{ title: 'Water plants', recurrence: 'FREQ=DAILY;INTERVAL=3', dueDate: '2026-10-28' }
		],
		[
			'Mow every 2 weeks',
			{ title: 'Mow', recurrence: 'FREQ=WEEKLY;INTERVAL=2', dueDate: '2026-10-28' }
		],
		[
			'Haircut every other month',
			{ title: 'Haircut', recurrence: 'FREQ=MONTHLY;INTERVAL=2', dueDate: '2026-10-28' }
		],
		[
			'Bins every monday',
			{ title: 'Bins', recurrence: 'FREQ=WEEKLY;BYDAY=MO', dueDate: '2026-11-02' }
		],
		[
			'Gym every mon and thu',
			{ title: 'Gym', recurrence: 'FREQ=WEEKLY;BYDAY=MO,TH', dueDate: '2026-10-29' }
		],
		[
			'Gym every thursday and monday 7am',
			{
				title: 'Gym',
				recurrence: 'FREQ=WEEKLY;BYDAY=MO,TH',
				dueDate: '2026-10-29',
				dueTime: '07:00'
			}
		],
		[
			'Piano every mon, wed, fri',
			{ title: 'Piano', recurrence: 'FREQ=WEEKLY;BYDAY=MO,WE,FR', dueDate: '2026-10-28' }
		],
		['Vitamins daily', { title: 'Vitamins', recurrence: 'FREQ=DAILY', dueDate: '2026-10-28' }],
		['Backups weekly fri', { title: 'Backups', recurrence: 'FREQ=WEEKLY', dueDate: '2026-10-30' }],
		['Invoice monthly', { title: 'Invoice', recurrence: 'FREQ=MONTHLY', dueDate: '2026-10-28' }],
		['Taxes yearly', { title: 'Taxes', recurrence: 'FREQ=YEARLY', dueDate: '2026-10-28' }],
		// Reminders: a time alone is on the due date, else today (tomorrow if
		// it has passed); a date alone is at 9:00; "before" counts back from
		// the due moment and needs a due date.
		[
			'Call Dana tomorrow 3pm remind 2pm',
			{
				title: 'Call Dana',
				dueDate: '2026-10-29',
				dueTime: '15:00',
				reminder: { date: '2026-10-29', time: '14:00' }
			}
		],
		['Pay rent remind 3pm', { title: 'Pay rent', reminder: { date: '2026-10-28', time: '15:00' } }],
		[
			'Pay rent remind tomorrow 9am',
			{ title: 'Pay rent', reminder: { date: '2026-10-29', time: '09:00' } }
		],
		[
			'Pay rent remind me fri at 8:30am',
			{ title: 'Pay rent', reminder: { date: '2026-10-30', time: '08:30' } }
		],
		['Pay rent remind mon', { title: 'Pay rent', reminder: { date: '2026-11-02', time: '09:00' } }],
		[
			'Dentist fri 10am remind 1h before',
			{ title: 'Dentist', dueDate: '2026-10-30', dueTime: '10:00', reminder: { offset: 60 } }
		],
		[
			'Dentist fri remind 2 days before',
			{ title: 'Dentist', dueDate: '2026-10-30', reminder: { offset: 2880 } }
		],
		[
			'Dentist remind me 30 minutes before tomorrow 10am',
			{ title: 'Dentist', dueDate: '2026-10-29', dueTime: '10:00', reminder: { offset: 30 } }
		],
		// Before nothing, or about something else, it stays in the title.
		['Sleep remind 1h before', { title: 'Sleep remind 1h before' }],
		// More than a year before is more than the server allows.
		['Trip fri remind 9000h before', { title: 'Trip remind 9000h before', dueDate: '2026-10-30' }],
		[
			'Trip fri remind 9999 days before',
			{ title: 'Trip remind 9999 days before', dueDate: '2026-10-30' }
		],
		['Remind Dana about rent', { title: 'Remind Dana about rent' }],
		// "every" alone, or before something else, stays in the title.
		['Read every chapter', { title: 'Read every chapter' }],
		['Every so often', { title: 'Every so often' }]
	])('%s', (input, want) => {
		const got = parseQuickAdd(input, ctx);
		expect(got).toMatchObject({ labelIds: [], assigneeIds: [], ...want });
		for (const k of [
			'dueDate',
			'dueTime',
			'projectId',
			'priority',
			'recurrence',
			'reminder'
		] as const) {
			if (!(k in want)) expect(got[k], k).toBeUndefined();
		}
	});

	it('marks what it recognized, for highlighting', () => {
		const got = parseQuickAdd('Call Dana tomorrow #work', ctx);
		expect(
			got.tokens.map((t) => [t.kind, 'Call Dana tomorrow #work'.slice(t.start, t.end)])
		).toEqual([
			['date', 'tomorrow'],
			['project', '#work']
		]);
	});

	it('marks a reminder, for highlighting', () => {
		const text = 'Dentist fri 10am remind 1h before';
		const got = parseQuickAdd(text, ctx);
		expect(got.tokens.map((t) => [t.kind, text.slice(t.start, t.end)])).toEqual([
			['date', 'fri'],
			['time', '10am'],
			['reminder', 'remind 1h before']
		]);
	});

	it('moves a reminder whose time has passed today to tomorrow', () => {
		const evening = { ...ctx, today: new Date(2026, 9, 28, 16, 0) };
		expect(parseQuickAdd('Pay rent remind 3pm', evening).reminder).toEqual({
			date: '2026-10-29',
			time: '15:00'
		});
	});

	it('leaves a reminder whose moment has passed in the title', () => {
		const at = (h: number, m = 0) => ({ ...ctx, today: new Date(2026, 9, 28, h, m) });
		for (const [input, title] of [
			['Call Dana today 5pm remind 2pm', 'Call Dana remind 2pm'],
			['Call Dana today 5pm remind 1h before', 'Call Dana remind 1h before'],
			['Call Dana 5pm remind today 3pm', 'Call Dana remind today 3pm']
		]) {
			const got = parseQuickAdd(input, at(16, 30));
			expect(got.reminder, input).toBeUndefined();
			expect(got.title, input).toBe(title);
			expect(got.tokens.map((t) => t.kind)).not.toContain('reminder');
		}
		expect(parseQuickAdd('Call Dana today 5pm remind 1h before', at(15)).reminder).toEqual({
			offset: 60
		});
		expect(parseQuickAdd('Call Dana today 5pm remind 2pm', at(13)).reminder).toEqual({
			date: '2026-10-28',
			time: '14:00'
		});
	});

	it('marks a repeat, for highlighting', () => {
		const text = 'Gym every mon and thu 7am';
		const got = parseQuickAdd(text, ctx);
		expect(got.tokens.map((t) => [t.kind, text.slice(t.start, t.end)])).toEqual([
			['repeat', 'every mon and thu'],
			['time', '7am']
		]);
	});
});
