// What the service worker does with a web push: the notification it shows,
// and where a click on it leads. Kept free of browser globals so the
// service worker and the tests share it.

/** A push as the server sends it. */
export interface PushPayload {
	title: string;
	body: string;
	/** A path in the app, such as "/?task=<id>&ws=<id>". */
	url: string;
	/** The task it is about; a newer push with the same tag replaces it. */
	tag: string;
}

const PLACEHOLDER = 'https://mila.invalid';

/**
 * A path inside the app, or "/" for anything that would lead elsewhere:
 * a push is only ever about Mila.
 */
export function appPath(url: unknown): string {
	if (typeof url !== 'string' || url === '') return '/';
	try {
		const u = new URL(url, PLACEHOLDER);
		if (u.origin !== PLACEHOLDER) return '/';
		return u.pathname + u.search + u.hash;
	} catch {
		return '/';
	}
}

/** Reads a push's JSON, putting up with anything malformed. */
export function readPush(data: unknown): PushPayload {
	const o = typeof data === 'object' && data !== null ? (data as Record<string, unknown>) : {};
	const text = (v: unknown) => (typeof v === 'string' ? v : '');
	return {
		title: text(o.title) || 'Mila',
		body: text(o.body),
		url: appPath(o.url),
		tag: text(o.tag)
	};
}

/** The options showNotification takes, including renotify, which TypeScript leaves out. */
export interface NoticeOptions {
	body: string;
	icon: string;
	badge: string;
	tag?: string;
	renotify?: boolean;
	data: { url: string };
}

/**
 * The notification for a push. One per task: a newer push about the same
 * task replaces the old one and still alerts.
 */
export function noticeOf(p: PushPayload): { title: string; options: NoticeOptions } {
	const options: NoticeOptions = {
		body: p.body,
		icon: '/icon-192.png',
		badge: '/badge-96.png',
		data: { url: p.url }
	};
	// renotify without a tag is an error.
	if (p.tag) {
		options.tag = p.tag;
		options.renotify = true;
	}
	return { title: p.title, options };
}

/** A Mila window, as the service worker sees it. */
export interface AppWindow {
	url: string;
	focused?: boolean;
	visibilityState?: string;
	focus(): Promise<unknown>;
	postMessage(message: unknown): void;
}

/** What openApp needs from the service worker's clients. */
export interface AppWindows {
	list(): Promise<readonly AppWindow[]>;
	open(url: string): Promise<unknown>;
}

/** The message a window gets to go to a path; the app layout acts on it. */
export interface OpenMessage {
	type: 'open';
	url: string;
}

/**
 * Brings Mila forward at path: the focused window, else a visible one,
 * else any, is told to go there and focused. With no window open, a new
 * one opens at path.
 */
export async function openApp(windows: AppWindows, origin: string, path: string): Promise<void> {
	const open = (await windows.list()).filter((w) => {
		try {
			return new URL(w.url).origin === origin;
		} catch {
			return false;
		}
	});
	const target =
		open.find((w) => w.focused) ?? open.find((w) => w.visibilityState === 'visible') ?? open[0];
	if (!target) {
		await windows.open(path);
		return;
	}
	target.postMessage({ type: 'open', url: path } satisfies OpenMessage);
	await target.focus();
}

/** Whether a window message asks the app to go somewhere. */
export function isOpenMessage(data: unknown): data is OpenMessage {
	return (
		typeof data === 'object' &&
		data !== null &&
		(data as OpenMessage).type === 'open' &&
		typeof (data as OpenMessage).url === 'string'
	);
}
