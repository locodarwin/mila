// A saved view's config as stored, which a broken app or an old REST
// write may have left in any shape. Apart from filter.ts so the replica,
// which every page loads, reads a view without the filter's rules.
import type { ViewFilter } from './sync/entities';

const isObject = (v: unknown): v is Record<string, unknown> =>
	typeof v === 'object' && v !== null && !Array.isArray(v);

/**
 * What a filter that cannot be read stands as: not an object, so it matches
 * nothing, yet safe to read a part from or spread.
 */
export const UNREADABLE = Object.freeze([]) as unknown as ViewFilter;

/**
 * The filter in a saved view's config: its "filter", or {} (every open
 * task) when the config has none or it is null. A config or a filter that
 * is not an object is UNREADABLE.
 */
export function filterOf(config: unknown): ViewFilter {
	if (config === undefined || config === null) return {};
	if (!isObject(config)) return UNREADABLE;
	const f = config.filter;
	if (f === undefined || f === null) return {};
	return isObject(f) ? (f as ViewFilter) : UNREADABLE;
}
