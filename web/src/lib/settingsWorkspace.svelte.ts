// The workspace Settings is about. Pages under /settings/w/<id> act on the
// workspace their address names, which the picker in the rail's Workspace
// heading chooses, and not on the one open in the sidebar, so changing it leaves
// the rest of the app where it was. Anywhere else this is the sidebar's.
import { page } from '$app/state';
import type { Schemas } from './api/client';
import { session } from './session.svelte';

/** The workspace in view in Settings, or null for one this person is not in. */
export function settingsWorkspace(): Schemas['Workspace'] | null {
	const id = page.params.ws;
	if (!id) return session.workspace;
	return session.workspaces.find((w) => w.id === id) ?? null;
}
