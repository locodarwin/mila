// A help topic: its entry in the index and its text, which loads as a
// part of its own. A slug that names no topic gets a page that says so,
// inside the app, rather than an error page without it.
import { loadTopic, topic } from '#lib/help/index.js';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => {
	const t = topic(params.topic);
	const body = t ? await loadTopic(t.slug) : undefined;
	return { topic: body ? t! : null, body: body ?? null };
};
