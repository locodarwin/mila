// The in-app notification list: loaded from the server, refreshed when the
// socket says something changed, and kept on the device so the last list
// still shows offline.
import { desktopNotify, isDesktop } from './desktop';
import { api, ApiError, call, type Schemas } from './api/client';

export type Notification = Schemas['Notification'];

const CACHE_KEY = 'mila.notifications';
const PAGE = 30;

interface Cached {
	userId: string;
	items: Notification[];
	unread: number;
}

class Notifications {
	items = $state<Notification[]>([]);
	unread = $state(0);
	/** The cursor for older notifications, or null at the end. */
	next = $state<string | null>(null);
	#userId = '';
	#loading: Promise<void> | null = null;
	#again = false;

	/** Starts with what this device last saw for the user. */
	restore(userId: string) {
		if (this.#userId === userId) return;
		this.#userId = userId;
		this.items = [];
		this.unread = 0;
		this.next = null;
		try {
			const c = JSON.parse(localStorage.getItem(CACHE_KEY) ?? 'null') as Cached | null;
			if (c?.userId === userId) {
				this.items = c.items;
				this.unread = c.unread;
			}
		} catch {
			// starts empty
		}
	}

	#save() {
		try {
			localStorage.setItem(
				CACHE_KEY,
				JSON.stringify({ userId: this.#userId, items: this.items, unread: this.unread })
			);
		} catch {
			// kept for this visit only
		}
	}

	// In the desktop app, what is new since the last look also shows on the desktop.
	// The first look is only a starting point, or an old list would all pop up.
	#told: Set<string> | null = null;
	#tellDesktop(items: Notification[]) {
		if (!isDesktop()) return;
		const told = this.#told;
		this.#told = new Set([...items.map((n) => n.id), ...(told ?? [])]);
		if (!told) return;
		for (const n of items) if (!n.readAt && !told.has(n.id)) desktopNotify(n.title, n.body);
	}

	/** Loads the newest page. Calls made while one is loading run once more after it. */
	refresh(): Promise<void> {
		if (this.#loading) {
			this.#again = true;
			return this.#loading;
		}
		this.#loading = (async () => {
			try {
				do {
					this.#again = false;
					const page = await call(
						api.GET('/me/notifications', { params: { query: { limit: PAGE } } })
					);
					this.#tellDesktop(page.items);
					this.items = page.items;
					this.unread = page.unread;
					this.next = page.next ?? null;
					this.#save();
				} while (this.#again);
			} catch {
				// offline: the last list stays
			} finally {
				this.#loading = null;
			}
		})();
		return this.#loading;
	}

	/** Loads the next older page. */
	async more() {
		if (!this.next) return;
		const page = await call(
			api.GET('/me/notifications', { params: { query: { limit: PAGE, before: this.next } } })
		);
		const have = new Set(this.items.map((n) => n.id));
		this.items = [...this.items, ...page.items.filter((n) => !have.has(n.id))];
		this.unread = page.unread;
		this.next = page.next ?? null;
	}

	/** Marks one read, at once here and then on the server. */
	async read(id: string) {
		const n = this.items.find((x) => x.id === id);
		if (!n || n.readAt) return;
		n.readAt = new Date().toISOString();
		this.unread = Math.max(0, this.unread - 1);
		this.#save();
		try {
			await call(
				api.POST('/me/notifications/{notificationId}/read', {
					params: { path: { notificationId: id } }
				})
			);
		} catch {
			void this.refresh();
		}
	}

	/**
	 * Marks read one opened from outside the app (a push, an email), which
	 * this device may not have listed yet.
	 */
	async readOpened(id: string) {
		if (this.items.some((x) => x.id === id)) return this.read(id);
		try {
			await call(
				api.POST('/me/notifications/{notificationId}/read', {
					params: { path: { notificationId: id } }
				})
			);
		} catch {
			return; // gone, or not theirs: nothing to mark
		}
		void this.refresh();
	}

	/**
	 * Marks read everything up to the newest one shown, so one that reached
	 * the server meanwhile stays unread.
	 */
	async readAll() {
		const newest = this.items[0];
		if (!newest) return;
		const now = new Date().toISOString();
		for (const n of this.items) n.readAt ??= now;
		this.unread = 0;
		this.#save();
		try {
			await call(api.POST('/me/notifications/read-all', { body: { through: newest.id } }));
		} catch {
			void this.refresh();
		}
	}

	/** Takes one off the list for good, at once here and then on the server. */
	async clear(id: string) {
		const i = this.items.findIndex((x) => x.id === id);
		if (i < 0) return;
		const before = { items: this.items, unread: this.unread, next: this.next };
		const wasUnread = !this.items[i].readAt;
		this.items = this.items.filter((x) => x.id !== id);
		if (wasUnread) this.unread = Math.max(0, this.unread - 1);
		// The cursor for older ones names the last one listed, which this may be.
		if (this.next === id) this.next = this.items.at(-1)?.id ?? null;
		this.#save();
		try {
			await call(
				api.DELETE('/me/notifications/{notificationId}', {
					params: { path: { notificationId: id } }
				})
			);
		} catch (err) {
			Object.assign(this, before);
			this.#save();
			throw err;
		}
	}

	/**
	 * Clears everything up to the newest one shown, older pages included, so one that
	 * reached the server meanwhile stays.
	 */
	async clearAll() {
		const newest = this.items[0];
		if (!newest) return;
		const before = { items: this.items, unread: this.unread, next: this.next };
		this.items = [];
		this.unread = 0;
		this.next = null;
		this.#save();
		try {
			await call(api.POST('/me/notifications/clear', { body: { through: newest.id } }));
		} catch (err) {
			// Another device cleared that one first: this one's list was out of date.
			if (err instanceof ApiError && err.status === 404) return void this.refresh();
			Object.assign(this, before);
			this.#save();
			throw err;
		}
	}

	/** Forgets the list on this device (signing out). */
	forget() {
		this.#userId = '';
		this.items = [];
		this.unread = 0;
		this.next = null;
		try {
			localStorage.removeItem(CACHE_KEY);
		} catch {
			// nothing kept
		}
	}
}

export const notifications = new Notifications();
