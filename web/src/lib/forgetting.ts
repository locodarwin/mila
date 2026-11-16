// What signing out forgets besides what signout.ts names: state held in
// memory by parts of the app loaded only when used (a page's filters,
// Reports' setup), which register here as they load, so the next person
// to sign in on the same tab finds none of it, and a part never loaded
// costs the first page nothing.

const forgetters = new Set<() => void>();

/** Has `forget` run when the person signs out. */
export function onSignOut(forget: () => void): void {
	forgetters.add(forget);
}

/** Runs each, for forgetDevice. */
export function forgetRegistered(): void {
	for (const forget of forgetters) forget();
}
