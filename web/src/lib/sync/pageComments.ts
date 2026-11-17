// Comments pinned to text in a wiki page: what the panel
// beside the page may do. Apart from data.ts, so first loads do not carry it.
import { uuid } from '../uuid';
import { engine } from './engine.svelte';

const now = () => new Date().toISOString();

/** A new thread on some words, with its first comment, undone as one. */
export function startThread(
	ws: string,
	pageId: string,
	me: string,
	pin: { anchor: string; quote: string },
	body: string
) {
	const thread = uuid();
	return engine
		.group(async () => {
			await engine.mutate(ws, 'page_thread', thread, {
				page_id: pageId,
				created_by: me,
				quote: pin.quote,
				anchor: pin.anchor
			});
			await engine.mutate(ws, 'page_comment', uuid(), {
				page_id: pageId,
				thread_id: thread,
				author_id: me,
				body: body.trim()
			});
		})
		.then(() => thread);
}

export function replyToThread(
	ws: string,
	pageId: string,
	thread: string,
	me: string,
	body: string
) {
	return engine.mutate(ws, 'page_comment', uuid(), {
		page_id: pageId,
		thread_id: thread,
		author_id: me,
		body: body.trim()
	});
}

export function editPageComment(ws: string, id: string, body: string) {
	return engine.mutate(ws, 'page_comment', id, { body: body.trim(), edited_at: now() });
}

export function deletePageComment(ws: string, id: string) {
	return engine.mutate(ws, 'page_comment', id, { deleted_at: now() });
}

export function resolveThread(ws: string, thread: string) {
	return engine.mutate(ws, 'page_thread', thread, { resolved_at: now() });
}

export function reopenThread(ws: string, thread: string) {
	return engine.mutate(ws, 'page_thread', thread, { resolved_at: null });
}
