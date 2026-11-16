// What a live document may hold (docs/PLAN.md section 3): the editor's
// nodes and marks, the same for task notes and wiki pages, with how each
// reads and writes Markdown. The editor, its read-only view and the test
// vectors (vectors.ts) all build from this list, so the schema the server
// holds documents to (server/internal/ydoc/schema.json) is this one.
// Loaded only with the editor.
import {
	mergeAttributes,
	Node,
	type AnyExtension,
	type JSONContent,
	type NodeViewRenderer
} from '@tiptap/core';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import Image from '@tiptap/extension-image';
import { TaskItem, TaskList } from '@tiptap/extension-list';
import BaseParagraph from '@tiptap/extension-paragraph';
import { Link } from '@tiptap/extension-link';
import { Fragment, type Node as PMNode } from '@tiptap/pm/model';
import { Table, TableKit } from '@tiptap/extension-table';
import StarterKit from '@tiptap/starter-kit';
import {
	captionLine,
	formattingExtensions,
	IMAGE_SIDES,
	IMAGE_SIZES,
	imageExtras,
	imageMarkers
} from './formatting';
import { common, createLowlight } from 'lowlight';

/** GitHub's boxes, `> [!NOTE]` and the rest, in the order the menu offers them. */
export const CALLOUT_KINDS = ['note', 'tip', 'important', 'warning', 'caution'] as const;
export type CalloutKind = (typeof CALLOUT_KINDS)[number];

/** What each box is called, as it shows at the top of the box and to screen readers. */
export const CALLOUT_NAMES: Record<CalloutKind, string> = {
	note: 'Note',
	tip: 'Tip',
	important: 'Important',
	warning: 'Warning',
	caution: 'Caution'
};

const CALLOUT =
	/^ {0,3}> ?\[!(note|tip|important|warning|caution)\][ \t]*(?:\n|$)((?: {0,3}>[^\n]*(?:\n|$))*)/i;

/** A note, tip, important, warning or caution box: GitHub's `> [!NOTE]`. */
export const Callout = Node.create({
	name: 'callout',
	group: 'block',
	content: 'block+',
	defining: true,
	addAttributes() {
		return {
			kind: {
				default: 'note',
				parseHTML: (el) => el.getAttribute('data-kind') ?? 'note',
				renderHTML: (attrs) => ({ 'data-kind': attrs.kind })
			}
		};
	},
	parseHTML() {
		return [{ tag: 'aside[data-callout]' }];
	},
	renderHTML({ node, HTMLAttributes }) {
		const name = CALLOUT_NAMES[node.attrs.kind as CalloutKind] ?? CALLOUT_NAMES.note;
		return [
			'aside',
			mergeAttributes(
				{ 'data-callout': '', class: 'callout', role: 'note', 'aria-label': name },
				HTMLAttributes
			),
			0
		];
	},
	markdownTokenName: 'callout',
	markdownTokenizer: {
		name: 'callout',
		level: 'block',
		start: (src) => src.search(/^ {0,3}> ?\[!/m),
		tokenize(src, _tokens, lexer) {
			const m = CALLOUT.exec(src);
			if (!m) return undefined;
			const inner = m[2]
				.replace(/\n$/, '')
				.split('\n')
				.map((l) => l.replace(/^ {0,3}> ?/, ''))
				.join('\n');
			return {
				type: 'callout',
				raw: m[0],
				kind: m[1].toLowerCase(),
				tokens: inner.trim() ? lexer.blockTokens(inner) : []
			};
		}
	},
	parseMarkdown(token, h) {
		const read = h.parseBlockChildren ?? h.parseChildren;
		const content = read(token.tokens ?? []);
		return h.createNode(
			'callout',
			{ kind: token.kind },
			content.length ? content : [h.createNode('paragraph', undefined, [])]
		);
	},
	renderMarkdown(node: JSONContent, h) {
		const kind = String(node.attrs?.kind ?? 'note').toUpperCase();
		const parts = (node.content ?? []).map((child, i) =>
			(h.renderChild?.(child, i) ?? h.renderChildren([child]))
				.split('\n')
				.map((l) => (l.trim() === '' ? '>' : `> ${l}`))
				.join('\n')
		);
		return [`> [!${kind}]`, parts.join('\n>\n')].filter(Boolean).join('\n');
	}
});

/**
 * Paragraphs as the starter kit has them, except that a paragraph holding
 * only an image stays a paragraph: images here are inline, and the
 * default reading lifts a lone one out, where an inline node cannot be.
 */
const Paragraph = BaseParagraph.extend({
	parseMarkdown(token, helpers) {
		const tokens = token.tokens ?? [];
		const node: JSONContent =
			tokens.length === 1 && tokens[0].type === 'image'
				? helpers.createNode('paragraph', undefined, helpers.parseInline(tokens))
				: this.parent!(token, helpers);
		// An image's size, side and caption (formatting.ts).
		if (node?.type === 'paragraph' && node.content?.some((n) => n.type === 'image'))
			node.content = imageMarkers(node.content);
		return node;
	},
	renderMarkdown(node, helpers, context) {
		const md = this.parent!(node, helpers, context);
		return md + captionLine(node.content, (c) => `*${c.replace(/([\\`*_[\]~<&])/g, '\\$1')}*`);
	}
});

/** Images, with the size, side and caption a page can give them. */
const SizedImage = Image.extend({
	addAttributes() {
		return {
			...this.parent?.(),
			size: {
				default: null,
				parseHTML: (el) => {
					const n = Number(el.getAttribute('data-size'));
					return IMAGE_SIZES.includes(n as never) ? n : null;
				},
				renderHTML: (a) => (a.size ? { 'data-size': a.size } : {})
			},
			side: {
				default: null,
				parseHTML: (el) => {
					const s = el.getAttribute('data-side');
					return IMAGE_SIDES.includes(s as never) ? s : null;
				},
				renderHTML: (a) => (a.side ? { 'data-side': a.side } : {})
			},
			caption: {
				default: null,
				parseHTML: (el) => el.getAttribute('data-caption'),
				renderHTML: (a) => (a.caption ? { 'data-caption': a.caption } : {})
			}
		};
	},
	renderMarkdown(node, helpers, context) {
		return this.parent!(node, helpers, context) + imageExtras(node.attrs);
	}
});

/** Code blocks highlighted in the languages highlight.js counts as common. */
export const lowlight = createLowlight(common);

// A link's class, target and rel are the app's alone. A hand-made update could
// set them to anything, and a class of the app's own styles could make a link
// cover the page; the document keeps whatever it holds, and it is drawn so.
const LINK_REL = 'noopener noreferrer nofollow';
const SafeLink = Link.extend({
	addAttributes() {
		return {
			...this.parent?.(),
			class: { default: null, parseHTML: () => null, renderHTML: () => ({}) },
			target: {
				default: '_blank',
				parseHTML: () => '_blank',
				renderHTML: () => ({ target: '_blank' })
			},
			rel: { default: LINK_REL, parseHTML: () => LINK_REL, renderHTML: () => ({ rel: LINK_REL }) }
		};
	}
});

/** A column width as a whole number of pixels, or none. */
function width(w: unknown): number {
	const n = Math.round(Number(w));
	return Number.isFinite(n) && n > 0 && n < 100_000 ? n : 0;
}

// A table is drawn from its cells' widths, which a hand-made update could make
// style text instead of numbers (`1px;position:fixed`); they are numbers here.
const SafeTable = Table.extend({
	renderHTML(props) {
		const clean = (n: PMNode): PMNode => {
			if (n.type.name === 'tableCell' || n.type.name === 'tableHeader') {
				const w = n.attrs.colwidth;
				const colwidth = Array.isArray(w) ? w.map(width) : null;
				return n.type.create({ ...n.attrs, colwidth }, n.content, n.marks);
			}
			const kids: PMNode[] = [];
			n.forEach((child) => kids.push(clean(child)));
			return n.copy(Fragment.fromArray(kids));
		};
		return this.parent!({ ...props, node: clean(props.node) });
	}
});

/** The document's nodes and marks. The editor adds its own behavior on top. */
export function docExtensions(
	options: { undo?: boolean; nodeViews?: Record<string, NodeViewRenderer> } = {}
): AnyExtension[] {
	const list: AnyExtension[] = [
		StarterKit.configure({
			codeBlock: false,
			paragraph: false,
			// In a live document Yjs keeps the undo history, one person's typing at a time.
			undoRedo: options.undo ? undefined : false,
			link: false
		}),
		SafeLink.configure({ openOnClick: false }),
		Paragraph,
		CodeBlockLowlight.configure({ lowlight }),
		TaskList,
		TaskItem.configure({
			nested: true,
			a11y: { checkboxLabel: (node) => node.textContent.trim() || 'Checklist item' }
		}),
		TableKit.configure({ table: false }),
		SafeTable.configure({ resizable: false }),
		SizedImage.configure({ inline: true }),
		Callout,
		...formattingExtensions
	];
	// How the editor draws some of them (nodeviews.svelte.ts); the vectors need none.
	const views = options.nodeViews ?? {};
	return list.map((ext) =>
		views[ext.name] ? ext.extend({ addNodeView: () => views[ext.name] }) : ext
	);
}
