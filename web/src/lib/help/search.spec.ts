import { describe, expect, it } from 'vitest';
import { searchHelp, SHORTCUTS } from './search';

describe('searchHelp', () => {
	it('finds the topic first for a word it is about', () => {
		const hits = searchHelp('repeat');
		expect(hits[0].topic).toBe('recurrence');
		expect(hits[0].section).toBeUndefined();
		expect(hits.some((h) => h.topic === 'quick-add' && h.section === 'repeats')).toBe(true);
	});

	it('finds keyboard commands, which lead to the shortcuts', () => {
		const hit = searchHelp('complete').find((h) => h.command === 'task.complete');
		expect(hit).toMatchObject({
			topic: SHORTCUTS,
			section: 'keys-list',
			title: 'Complete or reopen'
		});
	});

	it('finds a section by words from its title and keywords together', () => {
		expect(searchHelp('project tag')[0]).toMatchObject({ topic: 'quick-add', section: 'project' });
	});

	it('finds what people call things, not only the titles', () => {
		const first = (q: string) => searchHelp(q)[0];
		expect(first('group by')).toMatchObject({ topic: 'views-and-filters', section: 'display' });
		expect(first('board columns')).toMatchObject({
			topic: 'views-and-filters',
			section: 'display'
		});
		expect(first('dark mode')).toMatchObject({ topic: 'themes' });
		expect(first('density')).toMatchObject({ topic: 'themes', section: 'choosing' });
		expect(first('font size')).toMatchObject({ topic: 'themes', section: 'choosing' });
		expect(first('theme editor')).toMatchObject({ topic: 'themes' });
		expect(searchHelp('theme editor').some((h) => h.section === 'editing')).toBe(true);
		for (const q of ['change shortcut', 'remap', 'keyboard'])
			expect(first(q)).toMatchObject({ topic: SHORTCUTS, title: 'Keyboard shortcuts' });
		expect(first('remap').command).toBeUndefined();
	});

	it('finds nothing for nothing', () => {
		expect(searchHelp('')).toEqual([]);
		expect(searchHelp('   ')).toEqual([]);
		expect(searchHelp('zzzzqx')).toEqual([]);
	});

	it('keeps the order of the index on ties, and stops at 20', () => {
		const hits = searchHelp('a');
		expect(hits.length).toBeLessThanOrEqual(20);
		for (let i = 1; i < hits.length; i++) expect(hits[i - 1].s).toBeGreaterThanOrEqual(hits[i].s);
		const gestures = searchHelp('swipe').filter((h) => h.topic === 'touch-gestures');
		const order = gestures.map((h) => h.section ?? '');
		expect(order[0]).toBe('');
		expect(order.indexOf('complete')).toBeLessThan(order.indexOf('schedule'));
	});
});
