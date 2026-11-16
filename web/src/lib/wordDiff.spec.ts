import { describe, expect, it } from 'vitest';
import { wordDiff } from './wordDiff';

const show = (before: string, after: string) =>
	wordDiff(before, after)
		.map((c) =>
			c.kind === 'same' ? c.text : c.kind === 'added' ? `{+${c.text}+}` : `[-${c.text}-]`
		)
		.join('');

describe('wordDiff', () => {
	it('marks the words added and removed', () => {
		expect(show('Paint the hall white', 'Paint the hall sage green')).toBe(
			'Paint the hall [-white-]{+sage green+}'
		);
		expect(show('one two three', 'one three')).toBe('one [-two -]three');
		expect(show('one three', 'one two three')).toBe('one {+two +}three');
		expect(show('Beds: tomatoes.', 'Beds: tomatoes along the fence.')).toBe(
			'Beds: tomatoes{+ along the fence+}.'
		);
	});

	it('gives each text back from its side', () => {
		const before = '## Plan\n\n- buy paint\n- tape the edges';
		const after = '## Plan\n\n- buy primer and paint\n- tape the edges\n- two coats';
		const d = wordDiff(before, after);
		expect(
			d
				.filter((c) => c.kind !== 'added')
				.map((c) => c.text)
				.join('')
		).toBe(before);
		expect(
			d
				.filter((c) => c.kind !== 'removed')
				.map((c) => c.text)
				.join('')
		).toBe(after);
	});

	it('handles nothing before, nothing after, and no change', () => {
		expect(show('', 'New page')).toBe('{+New page+}');
		expect(show('Old page', '')).toBe('[-Old page-]');
		expect(wordDiff('Same', 'Same')).toEqual([{ kind: 'same', text: 'Same' }]);
	});
});
