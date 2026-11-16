// What the notes editor needs to know before it loads Yjs (docs.ts), which
// it loads only when a task's notes open.
import type { Row } from './replica.svelte';

/** The fragment y-tiptap keeps a document under. */
export const FRAGMENT = 'default';

/**
 * Whether a task's notes can be edited live here: they can once this
 * device holds the task's document, or when there are no notes to lose.
 * Notes with no document yet are from before live documents; the server
 * makes their document, and until it arrives they are shown as they are,
 * since a document made from them here and on another device would merge
 * into two copies.
 */
export function editable(notes: string, rows: Row[]): boolean {
	return rows.length > 0 || notes.trim() === '';
}
