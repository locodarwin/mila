// The desktop app's few features, for the pages it shows.
// In a browser none of this exists and every call does nothing. The app allows
// these named commands to Mila's own server and to nothing else.

interface Internals {
	invoke(command: string, args?: Record<string, unknown>): Promise<unknown>;
}

const internals = (): Internals | undefined =>
	typeof window === 'undefined'
		? undefined
		: (window as unknown as { __TAURI_INTERNALS__?: Internals }).__TAURI_INTERNALS__;

/** Whether this page is showing in the desktop app. */
export const isDesktop = () => !!internals();

function call(command: string, args?: Record<string, unknown>): Promise<unknown> {
	const tauri = internals();
	return tauri ? tauri.invoke(command, args) : Promise.resolve(undefined);
}

/** The code and secret of a sign-in the app is finishing, if it is, for this server. */
export async function takeDesktopSignIn(): Promise<{ code: string; verifier: string } | null> {
	return (
		((await call('desktop_take_sign_in')) as { code: string; verifier: string } | null) ?? null
	);
}

/**
 * Sends a link meant for a new tab through this window's own address: the app has one
 * window, and passes anything from elsewhere to the person's browser, while a file
 * downloads without leaving the page. Linux's webview would do nothing with such a
 * link. Undone by the function it returns.
 */
export function keepLinksInWindow(): () => void {
	if (!isDesktop()) return () => {};
	const click = (e: MouseEvent) => {
		const link = (e.target as Element | null)?.closest?.<HTMLAnchorElement>('a[target="_blank"]');
		if (!link || e.defaultPrevented || e.button !== 0) return;
		e.preventDefault();
		location.assign(link.href);
	};
	const open = window.open;
	window.open = (url?: string | URL) => {
		if (url) location.assign(url);
		return null;
	};
	document.addEventListener('click', click);
	return () => {
		window.open = open;
		document.removeEventListener('click', click);
	};
}

/**
 * In the app, right-click opens Mila's own menus and nothing else: the webview's
 * (Back, Reload, Inspect) would act on the app itself. Text keeps it, for cut,
 * copy, paste and spelling: fields, the editor, and text selected under the
 * pointer. Mila's menus stop the event before it gets here. Undone by the
 * function it returns.
 */
export function onlyMilaMenus(): () => void {
	if (!isDesktop()) return () => {};
	const contextmenu = (e: MouseEvent) => {
		if (!keepsBrowserMenu(e.target)) e.preventDefault();
	};
	document.addEventListener('contextmenu', contextmenu);
	return () => document.removeEventListener('contextmenu', contextmenu);
}

/** Whether a right-click here is on text, which keeps the browser's menu. */
export function keepsBrowserMenu(target: EventTarget | null): boolean {
	const el = target instanceof Element ? target : null;
	if (!el) return false;
	if (el.closest('input, textarea, [contenteditable="true"], [contenteditable=""]')) return true;
	const selection = document.getSelection();
	return !!selection && !selection.isCollapsed && selection.containsNode(el, true);
}

/** Shows a desktop notification. It never matters if one can't be shown. */
export function desktopNotify(title: string, body: string) {
	call('desktop_notify', { title, body }).catch(() => {});
}

/** Sets the shortcut that opens Quick Add from anywhere; empty for none. */
export function setDesktopShortcut(binding: string): Promise<unknown> {
	return call('desktop_set_shortcut', { binding });
}
