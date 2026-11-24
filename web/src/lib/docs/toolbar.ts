// The wiki page's formatting toolbar (DocToolbar.svelte): the groups that
// fold away when the page's bar is short of room and the order they go in,
// the Insert menu (the `/` menu's blocks, grouped), and making room on a line
// of its own for a block put in from the toolbar. Loaded with the editor.
import type { Editor } from '@tiptap/core';
import { NodeSelection, TextSelection } from '@tiptap/pm/state';
import type { Component } from 'svelte';
import { BLOCKS, type Block } from './blocks';

/** What the editor's tools (DocTools) do for the toolbar and for DocEditor. */
export interface DocToolsApi {
	keydown(e: KeyboardEvent): boolean;
	/** Puts in a block from the `/` menu's list, by its ID. */
	insert(id: string): void;
	/** Asks for a link's address. */
	startLink(): Promise<void>;
	/** Opens the `[[` menu, to link to a page or task. */
	linkTo(): void;
}

/** What DocEditor hands the wiki for a page's toolbar (DocEditor's toolbar()). */
export interface ToolbarParts {
	Toolbar: Component<Record<string, unknown>>;
	editor: Editor;
	/** Changes with every change to the editor's state. */
	version: number;
	/** Whether the text has focus. */
	focused: boolean;
	tools: DocToolsApi;
	/** The toolbar says how to move focus to it, or null as it goes. */
	register: (focus: (() => boolean) | null) => void;
}

/**
 * What folds away, first to last: the blocks and the pictures (both still
 * in Insert), then the lists and quote and the rarer marks (into the ⋯
 * button), then Insert's word, leaving its icon. Text style, Bold, Italic
 * and Insert always stay.
 */
export const FOLDS = ['blocks', 'pictures', 'lists', 'marks', 'label'] as const;
export type Fold = (typeof FOLDS)[number];

/** Each part's width in pixels, with its gap. base is what always stays, Insert's word included. */
export interface Widths {
	base: number;
	blocks: number;
	pictures: number;
	lists: number;
	marks: number;
	label: number;
	more: number;
}

/** The width the toolbar takes with the folds made. */
export function widthWith(w: Widths, folded: ReadonlySet<Fold>): number {
	let n = w.base;
	if (!folded.has('blocks')) n += w.blocks;
	if (!folded.has('pictures')) n += w.pictures;
	if (!folded.has('lists')) n += w.lists;
	if (!folded.has('marks')) n += w.marks;
	if (folded.has('lists') || folded.has('marks')) n += w.more;
	if (folded.has('label')) n -= w.label;
	return n;
}

/** What folds away to fit in room: in FOLDS' order, until what's left fits. */
export function folding(room: number, w: Widths): Set<Fold> {
	const folded = new Set<Fold>();
	for (const f of FOLDS) {
		if (widthWith(w, folded) <= room) break;
		folded.add(f);
	}
	return folded;
}

/** The toolbar with nothing folded, and with everything that can be. */
export const widest = (w: Widths) => widthWith(w, new Set());
export const narrowest = (w: Widths) => widthWith(w, new Set(FOLDS));

/** The Insert menu: every block in the `/` menu, in groups. */
export const INSERT_GROUPS: { name: string; ids: string[] }[] = [
	{
		name: 'Text',
		ids: ['heading', 'bullets', 'numbers', 'checklist', 'quote', 'code', 'table', 'rule']
	},
	{ name: 'Layout', ids: ['columns-2', 'columns-3', 'fold'] },
	{ name: 'Boxes', ids: ['note', 'tip', 'warning'] },
	{ name: 'Pictures', ids: ['image', 'diagram', 'drawing'] },
	{ name: 'From Mila', ids: ['view', 'report'] }
];

/** A block by its ID in the `/` menu. */
export const blockOf = (id: string): Block | undefined => BLOCKS.find((b) => b.id === id);

/**
 * Blocks that change the line the caret is on (a heading, a list, a box).
 * Every other block goes on a line of its own.
 */
export const IN_PLACE = new Set([
	'heading',
	'bullets',
	'numbers',
	'checklist',
	'code',
	'quote',
	'note',
	'tip',
	'warning'
]);

/** The text styles, as Text style offers them: 0 for text, else a heading's level. */
export const TEXT_STYLES = [
	{ level: 0, label: 'Text' },
	{ level: 1, label: 'Heading 1' },
	{ level: 2, label: 'Heading 2' },
	{ level: 3, label: 'Heading 3' }
] as const;

/** What Text style says of the line the caret is on. */
export function styleName(editor: Editor): string {
	for (const s of TEXT_STYLES)
		if (s.level && editor.isActive('heading', { level: s.level })) return s.label;
	return editor.isActive('heading') ? 'Heading' : 'Text';
}

/**
 * Puts the caret on an empty line, as `/` has it: where it is, if that line
 * is empty, else a new one after the line (or the block selected). A block
 * from the toolbar then goes in there, as it does from the `/` menu, and
 * never splits the words around the caret.
 */
export function openLine(editor: Editor): void {
	const { state } = editor;
	const sel = state.selection;
	const $to = sel.$to;
	if (sel.empty && $to.parent.type.name === 'paragraph' && $to.parent.content.size === 0) return;
	const pos = sel instanceof NodeSelection ? sel.to : $to.parent.isTextblock ? $to.after() : sel.to;
	try {
		const tr = state.tr.insert(pos, state.schema.nodes.paragraph.create());
		tr.setSelection(TextSelection.create(tr.doc, pos + 1));
		editor.view.dispatch(tr.scrollIntoView());
	} catch {
		// Nowhere a line may go here: the block goes where the caret is.
	}
}
