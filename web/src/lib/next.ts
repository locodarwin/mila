// Where to go after signing in. Only same-site paths are accepted, so a
// crafted ?next= cannot send someone to another site.
export function safeNext(raw: string | null): string {
	if (!raw || !raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return '/';
	return raw;
}

const REMEMBERED = 'mila.next';
const LINK_LIFETIME = 15 * 60_000;

/**
 * Remembers where to go once an email link signs this browser in, for as
 * long as the link works: the link opens on its own page, without ?next=.
 */
export function rememberNext(path: string) {
	try {
		localStorage.setItem(REMEMBERED, JSON.stringify({ path, at: Date.now() }));
	} catch {
		// the link then leads to the inbox
	}
}

/** Where an email link's sign-in goes: the page remembered for it, once, else "/". */
export function takeNext(): string {
	try {
		const saved = JSON.parse(localStorage.getItem(REMEMBERED) ?? 'null');
		localStorage.removeItem(REMEMBERED);
		if (saved && Date.now() - saved.at < LINK_LIFETIME) return safeNext(saved.path);
	} catch {
		// nothing usable remembered
	}
	return '/';
}
