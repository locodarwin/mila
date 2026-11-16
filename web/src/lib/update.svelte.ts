// Telling the person a new version of Mila is out, and reloading into it.
//
// Two things say so. The service worker takes over as soon as a new build
// is installed (it skips waiting), and the old page may then fail to load
// a lazily loaded part, so either of those offers a reload. And the server
// refuses a push from an app that syncs another version of the data
// (update_required): the sync engine stops sending and keeps every edit,
// and the toast asks for the reload that sends them. Both stay on screen;
// only the first can be closed, since the second means edits are waiting.
import { toasts } from './toasts.svelte';

export type UpdateReason = 'deployed' | 'required';

export const UPDATE_MESSAGES: Record<UpdateReason, string> = {
	deployed: 'A new version of Mila is ready.',
	required: 'Mila has been updated. Reload to keep syncing; your changes are kept until then.'
};

/** How often an open app asks whether a new build is out. */
export const CHECK_MS = 30 * 60_000;

export class Updates {
	reason = $state<UpdateReason | null>(null);
	#toast = 0;
	/** What reloading does; tests replace it. */
	reload: () => void = () => location.reload();

	/**
	 * Offers the reload, once: a second offer for the same reason changes
	 * nothing, and one saying sync has stopped replaces one that only says
	 * a new version is ready.
	 */
	offer(reason: UpdateReason) {
		if (this.reason === reason || this.reason === 'required') return;
		if (this.#toast) toasts.dismiss(this.#toast);
		this.reason = reason;
		this.#toast = toasts.show(UPDATE_MESSAGES[reason], {
			sticky: true,
			// Edits are waiting once sync has stopped: no closing that one.
			closable: reason === 'deployed',
			action: { label: 'Reload', run: () => this.reload() }
		});
	}
}

export const updates = new Updates();

/** What watchUpdates needs from the browser, so tests can stand in for it. */
export interface UpdateEnv {
	serviceWorker?: EventTarget & {
		controller: unknown;
		getRegistration(): Promise<{ update(): Promise<unknown> } | undefined>;
	};
	window: EventTarget;
	every(fn: () => void, ms: number): () => void;
}

function browserEnv(): UpdateEnv {
	return {
		serviceWorker: 'serviceWorker' in navigator ? navigator.serviceWorker : undefined,
		window,
		every: (fn, ms) => {
			const id = setInterval(fn, ms);
			return () => clearInterval(id);
		}
	};
}

/**
 * Listens for a new build: a new service worker taking over a page it
 * already controlled, or a part of the app that no longer loads. It asks
 * for a new service worker every half hour and when the connection comes
 * back. Returns what stops it.
 */
export function watchUpdates(env: UpdateEnv = browserEnv(), into: Updates = updates): () => void {
	const sw = env.serviceWorker;
	// The first service worker taking over a page is not an update.
	let controlled = !!sw?.controller;
	const onController = () => {
		if (controlled) into.offer('deployed');
		controlled = true;
	};
	const onChunk = () => into.offer('deployed');
	const check = () => {
		void sw
			?.getRegistration()
			.then((r) => r?.update())
			.catch(() => {});
	};
	sw?.addEventListener('controllerchange', onController);
	env.window.addEventListener('vite:preloadError', onChunk);
	env.window.addEventListener('online', check);
	const stop = env.every(check, CHECK_MS);
	return () => {
		sw?.removeEventListener('controllerchange', onController);
		env.window.removeEventListener('vite:preloadError', onChunk);
		env.window.removeEventListener('online', check);
		stop();
	};
}
