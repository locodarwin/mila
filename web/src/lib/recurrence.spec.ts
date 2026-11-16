import { describe as group, expect, it } from 'vitest';
import vectors from '../../../shared/recurrence-vectors.json';
import {
	describe,
	describeAfter,
	firstDue,
	formatRule,
	hashId,
	nextDue,
	nextTaskId,
	parseRule,
	repeatLabel,
	sha256,
	tryParseRule,
	upcoming,
	type RepeatMode
} from './recurrence';

const hex = (b: Uint8Array) => Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');

group('repeat rules', () => {
	it('agree with the server on what is valid', () => {
		for (const r of vectors.valid) expect(() => parseRule(r), r).not.toThrow();
		for (const r of vectors.invalid) expect(() => parseRule(r), r).toThrow();
	});

	it('agree with the server on the next date', () => {
		for (const c of vectors.next) {
			const got = nextDue(parseRule(c.rule), c.mode as RepeatMode, c.due, c.today);
			expect(got, `${c.rule} ${c.mode} from ${c.due} on ${c.today} ${c.note ?? ''}`).toBe(c.want);
		}
	});

	it('agree with the server on the IDs of next tasks', () => {
		for (const c of vectors.ids) expect(nextTaskId(c.after)).toBe(c.want);
	});

	it('hash like SHA-256', () => {
		expect(hex(sha256(new TextEncoder().encode('')))).toBe(
			'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'
		);
		expect(hex(sha256(new TextEncoder().encode('abc')))).toBe(
			'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad'
		);
		// Two blocks once padded.
		const long = 'abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq';
		expect(hex(sha256(new TextEncoder().encode(long)))).toBe(
			'248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1'
		);
		expect(hashId('x/2026-01-01')).toMatch(
			/^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-/
		);
	});

	it('are written in one form', () => {
		for (const [input, want] of [
			['FREQ=DAILY;INTERVAL=1', 'FREQ=DAILY'],
			['INTERVAL=2;FREQ=WEEKLY;BYDAY=TH,MO', 'FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH'],
			['UNTIL=20271231;FREQ=MONTHLY;BYDAY=-1FR', 'FREQ=MONTHLY;BYDAY=-1FR;UNTIL=20271231'],
			['FREQ=MONTHLY;BYMONTHDAY=-1', 'FREQ=MONTHLY;BYMONTHDAY=-1']
		])
			expect(formatRule(parseRule(input))).toBe(want);
		for (const r of vectors.valid)
			expect(formatRule(parseRule(formatRule(parseRule(r))))).toBe(formatRule(parseRule(r)));
		expect(tryParseRule('FREQ=HOURLY')).toBeNull();
		expect(tryParseRule(null)).toBeNull();
	});

	it.each([
		['FREQ=DAILY', 'schedule', null, 'Every day'],
		['FREQ=DAILY;INTERVAL=3', 'completion', null, '3 days after completion'],
		['FREQ=WEEKLY', 'completion', null, '1 week after completion'],
		['FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH', 'schedule', null, 'Every 2 weeks on Mon and Thu'],
		['FREQ=WEEKLY;BYDAY=MO,WE,FR', 'schedule', null, 'Weekly on Mon, Wed, and Fri'],
		['FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR', 'schedule', null, 'Every weekday'],
		['FREQ=WEEKLY', 'schedule', '2026-10-28', 'Weekly on Wed'],
		['FREQ=MONTHLY;BYDAY=-1FR', 'schedule', null, 'Monthly on the last Friday'],
		['FREQ=MONTHLY;BYDAY=2TU', 'schedule', null, 'Monthly on the second Tuesday'],
		['FREQ=MONTHLY;BYDAY=-2MO', 'schedule', null, 'Monthly on the second to last Monday'],
		['FREQ=MONTHLY;BYMONTHDAY=-1', 'schedule', null, 'Monthly on the last day'],
		['FREQ=MONTHLY;INTERVAL=3;BYMONTHDAY=22', 'schedule', null, 'Every 3 months on the 22nd'],
		['FREQ=MONTHLY', 'schedule', '2026-10-11', 'Monthly on the 11th'],
		['FREQ=YEARLY', 'schedule', '2026-10-28', 'Yearly on Oct 28'],
		['FREQ=DAILY;UNTIL=20261231', 'schedule', null, 'Every day, until Dec 31, 2026']
	])('%s %s reads "%s"', (rule, mode, anchor, want) => {
		expect(describe(parseRule(rule), mode as RepeatMode, anchor)).toBe(want);
	});

	it('read on from other words with day and month names kept', () => {
		expect(describeAfter(parseRule('FREQ=WEEKLY;BYDAY=MO,TH;UNTIL=20261130'), 'schedule')).toBe(
			'weekly on Mon and Thu, until Nov 30, 2026'
		);
		expect(repeatLabel('FREQ=DAILY', 'schedule', null)).toBe('Repeats every day');
	});

	it('list the dates to come, for the calendar', () => {
		const weekly = parseRule('FREQ=WEEKLY;BYDAY=MO,TH');
		expect(upcoming(weekly, '2026-10-29', '2026-10-26', '2026-11-08')).toEqual([
			'2026-11-02',
			'2026-11-05'
		]);
		// From later on, only dates in the range; the due date itself is never one.
		expect(upcoming(weekly, '2026-10-29', '2026-11-06', '2026-11-12')).toEqual([
			'2026-11-09',
			'2026-11-12'
		]);
		const until = parseRule('FREQ=DAILY;UNTIL=20261102');
		expect(upcoming(until, '2026-10-30', '2026-10-01', '2026-11-30')).toEqual([
			'2026-10-31',
			'2026-11-01',
			'2026-11-02'
		]);
		expect(
			upcoming(parseRule('FREQ=DAILY'), '2026-10-01', '2026-10-01', '2026-12-31', 5)
		).toHaveLength(5);
	});

	it('start a new series on its first date from today', () => {
		const today = '2026-10-28'; // a Wednesday
		expect(firstDue(parseRule('FREQ=DAILY'), today)).toBe(today);
		expect(firstDue(parseRule('FREQ=YEARLY'), today)).toBe(today);
		expect(firstDue(parseRule('FREQ=WEEKLY;BYDAY=WE'), today)).toBe(today);
		expect(firstDue(parseRule('FREQ=WEEKLY;BYDAY=MO,TH'), today)).toBe('2026-10-29');
		expect(firstDue(parseRule('FREQ=MONTHLY;BYDAY=1MO'), today)).toBe('2026-11-02');
		expect(firstDue(parseRule('FREQ=MONTHLY;BYMONTHDAY=-1'), today)).toBe('2026-10-31');
	});

	it('start a series with an interval in the current week or month', () => {
		const monday = '2026-11-02';
		expect(firstDue(parseRule('FREQ=WEEKLY;INTERVAL=2;BYDAY=MO'), monday)).toBe(monday);
		expect(firstDue(parseRule('FREQ=WEEKLY;INTERVAL=2;BYDAY=TU'), monday)).toBe('2026-11-03');
		expect(firstDue(parseRule('FREQ=WEEKLY;INTERVAL=3;BYDAY=MO'), '2026-10-28')).toBe(monday);
		const first = '2026-11-01';
		expect(firstDue(parseRule('FREQ=MONTHLY;INTERVAL=2;BYMONTHDAY=15'), first)).toBe('2026-11-15');
		expect(firstDue(parseRule('FREQ=MONTHLY;INTERVAL=2;BYMONTHDAY=1'), first)).toBe(first);
		expect(firstDue(parseRule('FREQ=MONTHLY;INTERVAL=3;BYDAY=-1FR'), first)).toBe('2026-11-27');
		// The series then counts from that date.
		const rule = parseRule('FREQ=WEEKLY;INTERVAL=2;BYDAY=MO');
		expect(nextDue(rule, 'schedule', monday, monday)).toBe('2026-11-16');
	});
});
