// Leaving a device: stop syncing, stop push here, delete this device's
// copy of every workspace and what Cache Storage holds besides the app,
// end the session. Also run when the server says the session is already
// gone.
import { APP_CACHE } from './appCache';
import { engine } from './sync/engine.svelte';
import { wipeLocal } from './sync/db';
import { dropHeld, hold, release } from './sync/held';
import { toasts } from './toasts.svelte';
import { notifications } from './notifications.svelte';
import { forgetPush } from './push';
import { session, WORKSPACE_KEY } from './session.svelte';
import { deviceOwner, setOwner } from './owner';
import { forgetRegistered } from './forgetting';

export { OWNER_KEY } from './owner';

export async function forgetDevice() {
	engine.stop();
	await forgetPush();
	await wipeLocal();
	dropHeld();
	await clearCaches();
	session.forget();
	notifications.forget();
	forgetRegistered();
	setOwner(null);
	forgetKept();
}

/**
 * Everything else Mila keeps in this browser for the person: preferences,
 * their themes, the workspace's theme, what is folded and how tables and
 * timelines are laid out. Signing out takes it too, so the next person to
 * sign in here finds none of it.
 */
function forgetKept() {
	try {
		for (const key of Object.keys(localStorage)) {
			if (key.startsWith('mila.')) localStorage.removeItem(key);
		}
	} catch {
		// Storage blocked: nothing was kept.
	}
}

/**
 * Before the app starts for this person: a copy someone else left here
 * (signed out without this device knowing, or another tab signed in as
 * someone else) goes first, as at a sign-out, unsent edits and all, so
 * nobody starts on another's workspaces or sends their edits. True if it
 * went: the page loads again, from nothing.
 */
export async function claimDevice(user: string): Promise<boolean> {
	const owner = deviceOwner();
	if (owner && owner !== user) {
		await forgetDevice();
		return true;
	}
	setOwner(user);
	return false;
}

export async function signOutHere() {
	await session.signOut();
	await forgetDevice();
}

let unsentNotice: number | undefined;

const changes = (n: number) => (n === 1 ? '1 change' : `${n} changes`);

/**
 * Before signing out: sends what it can, and if edits still have not gone,
 * warns that signing out deletes them and offers `anyway`. True if it warned.
 */
export async function warnOfUnsent(anyway: () => void): Promise<boolean> {
	await engine.flush();
	const n = engine.pending;
	if (n === 0) return false;
	if (unsentNotice !== undefined) toasts.dismiss(unsentNotice);
	unsentNotice = toasts.show(
		`${changes(n)} not sent yet. Signing out here deletes ${n === 1 ? 'it' : 'them'}.`,
		{
			tone: 'error',
			sticky: true,
			closable: true,
			action: { label: 'Sign out anyway', run: anyway }
		}
	);
	return true;
}

let endedNotice: number | undefined;

/**
 * The session ended while the app was open, from another device or by
 * age. The device's copy goes, as at a sign-out, but the person's unsent
 * edits wait for them to sign in again.
 */
export async function sessionEnded() {
	const user = session.user?.id;
	const edits = user ? await engine.unsent() : [];
	// The person is likely back soon, to the workspace they were in.
	let workspace: string | null = null;
	try {
		workspace = localStorage.getItem(WORKSPACE_KEY);
	} catch {
		// Without storage there is none to keep.
	}
	await forgetDevice();
	try {
		if (workspace) localStorage.setItem(WORKSPACE_KEY, workspace);
	} catch {
		// As above.
	}
	if (!user || edits.length === 0) return;
	const n = changes(edits.length);
	const kept = hold(user, edits);
	endedNotice = toasts.show(
		kept
			? `You were signed out. Sign in again to send ${n} not sent yet.`
			: `You were signed out, and ${n} not sent yet could not be kept.`,
		{ tone: 'error', sticky: true, closable: true }
	);
}

/** Back in: what waited for this person goes with the engine's start. */
export async function releaseHeld(user: string, workspaces: string[]) {
	const n = await release(user, workspaces);
	if (endedNotice !== undefined) toasts.dismiss(endedNotice);
	endedNotice = undefined;
	if (n) toasts.show(`Sending ${changes(n)} from before you were signed out.`);
}

/**
 * Empties Cache Storage except for the app itself, which is nobody's data
 * and keeps the sign-in page opening offline. Deleting it too would leave
 * nothing to open offline until the service worker could fetch it again.
 */
async function clearCaches() {
	try {
		if (typeof caches === 'undefined') return;
		for (const key of await caches.keys()) if (key !== APP_CACHE) await caches.delete(key);
	} catch {
		// Storage blocked: there is nothing cached to clear.
	}
}
