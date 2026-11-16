// Answers the page asked for before the app's scripts arrived: on the
// app's own pages (not the sign-in ones) the head script in app.html
// starts GET /auth/state and GET /workspaces as the page loads, so they
// cross the network while the scripts do rather than after them (the
// first-load budget, docs/PLAN.md section 13). Each is taken once, by the
// first load that wants it, and all are dropped once anyone signs in or
// the answer says nobody is.
import type { paths } from './schema';

type Early = '/auth/state' | '/workspaces';
type Answer<P extends Early> = paths[P]['get']['responses'][200]['content']['application/json'];

declare global {
	interface Window {
		milaEarly?: Partial<Record<Early, Promise<Response>>>;
	}
}

/**
 * The early answer for a path, shaped as openapi-fetch's, for `call()`; or
 * undefined if there is none (already taken, or not asked for).
 */
export function early<P extends Early>(
	path: P
): Promise<{ data?: Answer<P>; error?: unknown; response: Response }> | undefined {
	if (typeof window === 'undefined') return undefined;
	const answer = window.milaEarly?.[path];
	if (!answer) return undefined;
	delete window.milaEarly![path];
	return answer.then(async (response) => {
		const body = await response.json().catch(() => undefined);
		return response.ok ? { data: body as Answer<P>, response } : { error: body, response };
	});
}

/** Drops the early answers not yet taken: asked for by someone signed out, they are stale once anyone signs in. */
export function forgetEarly() {
	if (typeof window !== 'undefined') delete window.milaEarly;
}
