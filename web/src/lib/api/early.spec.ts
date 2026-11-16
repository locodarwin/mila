import { afterEach, describe, expect, it, vi } from 'vitest';
import { early, forgetEarly } from './early';

describe('early', () => {
	afterEach(() => vi.unstubAllGlobals());
	const answer = (status: number, body: unknown) =>
		Promise.resolve(new Response(JSON.stringify(body), { status }));

	it('hands over the answer the page asked for, once, shaped for call()', async () => {
		vi.stubGlobal('window', { milaEarly: { '/workspaces': answer(200, [{ id: 'w1' }]) } });
		const got = await early('/workspaces');
		expect(got?.data).toEqual([{ id: 'w1' }]);
		expect(got?.response.status).toBe(200);
		expect(early('/workspaces')).toBeUndefined();
	});

	it('passes a refusal on as an error', async () => {
		vi.stubGlobal('window', {
			milaEarly: { '/workspaces': answer(401, { error: 'unauthorized' }) }
		});
		const got = await early('/workspaces');
		expect(got?.data).toBeUndefined();
		expect(got?.error).toEqual({ error: 'unauthorized' });
	});

	it('has nothing once forgotten, or with nothing asked', () => {
		vi.stubGlobal('window', { milaEarly: { '/auth/state': answer(200, {}) } });
		forgetEarly();
		expect(early('/auth/state')).toBeUndefined();
		vi.stubGlobal('window', {});
		expect(early('/workspaces')).toBeUndefined();
	});
});
