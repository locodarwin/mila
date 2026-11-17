// The `/` menu's blocks: what typing `/` at the
// start of a line offers to insert in a task's notes or a wiki page, and
// how the menu narrows them as more is typed.
import type { Editor } from '@tiptap/core';

export interface Block {
	id: string;
	label: string;
	/** More words that find it. */
	words: string[];
	/** Inserts the block where the `/` was, or for an image, asks for one. */
	insert?: (editor: Editor) => void;
}

const chain = (e: Editor) => e.chain().focus();

const column = () => ({ type: 'column', content: [{ type: 'paragraph' }] });

/**
 * Puts a block where the empty line is and the caret this far into it: past
 * the openings of the block and what it holds first.
 */
function insertInto(e: Editor, content: object, depth: number) {
	const start = e.state.selection.$from.before();
	chain(e)
		.insertContent(content)
		.setTextSelection(start + depth)
		.run();
}

export const BLOCKS: Block[] = [
	{
		id: 'heading',
		label: 'Heading',
		words: ['title', 'h2'],
		insert: (e) => chain(e).setNode('heading', { level: 2 }).run()
	},
	{
		id: 'bullets',
		label: 'Bulleted list',
		words: ['list', 'bullet', 'ul'],
		insert: (e) => chain(e).toggleBulletList().run()
	},
	{
		id: 'numbers',
		label: 'Numbered list',
		words: ['list', 'ordered', 'ol'],
		insert: (e) => chain(e).toggleOrderedList().run()
	},
	{
		id: 'checklist',
		label: 'Checklist',
		words: ['todo', 'task', 'checkbox'],
		insert: (e) => chain(e).toggleTaskList().run()
	},
	{
		id: 'table',
		label: 'Table',
		words: ['grid', 'columns', 'rows'],
		insert: (e) => chain(e).insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
	},
	{
		id: 'code',
		label: 'Code block',
		words: ['code', 'pre', 'snippet'],
		insert: (e) => chain(e).toggleCodeBlock().run()
	},
	{
		id: 'quote',
		label: 'Quote',
		words: ['blockquote', 'cite'],
		insert: (e) => chain(e).toggleBlockquote().run()
	},
	{
		id: 'columns-2',
		label: 'Two columns',
		words: ['columns', 'side by side', 'layout'],
		insert: (e) =>
			insertInto(e, { type: 'columns', attrs: { layout: '1:1' }, content: [column(), column()] }, 3)
	},
	{
		id: 'columns-3',
		label: 'Three columns',
		words: ['columns', 'side by side', 'layout'],
		insert: (e) =>
			insertInto(
				e,
				{ type: 'columns', attrs: { layout: '1:1:1' }, content: [column(), column(), column()] },
				3
			)
	},
	{
		id: 'fold',
		label: 'Foldable section',
		words: ['toggle', 'collapse', 'details', 'expand', 'section'],
		insert: (e) =>
			insertInto(
				e,
				{
					type: 'fold',
					attrs: { open: true },
					content: [{ type: 'foldTitle' }, { type: 'paragraph' }]
				},
				2
			)
	},
	// A view asks which one first; the editor's tools do that.
	{ id: 'view', label: 'View', words: ['saved view', 'tasks', 'embed'] },
	// So does a report, then which of its charts.
	{ id: 'report', label: 'Report', words: ['chart', 'graph', 'burnup', 'reports'] },
	{
		id: 'note',
		label: 'Note box',
		words: ['callout', 'info', 'box'],
		insert: (e) => chain(e).wrapIn('callout', { kind: 'note' }).run()
	},
	{
		id: 'tip',
		label: 'Tip box',
		words: ['callout', 'hint', 'box'],
		insert: (e) => chain(e).wrapIn('callout', { kind: 'tip' }).run()
	},
	{
		id: 'warning',
		label: 'Warning box',
		words: ['callout', 'caution', 'danger', 'box'],
		insert: (e) => chain(e).wrapIn('callout', { kind: 'warning' }).run()
	},
	// The image asks which one first; the editor's tools do that.
	{ id: 'image', label: 'Image', words: ['picture', 'photo', 'attachment'] },
	{
		id: 'rule',
		label: 'Divider',
		words: ['rule', 'line', 'separator', 'hr'],
		insert: (e) => chain(e).setHorizontalRule().run()
	}
];

/** The blocks a query finds: those whose name or words start with it, in menu order. */
export function findBlocks(query: string): Block[] {
	const q = query.trim().toLowerCase();
	if (!q) return BLOCKS;
	return BLOCKS.filter((b) =>
		[b.label, ...b.words].some((w) =>
			w
				.toLowerCase()
				.split(/\s+/)
				.some((part) => part.startsWith(q))
		)
	);
}

/**
 * The `/` being typed at the start of a paragraph, with what follows it so
 * far: the menu is open while this is found. from and to are the text's
 * positions in the document, which the chosen block replaces.
 */
export function slashAt(editor: Editor): { query: string; from: number; to: number } | null {
	const { selection } = editor.state;
	if (!selection.empty) return null;
	const $from = selection.$from;
	if ($from.parent.type.name !== 'paragraph') return null;
	const before = $from.parent.textBetween(0, $from.parentOffset, undefined, '￼');
	const m = /^\/(\p{L}{0,24})$/u.exec(before);
	if (!m) return null;
	return { query: m[1], from: $from.start(), to: $from.pos };
}
