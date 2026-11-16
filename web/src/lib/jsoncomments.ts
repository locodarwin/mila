// Comments an app's reader gathered as JSON while making its files one
// table (todoistimport.ts, mondayimport.ts), read back by its dialect. A
// module of its own so the readers, which load only when used, share it.
import { ANY_CSV, type ReadComment } from './csvimport';

/**
 * Comments a reader gathered as JSON (`[{author, at, body}]`, `at` an ISO
 * time), each body made by `body`; any other text as one comment.
 */
export function jsonComments(
	s: string,
	problems: string[],
	body: (text: string, problems: string[]) => string = (t) => t
): ReadComment[] {
	let list: unknown;
	try {
		list = JSON.parse(s);
	} catch {
		return ANY_CSV.comments(s, problems);
	}
	if (!Array.isArray(list)) return [];
	return list.flatMap((c: Record<string, unknown>) => {
		if (!c || typeof c !== 'object' || typeof c.body !== 'string') return [];
		const at = typeof c.at === 'string' ? new Date(c.at) : null;
		const text = body(c.body, problems);
		if (!text) return [];
		return [
			{
				author: typeof c.author === 'string' ? c.author : null,
				at: at && !Number.isNaN(at.getTime()) ? at : null,
				body: text
			}
		];
	});
}
