import { describe, expect, it } from 'vitest';
import {
	countOf,
	dependenciesOf,
	fieldsOf,
	notesOf,
	progressOf,
	scheduleOf,
	timeOf
} from './taskPanel';

describe('the task panel summary lines', () => {
	it('counts', () => {
		expect(countOf(0, 'file')).toBe('None yet');
		expect(countOf(1, 'file')).toBe('1 file');
		expect(countOf(3, 'comment')).toBe('3 comments');
		expect(progressOf(0, 0)).toBe('None yet');
		expect(progressOf(1, 3)).toBe('1 of 3 done');
	});

	it('sums up a schedule', () => {
		expect(scheduleOf({ reminders: 0 })).toBe('Nothing set');
		expect(scheduleOf({ due: 'Nov 10', start: 'Nov 3', reminders: 0 })).toBe('Due Nov 10');
		expect(scheduleOf({ start: 'Nov 3', reminders: 0 })).toBe('Starts Nov 3');
		expect(scheduleOf({ repeat: 'Monthly on the second Tuesday', reminders: 0 })).toBe(
			'Monthly on the second Tuesday'
		);
		expect(scheduleOf({ due: 'Nov 10', repeat: 'Weekly', reminders: 2 })).toBe(
			'Due Nov 10 · Weekly · 2 reminders'
		);
	});

	it('sums up custom fields', () => {
		expect(fieldsOf(0, 1)).toBe('Not filled');
		expect(fieldsOf(0, 3)).toBe('3 fields, none filled');
		expect(fieldsOf(2, 3)).toBe('2 of 3 filled');
	});

	it('sums up dependencies', () => {
		expect(dependenciesOf(0, 0)).toBe('Not blocked');
		expect(dependenciesOf(1, 0)).toBe('Blocked by 1 task');
		expect(dependenciesOf(0, 2)).toBe('Blocking 2 tasks');
		expect(dependenciesOf(2, 1)).toBe('Blocked by 2 tasks · Blocking 1 task');
	});

	it('sums up time', () => {
		expect(timeOf(null, null)).toBe('No estimate');
		expect(timeOf('20m', null)).toBe('20m logged');
		expect(timeOf(null, '1h')).toBe('1h estimated');
		expect(timeOf('20m', '1h')).toBe('20m of 1h');
	});

	it.each([
		[null, 'Empty'],
		['', 'Empty'],
		['\n\n', 'Empty'],
		['Plain words', 'Plain words'],
		['\n\n## A heading', 'A heading'],
		['- [x] Buy paint', 'Buy paint'],
		['1. **First** step', 'First step'],
		['> [!NOTE] Mind this', 'Mind this'],
		['See [the plan](/wiki/plan) first', 'See the plan first'],
		['![a sketch](/attachments/x/file)', 'a sketch'],
		['| a | b |\n| - | - |\n\nAfter the table', 'After the table']
	])('takes the first line of the notes %j', (markdown, want) => {
		expect(notesOf(markdown)).toBe(want);
	});
});
