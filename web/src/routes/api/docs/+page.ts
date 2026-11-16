// The API reference: the OpenAPI document as the server serves it, and who
// is signed in, for trying requests. It needs no sign-in to read.
import type { Spec } from '#lib/apidocs.js';
import { session } from '#lib/session.svelte.js';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const spec = fetch('/api/v1/openapi.json')
		.then((res) => (res.ok ? (res.json() as Promise<Spec>) : null))
		.catch(() => null);
	await session.load().catch(() => {
		// Offline, or the server is down: the page says so.
	});
	if (session.user) await session.loadWorkspaces().catch(() => {});
	return { spec: await spec };
};
