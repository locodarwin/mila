import { describe, expect, it } from 'vitest';
import { safeNext } from './next';

describe('safeNext', () => {
	it.each([
		[null, '/'],
		['', '/'],
		['/upcoming', '/upcoming'],
		['/settings/security?x=1', '/settings/security?x=1'],
		['//evil.example/x', '/'],
		['/\\evil.example', '/'],
		['https://evil.example', '/'],
		['javascript:alert(1)', '/']
	])('%s goes to %s', (raw, want) => {
		expect(safeNext(raw)).toBe(want);
	});
});
