// Whose copy of the workspaces this device holds: the person the app last
// started for (claimDevice in signout.ts). Small, for the sign-in pages.
export const OWNER_KEY = 'mila.owner';

export function deviceOwner(): string | null {
	try {
		return localStorage.getItem(OWNER_KEY);
	} catch {
		return null;
	}
}

// When this browser last asked for a sign-in link: one it asked for signs
// in as it opens; any other asks first (the link page).
const ASKED_KEY = 'mila.asked';
const ASKED_FOR = 15 * 60 * 1000;

export function askedForLink() {
	try {
		localStorage.setItem(ASKED_KEY, String(Date.now()));
	} catch {
		// Then the link asks before it signs in.
	}
}

/** Whether this browser asked for a link lately, forgetting that it did. */
export function tookAskedLink(): boolean {
	try {
		const at = Number(localStorage.getItem(ASKED_KEY));
		localStorage.removeItem(ASKED_KEY);
		return Date.now() - at < ASKED_FOR;
	} catch {
		return false;
	}
}

export function setOwner(user: string | null) {
	try {
		if (user) localStorage.setItem(OWNER_KEY, user);
		else localStorage.removeItem(OWNER_KEY);
	} catch {
		// Without storage nothing is kept to mix up.
	}
}
