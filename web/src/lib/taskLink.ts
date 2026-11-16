// The open task lives in the URL (?task=<id>), so back closes it and a link
// opens it.
import { goto } from '$app/navigation';
import { page } from '$app/state';

export function openTask(id: string | null) {
	const u = new URL(page.url.href);
	if (id) u.searchParams.set('task', id);
	else u.searchParams.delete('task');
	// eslint-disable-next-line svelte/no-navigation-without-resolve -- same page, new query
	return goto(u, { reset: false });
}
