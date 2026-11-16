import { describe, expect, it } from 'vitest';
import { mainPageId, wikiMain, wikiOfMain } from './pages';

describe('a main page', () => {
	it('has the ID the server gives it', () => {
		// server/internal/syncer/pages_test.go holds the same.
		expect(mainPageId('0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b')).toBe(
			'2ecfcdbc-494f-3221-17a1-670361153603'
		);
	});

	it('says whose wiki it heads from its ID', () => {
		const ws = '0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b';
		const project = '0193a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c';
		expect(wikiOfMain(ws, wikiMain(ws, null), [project])).toBeNull();
		expect(wikiOfMain(ws, wikiMain(ws, project), [project])).toBe(project);
		expect(wikiOfMain(ws, project, [project])).toBeUndefined();
	});
});
