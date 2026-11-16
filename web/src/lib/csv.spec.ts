import { describe, expect, it } from 'vitest';
import vectors from '../../../shared/csv-vectors.json';
import { writeCsv } from './csv';

describe('writing CSV', () => {
	it('keeps a formula from running in a ; locale and after spaces', () => {
		expect(writeCsv([['x;=1+2', ' =1+2', '=1', 'tab\there', 'plain', -3]])).toBe(
			`"x;=1+2",' =1+2,'=1,"tab\there",plain,-3\r\n`
		);
	});

	it('writes each field as the server export does (shared/csv-vectors.json)', () => {
		expect(vectors.fields.length).toBeGreaterThan(10);
		for (const [text, want] of vectors.fields) {
			expect(writeCsv([[text]]), JSON.stringify(text)).toBe(`${want}\r\n`);
		}
	});
});
