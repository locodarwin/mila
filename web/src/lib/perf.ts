// User-timing marks for the performance budgets.
// The browser budget tests (web/e2e/budgets) and Lighthouse CI read them:
// "interactive" is measured as mila:ready, since Lighthouse no longer
// reports a time to interactive of its own.

/**
 * Marks the first page this load showed as ready to use: once, after the
 * frame holding it has painted. App pages render only once their
 * workspace is loaded from the device or, on a first visit, synced.
 */
export function markReady() {
	if (typeof performance === 'undefined' || performance.getEntriesByName('mila:ready').length)
		return;
	requestAnimationFrame(() => setTimeout(() => performance.mark('mila:ready')));
}

/** Marks a workspace's first sync (its bootstrap) as written to the device. */
export function markSynced(ws: string, rows: number) {
	if (typeof performance === 'undefined') return;
	performance.mark('mila:synced', { detail: { ws, rows } });
}
