// Writing a saved view's display: replacing some of its parts and keeping
// the rest. Apart from viewDisplay.ts, so data.ts, which every page loads,
// does not bring the sorting and grouping with it.
import type { ViewDisplay } from './sync/entities';

/** The parts of a display a write can set; undefined takes one away. */
export type DisplayPath =
	| 'sort'
	| 'group'
	| 'columns'
	| 'table'
	| 'timeline'
	| 'report'
	| 'fields.list'
	| 'fields.board'
	| 'fields.table';

/**
 * A display with some parts replaced, keeping everything else (keys from a
 * newer app among them). Worked out from the row as it stands when the
 * write runs, so edits made elsewhere survive.
 */
export function applyPaths(
	display: ViewDisplay | null | undefined,
	patch: Partial<Record<DisplayPath, unknown>>
): ViewDisplay {
	const out: ViewDisplay = { ...(display ?? {}) };
	for (const [path, value] of Object.entries(patch)) {
		const [head, sub] = path.split('.');
		if (!sub) {
			if (value === undefined) delete out[head];
			else out[head] = value;
			continue;
		}
		const inner: Record<string, unknown> =
			typeof out[head] === 'object' && out[head] !== null ? { ...(out[head] as object) } : {};
		if (value === undefined) delete inner[sub];
		else inner[sub] = value;
		if (Object.keys(inner).length) out[head] = inner;
		else delete out[head];
	}
	return out;
}
