// How well a piece of text matches what someone typed, for the command
// palette, help search and the shortcut reference.

/**
 * How well text matches the query: whole, start, word start, anywhere,
 * then every word of the query somewhere. The query is lowercase and
 * trimmed. A row synced in part (only the fields another device sent)
 * may lack the text; it simply does not match. An empty query matches
 * everything a little.
 */
export function score(text: string | null | undefined, q: string): number {
	const t = (text ?? '').toLowerCase();
	if (!q) return 1;
	if (t === q) return 100;
	if (t.startsWith(q)) return 80;
	if (t.includes(' ' + q)) return 60;
	if (t.includes(q)) return 40;
	// Every word of the query somewhere.
	const parts = q.split(/\s+/).filter(Boolean);
	return parts.length > 1 && parts.every((p) => t.includes(p)) ? 20 : 0;
}
