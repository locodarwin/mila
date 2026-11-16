import { describe, expect, it } from 'vitest';
import { score } from './search';

describe('score', () => {
	it('ranks whole, start, word start, anywhere and every word', () => {
		expect(score('Inbox', 'inbox')).toBe(100);
		expect(score('Inbox zero', 'inbox')).toBe(80);
		expect(score('Go to Inbox', 'inbox')).toBe(60);
		expect(score('Sandbox', 'box')).toBe(40);
		expect(score('Add a task to My Day', 'day task')).toBe(20);
		expect(score('Add a task', 'nothing')).toBe(0);
	});

	it('matches everything a little on an empty query, and nothing in missing text', () => {
		expect(score('Anything', '')).toBe(1);
		expect(score(null, 'a')).toBe(0);
		expect(score(undefined, 'a')).toBe(0);
	});
});
