// Uploading a file to a task or a wiki page. Uploads need the network;
// the new row also arrives by sync, and is pulled at once so it shows.
import { engine } from './sync/engine.svelte';
import type { DocTarget } from './sync/docs';

/** The largest file the server takes (httpapi.MaxUpload). */
export const MAX_UPLOAD = 25 * 1024 * 1024;

/** The image types the server shows in the browser; the rest download. */
export const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/svg+xml'];

/** Where a task's or a page's file is served. */
export const fileUrl = (ws: string, attachmentId: string) =>
	`/api/v1/workspaces/${ws}/attachments/${attachmentId}/file`;

/** An upload that failed, with a message for people. */
export class UploadError extends Error {}

/** Uploads a file to a task or page and returns its attachment's ID. */
export async function uploadFile(ws: string, to: DocTarget, file: File): Promise<string> {
	if (file.size > MAX_UPLOAD) throw new UploadError('Files can be at most 25 MB.');
	const body = new FormData();
	body.append('file', file);
	const path = to.kind === 'task' ? `tasks/${to.id}` : `pages/${to.id}`;
	// The task or page may be new and not on the server yet (a wiki's main
	// page is made on its first visit): send it first, or the server answers
	// that there is no such thing.
	await engine.flush();
	let res: Response;
	try {
		res = await fetch(`/api/v1/workspaces/${ws}/${path}/attachments`, { method: 'POST', body });
	} catch {
		throw new UploadError('Adding a file needs a connection.');
	}
	const out = await res.json().catch(() => ({}));
	if (!res.ok) throw new UploadError(out.message ?? 'The upload failed.');
	await engine.pull(ws);
	return out.id as string;
}
