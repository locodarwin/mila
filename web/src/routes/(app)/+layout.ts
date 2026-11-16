// Every app page needs a signed-in user. With rendering on the client
// only, this runs in the browser before the page shows, and starts the
// sync engine the first time.
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { redirect } from '@sveltejs/kit';
import { notifications } from '#lib/notifications.svelte.js';
import { preferences } from '#lib/preferences.svelte.js';
import { isWorkspaceTheme } from '#lib/theme/follow.svelte.js';
import { isPreset } from '#lib/theme/presetId.js';
import { themes } from '#lib/theme/themes.svelte.js';
import { forgetPush, refreshPush } from '#lib/push.js';
import { session } from '#lib/session.svelte.js';
import { OWNER_KEY, claimDevice, releaseHeld, sessionEnded } from '#lib/signout.js';
import { engine } from '#lib/sync/engine.svelte.js';
import { updates } from '#lib/update.svelte.js';
import type { LayoutLoad } from './$types';

let started = false;
let ending = false;

/** Whether the browser holds a live session now, whoever's. Offline, no. */
async function someoneSignedIn(): Promise<boolean> {
	try {
		const res = await fetch('/api/v1/auth/state', { cache: 'no-store' });
		return res.ok && Boolean(((await res.json()) as { user?: unknown }).user);
	} catch {
		return false;
	}
}

const startPaths: Record<string, string | undefined> = {
	today: resolve('/(app)/today'),
	upcoming: resolve('/(app)/upcoming'),
	projects: resolve('/(app)/projects')
};

export const load: LayoutLoad = async ({ url }) => {
	// Signing out in this tab (from Settings, Security, say) stops the engine,
	// so the next person, or the same one again, starts it afresh here.
	if (!started || !engine.running) {
		await session.load();
		if (session.setupRequired) redirect(307, '/setup');
		if (!session.user) {
			// Signed out while away, maybe from another device: push here
			// stops before someone else signs in.
			await forgetPush();
			redirect(307, '/login?next=' + encodeURIComponent(url.pathname + url.search));
		}
		// Someone else's copy left on this device goes before this person's
		// starts, and the page starts again from nothing: the page being gone
		// to, since the address still names the one before while a navigation
		// in the app loads (the desktop app's sign-in page, which works once).
		if (await claimDevice(session.user.id)) {
			location.replace(url.href);
			return new Promise<never>(() => {});
		}
		// Another tab signing out, or in as someone else, ends this one's
		// copy too: it starts again as whoever is signed in now.
		addEventListener('storage', (e) => {
			if (e.key === OWNER_KEY && e.newValue !== session.user?.id) location.reload();
		});
		await session.loadWorkspaces();
		engine.onSignedOut = async () => {
			// A push and a check of the session can both find out.
			if (ending) return;
			ending = true;
			// The refusal may answer a request sent before another tab signed
			// in, as someone else or the same person again: the session the
			// browser holds now is not this tab's to end. It starts again as
			// whoever that is.
			if (await someoneSignedIn()) {
				location.reload();
				return;
			}
			started = false;
			// The session is gone already; the server's answer still tells
			// the browser to empty its HTTP cache (pictures, attachments).
			await fetch('/api/v1/auth/sign-out', { method: 'POST' }).catch(() => {});
			await sessionEnded();
			await goto(resolve('/(auth)/login'), { replaceState: true });
			ending = false;
		};
		engine.onWorkspacesChanged = async () => {
			// A workspace gone or back takes its notifications with it.
			void notifications.refresh();
			await session.loadWorkspaces();
			await engine.setWorkspaces(session.workspaces.map((w) => w.id));
			// Someone joined, left or deleted their account.
			void session.refreshPeople();
		};
		notifications.restore(session.user.id);
		engine.onNotify = () => void notifications.refresh();
		// A custom theme put in use on another device comes with its theme.
		engine.onSettings = () =>
			void preferences.refresh().then(() => {
				const id = preferences.current.customTheme;
				if (!isWorkspaceTheme(id) && !isPreset(id)) themes.follow(id);
			});
		engine.onThemes = (id) => (id ? void themes.onMessage(id) : themes.reconnected());
		engine.onPeopleChanged = () => {
			void session.refreshPeople();
			// Notifications name their actor with a picture too.
			void notifications.refresh();
		};
		engine.onUpdateRequired = () => updates.offer('required');
		engine.user = session.user.id;
		engine.onOtherUser = () => location.reload();
		const workspaces = session.workspaces.map((w) => w.id);
		await releaseHeld(session.user.id, workspaces);
		await engine.start(workspaces);
		session.syncTimezone();
		void refreshPush(session.user.id);
		started = true;
		// The start page, on a bare launch only: never for a link to a task
		// or a workspace, and never once the app is open, where "/" is the
		// inbox.
		const start = startPaths[preferences.current.startPage];
		if (start && url.pathname === resolve('/(app)') && !url.search) redirect(307, start);
	} else if (!session.user) {
		started = false;
		redirect(307, '/login?next=' + encodeURIComponent(url.pathname + url.search));
	}
};
