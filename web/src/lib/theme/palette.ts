// Label and project colors are stored by name. Each theme renders the name
// through its own --palette-<name> token, so colors stay legible everywhere.
export const PALETTE = [
	'red',
	'orange',
	'amber',
	'yellow',
	'lime',
	'green',
	'teal',
	'cyan',
	'blue',
	'indigo',
	'purple',
	'pink'
] as const;

export type PaletteColor = (typeof PALETTE)[number];

/** The colors words in a page can be highlighted in, each its theme's --highlight-<name>. */
export const HIGHLIGHTS = ['yellow', 'green', 'blue', 'pink', 'gray'] as const;

export const STATUS_CATEGORIES = ['backlog', 'todo', 'in-progress', 'done', 'canceled'] as const;
export type StatusCategory = (typeof STATUS_CATEGORIES)[number];

export const PRIORITIES = ['none', 'low', 'medium', 'high', 'urgent'] as const;
export type Priority = (typeof PRIORITIES)[number];
