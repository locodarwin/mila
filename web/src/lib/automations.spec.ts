import { describe, expect, it } from 'vitest';
import {
	CHANGES,
	changeWords,
	fits,
	KIND,
	MAX_CHANGES,
	readRule,
	ready,
	sentenceOf,
	STARTS,
	whenWords,
	WHO_MADE_TASK,
	type Names,
	type Rule
} from './automations';
import type { FilterContext } from './filter';
import type { Task } from './sync/entities';

const names: Names = {
	status: (id) => ({ s1: 'Done' })[id],
	label: (id) => ({ l1: 'Urgent' })[id],
	field: (id) => ({ f1: 'Order size' })[id],
	person: (id) => ({ p1: 'Dana' })[id],
	project: (id) => ({ pr1: 'Wholesale' })[id]
};

const rule = (over: Partial<Rule> = {}): Rule => ({
	when: { on: 'created' },
	then: [{ do: 'priority', value: 'high' }],
	...over
});

describe('reading a rule', () => {
	it('takes what the app knows and refuses the rest', () => {
		expect(readRule(rule())).not.toBeNull();
		for (const bad of [
			null,
			[],
			'x',
			{ when: { on: 'explode' }, then: [] },
			{ when: { on: 'created' } },
			{ when: { on: 'created' }, then: [{ do: 'delete' }] }
		]) {
			expect(readRule(bad)).toBeNull();
		}
	});
});

describe('saying a rule', () => {
	it('reads as a sentence, with where it works and how many conditions', () => {
		const r = rule({
			when: { on: 'status', value: 's1' },
			then: [
				{ do: 'assign', value: 'p1' },
				{ do: 'notify', value: WHO_MADE_TASK }
			]
		});
		expect(sentenceOf(r, 'pr1', names, 2)).toBe(
			'When a task moves to Done in Wholesale, if it fits 2 conditions: assign Dana and notify whoever made the task.'
		);
		expect(sentenceOf(rule(), null, names)).toBe(
			'When a task is created: set the priority to High.'
		);
	});

	it('says when a name is no longer there', () => {
		expect(whenWords(rule({ when: { on: 'label', value: 'gone' } }), names)).toContain('no longer');
		expect(changeWords({ do: 'assign', value: 'gone' }, names)).toContain('no longer');
		expect(changeWords({ do: 'status', value: KIND + 'done' }, names)).toBe(
			'set the status to a done status'
		);
	});

	it('has words for everything that can start a rule or be done', () => {
		for (const s of STARTS) {
			expect(whenWords(rule({ when: { on: s.on, value: 'x' } }), names).length).toBeGreaterThan(5);
		}
		for (const c of CHANGES) {
			expect(changeWords({ do: c.do, value: 'x' }, names).length).toBeGreaterThan(5);
		}
	});
});

describe('a rule ready to save', () => {
	it('needs something to do, and what each part asks for', () => {
		expect(ready(rule())).toBe(true);
		expect(ready(rule({ then: [] }))).toBe(false);
		expect(ready(rule({ then: [{ do: 'comment', value: '  ' }] }))).toBe(false);
		expect(ready(rule({ when: { on: 'status' } }))).toBe(false);
		// Anyone at all is an answer.
		expect(ready(rule({ when: { on: 'assigned', value: '' } }))).toBe(true);
		expect(ready(rule({ then: Array(MAX_CHANGES + 1).fill({ do: 'complete' }) }))).toBe(false);
		expect(ready(rule({ then: [{ do: 'complete' }] }))).toBe(true);
	});
});

describe('testing a rule on a task', () => {
	const task = {
		id: 't',
		priority: 'high',
		projectId: null,
		completedAt: 'now'
	} as unknown as Task;
	const ctx = {
		me: 'p1',
		today: '2026-11-05',
		category: () => 'done',
		labels: () => [],
		assignees: () => []
	} as unknown as FilterContext;

	it('counts a finished task, as the server does', () => {
		expect(fits(rule(), task, ctx)).toBe(true);
	});

	it('uses a saved view filter', () => {
		expect(fits(rule({ config: { filter: { priorities: ['high'] } } }), task, ctx)).toBe(true);
		expect(fits(rule({ config: { filter: { priorities: ['low'] } } }), task, ctx)).toBe(false);
	});
});
