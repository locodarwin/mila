// The theme tokens: every value a custom theme may change, read from
// shared/theme-tokens.json (which the server's accounts/themetokens.go
// keeps in step with). tokens.css declares each one; components use
// these and the INTERNAL properties below, never anything else
// (tokens.spec.ts checks).
//
// This module carries the labels too, so the app loads it only when it
// needs it: building a custom theme's CSS, or the theme editor.
import registry from '../../../../shared/theme-tokens.json' with { type: 'json' };
import { HIGHLIGHTS, PALETTE, PRIORITIES, STATUS_CATEGORIES } from './palette';

export type TokenType = 'color' | 'px' | 'ms' | 'number' | 'font' | 'ease' | 'shadow' | 'hue';

export interface TokenMeta {
	name: string;
	group: string;
	type: TokenType;
	/** Set apart for light and dark (a theme's light and dark maps), or once for both (tokens). */
	perScheme: boolean;
	/**
	 * The layer: 0 primitives, 1 core semantic, 2 derived semantic, 3
	 * component. A ref points only to a lower rank, so refs never loop.
	 */
	rank: 0 | 1 | 2 | 3;
	/** Colors only: whether the color may be see-through. */
	alpha?: boolean;
	label: string;
	used: string;
	/** The token tokens.css sets this one from, where it is that simple. */
	default?: { ref: string };
	min?: number;
	max?: number;
	step?: number;
}

export interface Group {
	id: string;
	label: string;
}

export const TOKENS: readonly TokenMeta[] = registry.tokens as TokenMeta[];
export const GROUPS: readonly Group[] = registry.groups;

/** Old token names, mapped to their new ones, for themes saved before a rename. */
export const RENAMED: Readonly<Record<string, string>> = registry.renamed;

const index = new Map(TOKENS.map((t) => [t.name, t]));

/** A token by name, after any rename; undefined for a name Mila does not know. */
export function byName(name: string): TokenMeta | undefined {
	return index.get(Object.hasOwn(RENAMED, name) ? RENAMED[name] : name);
}

/**
 * Custom properties components may use that are not theme tokens: layout
 * and runtime values (widths, density and font size arithmetic, the
 * accent apply() works out), the sizes built from the type steps, and the
 * focus ring built from its parts. A theme never sets these.
 */
export const INTERNAL: ReadonlySet<string> = new Set([
	'sidebar-width',
	// The sidebar's width in the frame index.html paints (src/app.html).
	'frame-sidebar',
	'detail-width',
	// A board's columns, all one width, kept on the device (Board.svelte).
	'column-width',
	// A color slider's track, the color's own range (ColorField.svelte).
	'track',
	// The selection bar's height, which toasts rise above (SelectionBar.svelte).
	'selection-bar',
	// What the app layout puts over a page (the phone's bar, a theme notice),
	// which a page one screen tall leaves room for (routes/(app)/+layout.svelte).
	'above',
	// The room under the workspace switcher, which its list grows into
	// (WorkspaceSwitcher.svelte).
	'room',
	'icon-size',
	'project-color',
	'chip',
	'chip-soft',
	// Someone else's cursor and selection in a live document (DocEditor).
	'peer',
	// A page's own font and size, which its pageStyle block sets (DocEditor).
	'doc-font',
	'doc-heading-font',
	'doc-size',
	// How deep a page sits in the wiki's tree (WikiTree).
	'depth',
	// A note, tip or warning box's color and its tint, from the palette (DocEditor).
	'box',
	'box-soft',
	'demo-left',
	'density',
	'ui-scale',
	'control-scale',
	'row-height',
	'pad-y',
	'pad-x',
	'motion',
	'tap-min',
	'touch-target',
	'tabbar-height',
	'text-base-px',
	'text-xs',
	'text-sm',
	'text-md',
	'text-lg',
	'text-xl',
	'text-2xl',
	'text-3xl',
	'dur-fade',
	'focus-ring',
	'accent-hue',
	'accent-l',
	'accent-c',
	'accent-text-l',
	'accent-text-c',
	'accent-text-shift'
]);

/**
 * Names built in markup or code from a list: `var(--palette-${color})`
 * and the like. Each family's prefix and suffix, with every name the list
 * can give, which must all be tokens or INTERNAL.
 */
export const FAMILIES: Readonly<Record<string, readonly string[]>> = {
	'palette-*': PALETTE,
	'palette-*-soft': PALETTE,
	'hue-*': PALETTE,
	// motion.ts reads the durations by name.
	'dur-*': ['micro', 'panel', 'view', 'fade'],
	'text-*': ['xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl'],
	// The table's distribution bars (table.ts) color by category and priority.
	'status-*': STATUS_CATEGORIES,
	'priority-*': PRIORITIES,
	// Highlighted words in a page (docs/formatting.ts).
	'highlight-*': HIGHLIGHTS,
	// A pie chart's slices in a diagram (docs/mermaid.ts).
	'chart-series-*': ['1', '2', '3', '4', '5', '6']
};
