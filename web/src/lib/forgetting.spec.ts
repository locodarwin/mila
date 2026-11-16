import { describe, expect, it } from 'vitest';
import { forgetRegistered } from './forgetting';
import { listViews } from './listViews.svelte';
import { freshReportSetup, reportSetups } from './reportSetup.svelte';

describe('signing out', () => {
	it("forgets the filters and setups held in memory, so the next person doesn't find them", () => {
		reportSetups.set('ws-1', { ...freshReportSetup(), filter: { text: 'layoffs' } });
		listViews.set('ws-1', 'inbox', {
			...listViews.get('ws-1', 'inbox'),
			filter: { text: 'layoffs' }
		});
		expect(reportSetups.get('ws-1').filter.text).toBe('layoffs');
		forgetRegistered();
		expect(reportSetups.get('ws-1').filter.text).toBeUndefined();
		expect(listViews.get('ws-1', 'inbox').filter.text).toBeUndefined();
	});
});
