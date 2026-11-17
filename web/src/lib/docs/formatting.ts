// The formatting beyond Markdown: highlights, fonts
// and sizes, columns, foldable sections, an image's size, side and caption, a
// saved view in a page, and a page's own font and size. What Markdown can't
// say is written as an HTML comment marker or a little HTML that other
// Markdown apps show as plain words. server/internal/ydoc/formatting.go reads
// and writes the same, and shared/doc-vectors.json holds the two together.
// Loaded only with the editor.
import {
	Extension,
	Mark,
	mergeAttributes,
	Node,
	type JSONContent,
	type MarkdownTokenizer
} from '@tiptap/core';
import { HIGHLIGHTS } from '../theme/palette';

export const HIGHLIGHT_COLORS = HIGHLIGHTS;
export type HighlightColor = (typeof HIGHLIGHT_COLORS)[number];
/** The fonts a page or words may take: the ones themes pick from (theme/document.ts FONTS). */
export const PAGE_FONTS = [
	'inter',
	'nunito',
	'source-serif',
	'jetbrains-mono',
	'system-sans',
	'system-serif',
	'system-mono'
] as const;
export type PageFont = (typeof PAGE_FONTS)[number];
/** What each font is, as theme/document.ts FONTS has it (formatting.spec.ts holds the two together). */
export const FONT_STACKS: Record<PageFont, string> = {
	inter: "'Inter Variable', system-ui, sans-serif",
	nunito: "'Nunito Variable', system-ui, sans-serif",
	'source-serif': "'Source Serif 4 Variable', Georgia, serif",
	'jetbrains-mono': "'JetBrains Mono Variable', ui-monospace, monospace",
	'system-sans': 'system-ui, sans-serif',
	'system-serif': 'ui-serif, Georgia, serif',
	'system-mono': 'ui-monospace, monospace'
};
/** What each font is called where it's picked. */
export const FONT_NAMES: Record<PageFont, string> = {
	inter: 'Inter',
	nunito: 'Nunito',
	'source-serif': 'Source Serif',
	'jetbrains-mono': 'JetBrains Mono',
	'system-sans': "This device's sans",
	'system-serif': "This device's serif",
	'system-mono': "This device's mono"
};
/** How much bigger or smaller each size is than the text around it, or than the page's. */
export const SIZE_SCALE: Record<WordSize | 'normal', number> = {
	small: 0.85,
	normal: 1,
	large: 1.3,
	huge: 1.7
};
export const PAGE_SCALE: Record<PageSize | 'normal', number> = {
	small: 0.9,
	normal: 1,
	large: 1.15
};
export const WORD_SIZES = ['small', 'large', 'huge'] as const;
export type WordSize = (typeof WORD_SIZES)[number];
export const PAGE_SIZES = ['small', 'large'] as const;
export type PageSize = (typeof PAGE_SIZES)[number];
export const COLUMN_LAYOUTS: Record<2 | 3, string[]> = { 2: ['1:1', '2:1', '1:2'], 3: ['1:1:1'] };
export const IMAGE_SIZES = [25, 50, 75, 100] as const;
export const IMAGE_SIDES = ['left', 'center', 'right'] as const;
export type ImageSide = (typeof IMAGE_SIDES)[number];

const FONT = PAGE_FONTS.join('|');
const UUID = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}';
const COLUMNS = /^<!-- mila:columns(?: (1:1|2:1|1:2|1:1:1))? -->$/;
const COLUMN = '<!-- mila:column -->';
const END = '<!-- mila:end -->';
const FOLD = /^<details( open)?>$/;
const SUMMARY = /^<summary>(.*)<\/summary>[ \t]*$/;
const FOLD_END = '</details>';
const VIEW = new RegExp(`^<!-- mila:view (${UUID})(?: (list|board|table))? -->[ \\t]*(?:\\n|$)`);
/** How a view in a page is drawn, when not as the view itself is. */
export const VIEW_LAYOUTS = ['list', 'board', 'table'] as const;
/** A chart's key (lib/reports.ts, REPORT_CHARTS), kept as written even when unknown. */
const CHART = '[a-z][a-z0-9-]{0,39}';
const REPORT = new RegExp(`^<!-- mila:report (${UUID}) (${CHART}) -->[ \\t]*(?:\\n|$)`);
const PAGE = new RegExp(
	`^<!-- mila:page(?: font=(${FONT}))?(?: size=(small|large))? -->[ \\t]*(?:\\n|$)`
);
export const IMAGE_MARKER = /^<!-- mila:image (25|50|75|100) (left|center|right) -->/;

/** The address of a saved view in the app. */
export const viewAddress = (id: string) => `/views/${id}`;

const blank = (line: string | undefined) => line === undefined || line.trim() === '';
const trimBlankLines = (s: string) => s.replace(/^(?:[ \t]*\n)+/, '').replace(/(?:\n[ \t]*)+$/, '');

/** Text in HTML, as it's written: the few characters HTML needs, and numbers. */
function decodeHtml(s: string): string {
	return s
		.replace(/&#(\d+);/g, (_, n) => String.fromCodePoint(Number(n)))
		.replace(/&#[xX]([0-9a-fA-F]+);/g, (_, n) => String.fromCodePoint(parseInt(n, 16)))
		.replace(/&(amp|lt|gt|quot|apos|nbsp);/g, (_, e: string) =>
			e === 'amp'
				? '&'
				: e === 'lt'
					? '<'
					: e === 'gt'
						? '>'
						: e === 'quot'
							? '"'
							: e === 'apos'
								? "'"
								: ' '
		);
}
const encodeHtml = (s: string) =>
	s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

/** A text's lines, and whether each is inside a fenced code block, where markers aren't markers. */
function linesOf(src: string) {
	let fence: string | null = null;
	return src.split('\n').map((line) => {
		const f = /^ {0,3}(`{3,}|~{3,})/.exec(line);
		const was = fence;
		if (f && fence === null) fence = f[1];
		else if (f && fence !== null && f[1][0] === fence[0] && f[1].length >= fence.length)
			fence = null;
		return { line, inFence: was !== null || fence !== null };
	});
}

/** An inline HTML tag and the text up to its closing tag, as underline reads `<u>`. */
function tagTokenizer(name: string, open: RegExp, close: string): MarkdownTokenizer {
	return {
		name,
		level: 'inline',
		start: (src: string) => src.indexOf(close === '</mark>' ? '<mark ' : '<span '),
		tokenize(src, _tokens, lexer) {
			const m = open.exec(src);
			if (!m) return undefined;
			const delimiters = new RegExp(`\\\\[\\s\\S]|\`+|${close.replace('/', '\\/')}`, 'gi');
			delimiters.lastIndex = m[0].length;
			let d = delimiters.exec(src);
			while (d) {
				if (d[0].startsWith('`')) {
					const span = /^(`+)([^`]|[^`][\s\S]*?[^`])\1(?!`)/.exec(src.slice(d.index));
					if (span) delimiters.lastIndex = d.index + span[0].length;
				} else if (!d[0].startsWith('\\')) {
					const text = src.slice(m[0].length, d.index);
					if (!text) return undefined;
					return {
						type: name,
						raw: src.slice(0, delimiters.lastIndex),
						text,
						match: m,
						tokens: lexer.inlineTokens(text)
					};
				}
				d = delimiters.exec(src);
			}
			return undefined;
		}
	};
}

/** Words picked out in one of five colors, each a theme token. */
export const Highlight = Mark.create({
	name: 'highlight',
	addAttributes() {
		return {
			color: {
				default: 'yellow',
				parseHTML: (el) => el.getAttribute('data-color') ?? 'yellow',
				// Its theme's color by name, so every theme says what it is.
				renderHTML: (attrs) => {
					const color = HIGHLIGHTS.includes(attrs.color) ? attrs.color : 'yellow';
					return { 'data-color': color, style: `background-color: var(--highlight-${color})` };
				}
			}
		};
	},
	parseHTML() {
		return [{ tag: 'mark' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['mark', mergeAttributes({ class: 'highlight' }, HTMLAttributes), 0];
	},
	markdownTokenName: 'highlight',
	markdownTokenizer: tagTokenizer(
		'highlight',
		/^<mark data-color="(yellow|green|blue|pink|gray)">/,
		'</mark>'
	),
	parseMarkdown(token, h) {
		return h.applyMark('highlight', h.parseInline(token.tokens ?? []), { color: token.match[1] });
	},
	renderMarkdown(node, h) {
		const color = HIGHLIGHT_COLORS.includes(node.attrs?.color) ? node.attrs?.color : 'yellow';
		return `<mark data-color="${color}">${h.renderChildren(node)}</mark>`;
	}
});

/** Words in another of Mila's fonts, or another size. */
export const TextStyle = Mark.create({
	name: 'textStyle',
	addAttributes() {
		return {
			font: {
				default: null,
				parseHTML: (el) => el.getAttribute('data-font'),
				renderHTML: (attrs) =>
					attrs.font && Object.hasOwn(FONT_STACKS, attrs.font)
						? {
								'data-font': attrs.font,
								style: `font-family: ${FONT_STACKS[attrs.font as PageFont]}`
							}
						: {}
			},
			size: {
				default: null,
				parseHTML: (el) => el.getAttribute('data-size'),
				renderHTML: (attrs) => (attrs.size ? { 'data-size': attrs.size } : {})
			}
		};
	},
	parseHTML() {
		return [{ tag: 'span[data-font]' }, { tag: 'span[data-size]' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['span', mergeAttributes({ class: 'text-style' }, HTMLAttributes), 0];
	},
	markdownTokenName: 'textStyle',
	markdownTokenizer: tagTokenizer(
		'textStyle',
		new RegExp(`^<span(?: data-font="(${FONT})")?(?: data-size="(small|large|huge)")?>`),
		'</span>'
	),
	parseMarkdown(token, h) {
		const [, font, size] = token.match as string[];
		if (!font && !size) return h.parseInline(token.tokens ?? []);
		return h.applyMark('textStyle', h.parseInline(token.tokens ?? []), {
			font: font ?? null,
			size: size ?? null
		});
	},
	renderMarkdown(node, h) {
		const font = PAGE_FONTS.includes(node.attrs?.font) ? ` data-font="${node.attrs?.font}"` : '';
		const size = WORD_SIZES.includes(node.attrs?.size) ? ` data-size="${node.attrs?.size}"` : '';
		if (!font && !size) return h.renderChildren(node);
		return `<span${font}${size}>${h.renderChildren(node)}</span>`;
	}
});

/** Blocks between a start line and its end, by lines, with nested starts counted. */
function scan(
	src: string,
	from: number,
	opens: (lines: string[], i: number) => boolean,
	ends: (lines: string[], i: number) => boolean,
	splits?: (line: string) => boolean
): { parts: string[]; end: number } | null {
	const all = linesOf(src);
	const lines = all.map((l) => l.line);
	let depth = 0;
	let start = from;
	const parts: string[] = [];
	for (let i = from; i < lines.length; i++) {
		if (all[i].inFence) continue;
		const line = lines[i].replace(/[ \t]+$/, '');
		if (opens(lines, i)) depth++;
		else if (ends(lines, i) && depth > 0) depth--;
		else if (splits?.(line) && depth === 0) {
			parts.push(lines.slice(start, i).join('\n'));
			start = i + 1;
		} else if (ends(lines, i)) {
			parts.push(lines.slice(start, i).join('\n'));
			return { parts: parts.map(trimBlankLines), end: i };
		}
	}
	return null;
}

const rawUpTo = (src: string, line: number) => {
	const lines = src.split('\n');
	const raw = lines.slice(0, line + 1).join('\n');
	return line + 1 < lines.length ? raw + '\n' : raw;
};

type Lexer = { blockTokens: (s: string) => unknown[] };
type Helpers = {
	parseBlockChildren?: (t: unknown[]) => JSONContent[];
	parseChildren: (t: unknown[]) => JSONContent[];
	createNode: (type: string, attrs?: object, content?: JSONContent[]) => JSONContent;
};
const children = (h: Helpers, tokens: unknown[]) => {
	const content = (h.parseBlockChildren ?? h.parseChildren)(tokens);
	return content.length ? content : [h.createNode('paragraph', undefined, [])];
};
type Render = {
	renderChild?: (n: JSONContent, i: number) => string;
	renderChildren: (n: JSONContent[]) => string;
};
// An empty paragraph inside is written as the editor writes one at the top, a no-break space.
const renderBlocks = (nodes: JSONContent[], h: Render) =>
	nodes
		.map((child, i) =>
			child.type === 'paragraph' && !child.content?.length
				? '&nbsp;'
				: (h.renderChild?.(child, i) ?? h.renderChildren([child]))
		)
		.join('\n\n');

/** A column of blocks, in columns. */
export const Column = Node.create({
	name: 'column',
	content: 'block+',
	isolating: true,
	parseHTML() {
		return [{ tag: 'div[data-column]' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['div', mergeAttributes({ 'data-column': '', class: 'column' }, HTMLAttributes), 0];
	}
});

/** Two or three columns side by side, which stack where there's no room. */
export const Columns = Node.create({
	name: 'columns',
	group: 'block',
	content: 'column{2,3}',
	defining: true,
	isolating: true,
	addAttributes() {
		return {
			layout: {
				default: '1:1',
				parseHTML: (el) => el.getAttribute('data-layout') ?? '1:1',
				renderHTML: (attrs) => ({ 'data-layout': attrs.layout })
			}
		};
	},
	parseHTML() {
		return [{ tag: 'div[data-columns]' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['div', mergeAttributes({ 'data-columns': '', class: 'columns' }, HTMLAttributes), 0];
	},
	markdownTokenName: 'columns',
	markdownTokenizer: {
		name: 'columns',
		level: 'block',
		start: (src) => src.search(/^<!-- mila:columns/m),
		tokenize(src, _tokens, lexer) {
			const first = src.split('\n', 1)[0].replace(/[ \t]+$/, '');
			const m = COLUMNS.exec(first);
			if (!m) return undefined;
			const found = scan(
				src,
				1,
				(lines, i) => COLUMNS.test(lines[i].replace(/[ \t]+$/, '')),
				(lines, i) => lines[i].replace(/[ \t]+$/, '') === END,
				(line) => line === COLUMN
			);
			if (!found || found.parts.length < 2 || found.parts.length > 3) return undefined;
			const count = found.parts.length as 2 | 3;
			const layout = COLUMN_LAYOUTS[count].includes(m[1] ?? '') ? m[1] : COLUMN_LAYOUTS[count][0];
			return {
				type: 'columns',
				raw: rawUpTo(src, found.end),
				layout,
				parts: found.parts.map((p) => (lexer as unknown as Lexer).blockTokens(p))
			};
		}
	},
	parseMarkdown(token, h) {
		return h.createNode(
			'columns',
			{ layout: token.layout },
			(token.parts as unknown[][]).map((part) =>
				h.createNode('column', undefined, children(h as Helpers, part))
			)
		);
	},
	renderMarkdown(node: JSONContent, h) {
		const cols = node.content ?? [];
		const count = cols.length as 2 | 3;
		const layout = String(node.attrs?.layout ?? '');
		const named =
			COLUMN_LAYOUTS[count]?.includes(layout) && layout !== COLUMN_LAYOUTS[count][0]
				? ` ${layout}`
				: '';
		const parts = cols.map((col) => renderBlocks(col.content ?? [], h as Render));
		return `<!-- mila:columns${named} -->\n\n${parts.join(`\n\n${COLUMN}\n\n`)}\n\n${END}`;
	}
});

/** A foldable section's title: plain words, which every Markdown app shows. */
export const FoldTitle = Node.create({
	name: 'foldTitle',
	content: 'text*',
	marks: '',
	defining: true,
	parseHTML() {
		return [{ tag: 'summary' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['div', mergeAttributes({ class: 'fold-title' }, HTMLAttributes), 0];
	}
});

/** A title and what's under it, which a reader opens and closes for themselves. */
export const Fold = Node.create({
	name: 'fold',
	group: 'block',
	content: 'foldTitle block+',
	defining: true,
	addAttributes() {
		return {
			open: {
				default: false,
				parseHTML: (el) => el.hasAttribute('open'),
				renderHTML: (attrs) => (attrs.open ? { 'data-open': '' } : {})
			}
		};
	},
	parseHTML() {
		return [{ tag: 'details' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['div', mergeAttributes({ 'data-fold': '', class: 'fold' }, HTMLAttributes), 0];
	},
	addKeyboardShortcuts() {
		return {
			// From the title, Enter goes to what's under it.
			Enter: ({ editor }) => {
				const from = editor.state.selection.$from;
				if (from.parent.type.name !== 'foldTitle') return false;
				return editor.commands.setTextSelection(from.after() + 1);
			}
		};
	},
	markdownTokenName: 'fold',
	markdownTokenizer: {
		name: 'fold',
		level: 'block',
		start: (src) => src.search(/^<details/m),
		tokenize(src, _tokens, lexer) {
			const lines = src.split('\n');
			const head = FOLD.exec(lines[0] ?? '');
			const title = SUMMARY.exec(lines[1] ?? '');
			if (!head || !title || !blank(lines[2])) return undefined;
			const found = scan(
				src,
				2,
				(ls, i) => FOLD.test(ls[i]) && SUMMARY.test(ls[i + 1] ?? ''),
				(ls, i) => ls[i].replace(/[ \t]+$/, '') === FOLD_END && blank(ls[i + 1])
			);
			if (!found) return undefined;
			return {
				type: 'fold',
				raw: rawUpTo(src, found.end),
				open: head[1] !== undefined,
				title: decodeHtml(title[1]),
				body: (lexer as unknown as Lexer).blockTokens(found.parts[0])
			};
		}
	},
	parseMarkdown(token, h) {
		const title = token.title ? [{ type: 'text', text: token.title }] : undefined;
		return h.createNode('fold', { open: token.open }, [
			h.createNode('foldTitle', undefined, title),
			...children(h as Helpers, token.body as unknown[])
		]);
	},
	renderMarkdown(node: JSONContent, h) {
		const [titleNode, ...body] = node.content ?? [];
		const title = (titleNode?.content ?? []).map((t) => t.text ?? '').join('');
		const inner = renderBlocks(body, h as Render) || '&nbsp;';
		const open = node.attrs?.open ? ' open' : '';
		return `<details${open}>\n<summary>${encodeHtml(title.replace(/\n/g, ' '))}</summary>\n\n${inner}\n\n${FOLD_END}`;
	}
});

/** A shared saved view's tasks, drawn where it sits from the device's copy. */
export const ViewEmbed = Node.create({
	name: 'viewEmbed',
	group: 'block',
	atom: true,
	selectable: true,
	draggable: true,
	addAttributes() {
		return {
			id: {
				default: null,
				parseHTML: (el) => el.getAttribute('data-view'),
				renderHTML: (a) => ({ 'data-view': a.id })
			},
			layout: {
				default: null,
				parseHTML: (el) => el.getAttribute('data-layout'),
				renderHTML: (a) => (a.layout ? { 'data-layout': a.layout } : {})
			},
			name: {
				default: '',
				parseHTML: (el) => el.getAttribute('data-name') ?? '',
				renderHTML: (a) => ({ 'data-name': a.name })
			}
		};
	},
	parseHTML() {
		return [{ tag: 'div[data-view]' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['div', mergeAttributes({ class: 'view-embed' }, HTMLAttributes)];
	},
	markdownTokenName: 'viewEmbed',
	markdownTokenizer: {
		name: 'viewEmbed',
		level: 'block',
		start: (src) => src.search(/^<!-- mila:view /m),
		tokenize(src) {
			const m = VIEW.exec(src);
			if (!m) return undefined;
			const rest = src.slice(m[0].length);
			const link = new RegExp(
				`^\\[((?:\\\\.|[^\\]\\\\\\n])+)\\]\\(${viewAddress(m[1]).replace(/\//g, '\\/')}\\)[ \\t]*(?:\\n(?=[ \\t]*(?:\\n|$))|$)`
			).exec(m[0].endsWith('\n') ? rest : '');
			const layout = m[2] ?? null;
			if (!link) return { type: 'viewEmbed', raw: m[0], id: m[1], name: '', layout };
			const name = decodeHtml(link[1].replace(/\\([!-/:-@[-`{-~])/g, '$1'));
			return { type: 'viewEmbed', raw: m[0] + link[0], id: m[1], name, layout };
		}
	},
	parseMarkdown(token, h) {
		return h.createNode('viewEmbed', { id: token.id, name: token.name, layout: token.layout });
	},
	renderMarkdown(node: JSONContent) {
		const id = String(node.attrs?.id ?? '').toLowerCase();
		if (!new RegExp(`^${UUID}$`).test(id)) return '';
		const layout = VIEW_LAYOUTS.includes(node.attrs?.layout) ? ` ${node.attrs?.layout}` : '';
		const marker = `<!-- mila:view ${id}${layout} -->`;
		const name = String(node.attrs?.name ?? '')
			.replace(/\n/g, ' ')
			.trim();
		return name
			? `${marker}\n[${name.replace(/([\\`*_[\]~<&])/g, '\\$1')}](${viewAddress(id)})`
			: marker;
	}
});

/**
 * One chart of a shared report view, drawn where it sits from the device's
 * copy: a marker naming the view and the chart, then a link to the view.
 */
export const ReportEmbed = Node.create({
	name: 'reportEmbed',
	group: 'block',
	atom: true,
	selectable: true,
	draggable: true,
	addAttributes() {
		return {
			id: {
				default: null,
				parseHTML: (el) => el.getAttribute('data-report'),
				renderHTML: (a) => ({ 'data-report': a.id })
			},
			chart: {
				default: null,
				parseHTML: (el) => el.getAttribute('data-chart'),
				renderHTML: (a) => (a.chart ? { 'data-chart': a.chart } : {})
			},
			name: {
				default: '',
				parseHTML: (el) => el.getAttribute('data-name') ?? '',
				renderHTML: (a) => ({ 'data-name': a.name })
			}
		};
	},
	parseHTML() {
		return [{ tag: 'div[data-report]' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['div', mergeAttributes({ class: 'report-embed' }, HTMLAttributes)];
	},
	markdownTokenName: 'reportEmbed',
	markdownTokenizer: {
		name: 'reportEmbed',
		level: 'block',
		start: (src) => src.search(/^<!-- mila:report /m),
		tokenize(src) {
			const m = REPORT.exec(src);
			if (!m) return undefined;
			const rest = src.slice(m[0].length);
			const link = new RegExp(
				`^\\[((?:\\\\.|[^\\]\\\\\\n])+)\\]\\(${viewAddress(m[1]).replace(/\//g, '\\/')}\\)[ \\t]*(?:\\n(?=[ \\t]*(?:\\n|$))|$)`
			).exec(m[0].endsWith('\n') ? rest : '');
			if (!link) return { type: 'reportEmbed', raw: m[0], id: m[1], chart: m[2], name: '' };
			const name = decodeHtml(link[1].replace(/\\([!-/:-@[-`{-~])/g, '$1'));
			return { type: 'reportEmbed', raw: m[0] + link[0], id: m[1], chart: m[2], name };
		}
	},
	parseMarkdown(token, h) {
		return h.createNode('reportEmbed', { id: token.id, chart: token.chart, name: token.name });
	},
	renderMarkdown(node: JSONContent) {
		const id = String(node.attrs?.id ?? '').toLowerCase();
		const chart = String(node.attrs?.chart ?? '');
		if (!new RegExp(`^${UUID}$`).test(id) || !new RegExp(`^${CHART}$`).test(chart)) return '';
		const marker = `<!-- mila:report ${id} ${chart} -->`;
		const name = String(node.attrs?.name ?? '')
			.replace(/\n/g, ' ')
			.trim();
		return name
			? `${marker}\n[${name.replace(/([\\`*_[\]~<&])/g, '\\$1')}](${viewAddress(id)})`
			: marker;
	}
});

/** A page's own font and size, for everyone: a block that shows nothing, kept first. */
export const PageStyle = Node.create({
	name: 'pageStyle',
	group: 'block',
	atom: true,
	selectable: false,
	addAttributes() {
		return {
			font: {
				default: null,
				parseHTML: (el) => el.getAttribute('data-font'),
				renderHTML: (a) => (a.font ? { 'data-font': a.font } : {})
			},
			size: {
				default: null,
				parseHTML: (el) => el.getAttribute('data-size'),
				renderHTML: (a) => (a.size ? { 'data-size': a.size } : {})
			}
		};
	},
	parseHTML() {
		return [{ tag: 'div[data-page-style]' }];
	},
	renderHTML({ HTMLAttributes }) {
		return ['div', mergeAttributes({ 'data-page-style': '', hidden: '' }, HTMLAttributes)];
	},
	markdownTokenName: 'pageStyle',
	markdownTokenizer: {
		name: 'pageStyle',
		level: 'block',
		start: (src) => src.search(/^<!-- mila:page/m),
		tokenize(src) {
			const m = PAGE.exec(src);
			if (!m) return undefined;
			return { type: 'pageStyle', raw: m[0], font: m[1] ?? null, size: m[2] ?? null };
		}
	},
	parseMarkdown(token, h) {
		return h.createNode('pageStyle', { font: token.font, size: token.size });
	},
	renderMarkdown(node: JSONContent) {
		const font = PAGE_FONTS.includes(node.attrs?.font) ? ` font=${node.attrs?.font}` : '';
		const size = PAGE_SIZES.includes(node.attrs?.size) ? ` size=${node.attrs?.size}` : '';
		return font || size ? `<!-- mila:page${font}${size} -->` : '';
	}
});

/** The marker after an image, read as text for the paragraph to take up (imageMarkers). */
export const ImageMarker = Extension.create({
	name: 'imageMarker',
	markdownTokenName: 'imageMarker',
	markdownTokenizer: {
		name: 'imageMarker',
		level: 'inline',
		start: (src) => src.indexOf('<!-- mila:image '),
		tokenize(src) {
			const m = IMAGE_MARKER.exec(src);
			return m ? { type: 'imageMarker', raw: m[0] } : undefined;
		}
	},
	parseMarkdown(token) {
		return { type: 'text', text: token.raw };
	}
});

/** An image's marker, the size and side it gives, and the caption line under it. */
export function imageExtras(attrs: Record<string, unknown> | undefined): string {
	const size = attrs?.size;
	const side = attrs?.side;
	if (size == null && side == null) return '';
	const s = IMAGE_SIZES.includes(size as never) ? size : 100;
	const d = IMAGE_SIDES.includes(side as never) ? side : 'center';
	return `<!-- mila:image ${s} ${d} -->`;
}

/**
 * Takes up the marker after an image, its size and side, and the italic line
 * under an image alone in its paragraph, its caption, as the server does.
 */
export function imageMarkers(content: JSONContent[]): JSONContent[] {
	const out = content.map((n) => ({ ...n }));
	for (let i = 0; i + 1 < out.length; i++) {
		const img = out[i];
		const next = out[i + 1];
		if (img.type !== 'image' || next.type !== 'text' || next.marks?.length) continue;
		const m = IMAGE_MARKER.exec(next.text ?? '');
		if (!m) continue;
		img.attrs = { ...img.attrs, size: Number(m[1]), side: m[2] };
		next.text = next.text!.slice(m[0].length);
		if (!next.text) out.splice(i + 1, 1);
	}
	const [img, br, cap] = out;
	if (
		out.length === 3 &&
		img.type === 'image' &&
		img.attrs?.size != null &&
		br.type === 'text' &&
		br.text === '\n' &&
		!br.marks?.length &&
		cap.type === 'text' &&
		cap.marks?.length === 1 &&
		cap.marks[0].type === 'italic'
	) {
		img.attrs = { ...img.attrs, caption: cap.text };
		return [img];
	}
	return out;
}

/** The caption line written under an image alone in its paragraph. */
export function captionLine(
	content: JSONContent[] | undefined,
	italic: (text: string) => string
): string {
	if (content?.length !== 1 || content[0].type !== 'image' || !imageExtras(content[0].attrs))
		return '';
	const c = String(content[0].attrs?.caption ?? '')
		.replace(/\n/g, ' ')
		.trim();
	return c ? `\n${italic(c)}` : '';
}

export const formattingExtensions = [
	Highlight,
	TextStyle,
	Column,
	Columns,
	FoldTitle,
	Fold,
	ViewEmbed,
	ReportEmbed,
	PageStyle,
	ImageMarker
];
