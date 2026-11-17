// The test vectors that hold the server's reading and writing of live
// documents (server/internal/ydoc) to the editor's own: Markdown as Tiptap
// reads it, documents to write as Markdown, and Yjs updates as y-tiptap
// writes them, with what each comes to. Built here from the editor's own
// code; `npx tsx scripts/doc-vectors.ts` writes shared/doc-vectors.json,
// and docVectors.spec.ts fails when the editor's answers change. Only tests
// and the script import this, so none of it reaches the app's bundle.
import { getSchema, type JSONContent } from '@tiptap/core';
import { MarkdownManager } from '@tiptap/markdown';
import { EditorState } from '@tiptap/pm/state';
import type { Node as PMNode } from '@tiptap/pm/model';
import {
	initProseMirrorDoc,
	updateYFragment,
	yXmlFragmentToProsemirrorJSON,
	ySyncPluginKey
} from '@tiptap/y-tiptap';
import * as Y from 'yjs';
import { docExtensions } from './extensions';

export const FRAGMENT = 'default';
const extensions = docExtensions();
export const schema = getSchema(extensions);
const markdown = new MarkdownManager({ extensions });

/** Every node's and mark's attributes with their defaults, as the server needs them. */
export function schemaDefaults() {
	const pick = (
		types: Record<string, { spec: { attrs?: Record<string, { default?: unknown }> } }>
	) =>
		Object.fromEntries(
			Object.entries(types)
				.sort(([a], [b]) => (a < b ? -1 : 1))
				.map(([name, t]) => [
					name,
					Object.fromEntries(
						Object.entries(t.spec.attrs ?? {})
							.sort(([a], [b]) => (a < b ? -1 : 1))
							.map(([k, v]) => [k, v.default ?? null])
					)
				])
		);
	return {
		nodes: pick(schema.nodes as never),
		marks: pick(schema.marks as never)
	};
}

export const parseMarkdown = (md: string): JSONContent => markdown.parse(md);
export const writeMarkdown = (doc: JSONContent): string => markdown.serialize(doc);

// Markdown people write by hand, or that other tools send through the API.
const HAND: [string, string][] = [
	['paragraph', 'Plain paragraph.'],
	['two paragraphs', 'Para one\n\nPara two'],
	['soft break', 'Line one\nline two'],
	['hard break with spaces', 'Hard  \nbreak'],
	['hard break with backslash', 'Hard\\\nbreak'],
	['headings', '# One\n\n## Two\n\n### Three\n\n#### Four\n\n##### Five\n\n###### Six'],
	['setext headings', 'Title\n=====\n\nSub\n---'],
	['marks', '**bold** *italic* ~~strike~~ `code` [link](https://example.com)'],
	['underscore marks', '__bold__ and _italic_'],
	['nested marks', '***bold italic*** and **bold `code` inside**'],
	['link with title', '[docs](https://example.com/docs "The docs")'],
	['bullet list', '- one\n- two\n- three'],
	['star and plus bullets', '* one\n* two\n\n+ three'],
	['nested list', '- one\n- two\n  - nested\n  - again\n- three'],
	['ordered list', '1. first\n2. second\n3. third'],
	['ordered from three', '3. three\n4. four'],
	['ordered with parens', '1) first\n2) second'],
	['loose list', '- one\n\n  more of one\n- two'],
	['list in a quote', '> - in quote\n>\n> after'],
	['quote', '> quote line\n> more'],
	['nested quote', '> outer\n>\n> > inner'],
	['code block', '```js\nconst a = 1;\nconsole.log(a);\n```'],
	['code block without a language', '```\nplain\n  indented\n```'],
	['indented code', '    indented code\n    more'],
	['rule', 'above\n\n---\n\nbelow'],
	['escapes', 'Escapes: \\*not bold\\* and \\_not italic\\_ and \\# not a heading'],
	['entities', 'Fish &amp; chips &lt;3 &copy;'],
	['inline html marks', '<u>under</u> <em>em</em> <strong>strong</strong> <s>gone</s>'],
	['angle brackets', 'a < b > c'],
	['unicode', 'café — “quotes” 😀'],
	['brackets and pipes', 'a | b [c] (d) {e} !f'],
	['empty', ''],
	['only spaces', '   '],
	['trailing newline', 'text\n'],
	['three newlines between', 'a\n\n\nb'],
	['six newlines between', 'a\n\n\n\n\n\nb'],
	['blank lines before a list', 'a\n\n\n\n- b'],
	['code in a list', '- item\n\n  ```\n  code\n  ```'],
	['code block in python', '```python\nprint("hi")\n```'],
	['checklist', '- [ ] buy milk\n- [x] call Dana'],
	['nested checklist', '- [ ] a\n  - [x] b\n- [ ] c'],
	['checklist mixed with bullets', '- [ ] a\n- b\n- [x] c'],
	['checklist with an empty item', '- [ ] a\n- [ ]'],
	['numbered checkbox stays text', '1. [ ] not a task'],
	['table', '| Item | Qty |\n| --- | ---: |\n| Milk | 2 |\n| **Eggs** | 12 |'],
	['aligned table', '| l | c | r |\n|:--|:-:|--:|\n| 1 | 2 | 3 |'],
	['table with an escaped pipe and an empty cell', '| a | b |\n| --- | --- |\n| x \\| y |  |'],
	['table without outer pipes', 'a | b\n--- | ---\n1 | 2'],
	['table with code', '| code |\n| --- |\n| `x` |'],
	['table with only a header', '| only |\n| --- |'],
	['image', '![A seedling](/files/a.png "Spring")'],
	['image in text', 'Before ![alt](/a.png) after'],
	['image in a link', '[![i](/a.png)](https://example.com)'],
	['image in bold', '**bold ![i](/x.png) more**'],
	['image alt with escapes', '![a \\[b\\] \\\\ c \\*d](/e.png)'],
	['note box', '> [!NOTE]\n> Read this first.'],
	['tip box with a list', '> [!TIP]\n> One\n>\n> - two'],
	['empty warning box', '> [!WARNING]'],
	['lowercase box', '> [!caution]\n> Mind the step.'],
	['important box', '> [!IMPORTANT]\n> **Bold** words'],
	['box marker with a title is a quote', '> [!NOTE] title\n> body'],
	['quote then box', '> quote\n\n> [!TIP]\n> x'],
	// The formatting beyond Markdown (formatting.ts).
	[
		'highlight',
		'A <mark data-color="green">green</mark> word and <mark data-color="yellow">**bold** one</mark>'
	],
	['highlight without its closing tag stays text', 'An <mark data-color="blue">open tag'],
	['highlight in an unknown color stays text', '<mark data-color="red">x</mark>'],
	[
		'font and size',
		'<span data-font="source-serif" data-size="large">Grandma</span> and <span data-size="huge">big</span> <span data-font="jetbrains-mono">mono</span>'
	],
	['an unknown font stays text', '<span data-font="comic">x</span>'],
	['highlight in bold', '**a <mark data-color="pink">b</mark> c**'],
	[
		'columns',
		'<!-- mila:columns -->\n\n**Sun**\n\n- a\n- b\n\n<!-- mila:column -->\n\n**Shade**\n\n<!-- mila:end -->'
	],
	[
		'wide left columns',
		'<!-- mila:columns 2:1 -->\n\nleft\n\n<!-- mila:column -->\n\nright\n\n<!-- mila:end -->'
	],
	[
		'three columns',
		'<!-- mila:columns -->\n\na\n\n<!-- mila:column -->\n\nb\n\n<!-- mila:column -->\n\nc\n\n<!-- mila:end -->'
	],
	[
		'a layout for another count is the usual one',
		'<!-- mila:columns 1:1:1 -->\n\na\n\n<!-- mila:column -->\n\nb\n\n<!-- mila:end -->'
	],
	['columns without an end stay text', '<!-- mila:columns -->\n\nleft'],
	['one column stays text', '<!-- mila:columns -->\n\nonly\n\n<!-- mila:end -->'],
	[
		'columns in columns',
		'<!-- mila:columns -->\n\n<!-- mila:columns -->\n\na\n\n<!-- mila:column -->\n\nb\n\n<!-- mila:end -->\n\n<!-- mila:column -->\n\nc\n\n<!-- mila:end -->'
	],
	['a columns marker in code stays code', '```\n<!-- mila:columns -->\n```'],
	[
		'code with a marker inside columns',
		'<!-- mila:columns -->\n\n```\n<!-- mila:end -->\n```\n\n<!-- mila:column -->\n\nb\n\n<!-- mila:end -->'
	],
	[
		'columns with blank lines inside',
		'<!-- mila:columns -->\n\n\n\nleft\n\n\n\nmore\n\n<!-- mila:column -->\n\nright\n\n<!-- mila:end -->'
	],
	[
		'columns without blank lines',
		'<!-- mila:columns -->\nleft\n<!-- mila:column -->\nright\n<!-- mila:end -->'
	],
	['an empty column', '<!-- mila:columns -->\n\n<!-- mila:column -->\n\nb\n\n<!-- mila:end -->'],
	[
		'columns in a box',
		'> [!NOTE]\n> <!-- mila:columns -->\n>\n> a\n>\n> <!-- mila:column -->\n>\n> b\n>\n> <!-- mila:end -->'
	],
	['fold', '<details>\n<summary>Soil mix</summary>\n\nA third compost.\n\n- sand\n\n</details>'],
	[
		'open fold',
		'<details open>\n<summary>Mistakes &amp; lessons</summary>\n\nToo close.\n\n</details>'
	],
	[
		'folds in folds',
		'<details>\n<summary>Out</summary>\n\n<details>\n<summary>In</summary>\n\nx\n\n</details>\n\ny\n\n</details>'
	],
	[
		'a fold with nothing after its title but a line stays text',
		'<details>\n<summary>T</summary>\nbody\n\n</details>'
	],
	['a fold without its end stays text', '<details>\n<summary>T</summary>\n\nbody'],
	['a fold with an empty title', '<details>\n<summary></summary>\n\nx\n\n</details>'],
	['a fold whose end runs into text', '<details>\n<summary>T</summary>\n\nx\n\n</details>\nafter'],
	[
		'view',
		'<!-- mila:view 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa -->\n[Garden, this month](/views/7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa)'
	],
	[
		'view as a board',
		'<!-- mila:view 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa board -->\n[Garden](/views/7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa)'
	],
	[
		'view as a table with only its marker',
		'<!-- mila:view 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa table -->'
	],
	[
		'a view in an unknown layout stays text',
		'<!-- mila:view 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa gantt -->'
	],
	[
		'view with only its marker',
		'<!-- mila:view 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa -->\n\nAfter it.'
	],
	[
		'view with another link under it',
		'<!-- mila:view 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa -->\n[Elsewhere](/projects)'
	],
	[
		'view whose link runs on',
		'<!-- mila:view 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa -->\n[Garden](/views/7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa)\nmore'
	],
	[
		'view with escapes in its name',
		'<!-- mila:view 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa -->\n[A \\*starred\\* view](/views/7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa)'
	],
	[
		'report',
		'<!-- mila:report 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa burnup -->\n[Launch report](/views/7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa)'
	],
	[
		'report with only its marker',
		'<!-- mila:report 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa by-status -->\n\nAfter it.'
	],
	[
		'a report of a chart a newer app knows',
		'<!-- mila:report 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa sprint-velocity -->'
	],
	[
		'a report with no chart stays text',
		'<!-- mila:report 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa -->'
	],
	[
		'a report with a chart out of shape stays text',
		'<!-- mila:report 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa Burn_Up -->'
	],
	[
		'report whose link is to another view',
		'<!-- mila:report 7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa cfd -->\n[Other](/views/00000000-6a0e-4d1b-9c55-2e8b7d10f4aa)'
	],
	['page style', '<!-- mila:page font=source-serif size=large -->\n\n# Title'],
	['page style with only a size', '<!-- mila:page size=small -->\n\nText'],
	['a page style with an unknown font stays text', '<!-- mila:page font=comic -->'],
	['image size and side', '![Beds](/files/b.png)<!-- mila:image 50 right -->'],
	[
		'image caption',
		'![Beds](/files/b.png)<!-- mila:image 50 right -->\n*The three beds, as of last fall.*'
	],
	['image caption without a marker stays text', '![Beds](/b.png)\n*Not a caption*'],
	['image marker in text', 'Before ![a](/a.png)<!-- mila:image 25 left --> after'],
	['image marker away from an image stays text', 'Text <!-- mila:image 25 left --> here']
];

// Documents as the notes editor makes them, written out by Tiptap: the
// Markdown every task's notes held before live documents, which the
// server reads once to make each task's first document.
const t = (text: string, marks?: (string | { type: string; attrs: object })[]): JSONContent =>
	marks
		? { type: 'text', text, marks: marks.map((m) => (typeof m === 'string' ? { type: m } : m)) }
		: { type: 'text', text };
const p = (...content: JSONContent[]): JSONContent =>
	content.length ? { type: 'paragraph', content } : { type: 'paragraph' };
const li = (...content: JSONContent[]): JSONContent => ({ type: 'listItem', content });
const task = (checked: boolean, ...content: JSONContent[]): JSONContent => ({
	type: 'taskItem',
	attrs: { checked },
	content
});
/** A table row of one kind of cell, each cell given its blocks, and the columns' alignment. */
const row = (kind: 'tableHeader' | 'tableCell', cells: JSONContent[][], align?: string[]) => ({
	type: 'tableRow',
	content: cells.map((content, i) =>
		align ? { type: kind, attrs: { align: align[i] }, content } : { type: kind, content }
	)
});
const img = (src: string, alt?: string, title?: string): JSONContent => ({
	type: 'image',
	attrs: { src, alt: alt ?? null, title: title ?? null }
});
const box = (kind: string, ...content: JSONContent[]): JSONContent => ({
	type: 'callout',
	attrs: { kind },
	content
});
const EDITOR: [string, JSONContent[]][] = [
	['empty paragraphs', [p(t('a')), p(), p(), p(t('b'))]],
	['one empty paragraph', [p(t('a')), p(), p(t('b'))]],
	['three empty paragraphs', [p(t('a')), p(), p(), p(), p(t('b'))]],
	['empty paragraph first', [p(), p(t('a'))]],
	['empty paragraph last', [p(t('a')), p()]],
	['leading hash', [p(t('# not a heading'))]],
	['leading dash', [p(t('- not a list'))]],
	['leading number', [p(t('1. not a list'))]],
	['leading angle', [p(t('> not a quote'))]],
	['underscores', [p(t('snake_case_name and *star*'))]],
	['backtick in code', [p(t('a`b', ['code']))]],
	[
		'link with title',
		[p(t('x', [{ type: 'link', attrs: { href: 'https://e.com', title: 'T' } }]))]
	],
	['bold into italic', [p(t('a', ['bold']), t('b', ['bold', 'italic']), t('c', ['italic']))]],
	['spaces in bold', [p(t('x '), t(' bold ', ['bold']), t(' y'))]],
	[
		'break in heading',
		[{ type: 'heading', attrs: { level: 2 }, content: [t('a'), { type: 'hardBreak' }, t('b')] }]
	],
	[
		'code without language',
		[{ type: 'codeBlock', attrs: { language: null }, content: [t('x\n  y')] }]
	],
	['empty code', [{ type: 'codeBlock', attrs: { language: null } }]],
	[
		'list in quote',
		[
			{
				type: 'blockquote',
				content: [{ type: 'bulletList', content: [li(p(t('in quote')))] }, p(t('after'))]
			}
		]
	],
	[
		'loose list',
		[{ type: 'bulletList', content: [li(p(t('one')), p(t('two'))), li(p(t('three')))] }]
	],
	[
		'nested ordered',
		[
			{
				type: 'orderedList',
				attrs: { start: 1 },
				content: [li(p(t('a')), { type: 'orderedList', content: [li(p(t('b')))] })]
			}
		]
	],
	['empty list item', [{ type: 'bulletList', content: [li(p())] }]],
	['tab', [p(t('a\tb'))]],
	['html-looking text', [p(t('<script>alert(1)</script>'))]],
	['strike and code', [p(t('x', ['strike', 'code']))]],
	['underline and bold', [p(t('u', ['underline', 'bold']))]],
	['soft break', [p(t('l1\nl2'))]],
	['only an empty paragraph', [p()]],
	[
		'checklist',
		[
			{
				type: 'taskList',
				content: [
					task(false, p(t('buy milk'))),
					task(true, p(t('call '), t('Dana', ['bold'])), {
						type: 'taskList',
						content: [task(false, p(t('nested')))]
					}),
					task(false, p())
				]
			}
		]
	],
	[
		'two checklists',
		[
			{ type: 'taskList', content: [task(false, p(t('a')))] },
			{ type: 'taskList', content: [task(true, p(t('b')))] }
		]
	],
	[
		'bullets then a checklist',
		[
			{ type: 'bulletList', content: [li(p(t('a')))] },
			{ type: 'taskList', content: [task(false, p(t('b')))] }
		]
	],
	['bracket text in a list', [{ type: 'bulletList', content: [li(p(t('[ ] not a task')))] }]],
	[
		'table',
		[
			{
				type: 'table',
				content: [
					row('tableHeader', [[p(t('Item'))], [p(t('Qty'))]]),
					row('tableCell', [[p(t('Milk'))], [p(t('2'))]]),
					row('tableCell', [[p(t('Eggs', ['bold']))], [p()]])
				]
			}
		]
	],
	[
		'aligned table',
		[
			{
				type: 'table',
				content: [
					row('tableHeader', [[p(t('a'))], [p(t('b'))], [p(t('c'))]], ['left', 'center', 'right']),
					row('tableCell', [[p(t('1'))], [p(t('2'))], [p(t('3'))]], ['left', 'center', 'right'])
				]
			}
		]
	],
	[
		'table with pipes',
		[
			{
				type: 'table',
				content: [
					row('tableHeader', [[p(t('a|b'))], [p(t('c|d', ['code']))]]),
					row('tableCell', [[p(t('x'), { type: 'hardBreak' }, t('y'))], [p(t('*z*'))]])
				]
			}
		]
	],
	[
		'table cell with two paragraphs',
		[
			{
				type: 'table',
				content: [row('tableHeader', [[p(t('h'))]]), row('tableCell', [[p(t('one')), p(t('two'))]])]
			}
		]
	],
	['table without a header row', [{ type: 'table', content: [row('tableCell', [[p(t('x'))]])] }]],
	['image', [p(img('/files/a.png', 'A seedling', 'Spring'))]],
	['image between bold text', [p(t('see ', ['bold']), img('/b.png', 'b'), t(' here', ['bold']))]],
	['image with odd alt and src', [p(img('/a b(1).png', 'a [b] \\ c'))]],
	['image with an empty alt', [p(img('/c.png', ''))]],
	[
		'boxes',
		[
			box('note', p(t('Read this first.'))),
			box('tip', p(t('One')), { type: 'bulletList', content: [li(p(t('two')))] }),
			box('important', p(t('x'))),
			box('warning', p()),
			box('caution', { type: 'blockquote', content: [p(t('quoted'))] })
		]
	],
	['box marker text in a quote', [{ type: 'blockquote', content: [p(t('[!NOTE]')), p(t('x'))] }]],
	[
		'highlights, fonts and sizes',
		[
			p(
				t('a ', [{ type: 'highlight', attrs: { color: 'green' } }]),
				t('b', [{ type: 'highlight', attrs: { color: 'green' } }, 'bold']),
				t(' c '),
				t('serif', [{ type: 'textStyle', attrs: { font: 'source-serif', size: null } }]),
				t(' '),
				t('huge', [{ type: 'textStyle', attrs: { font: null, size: 'huge' } }, 'italic'])
			)
		]
	],
	[
		'columns',
		[
			{
				type: 'columns',
				attrs: { layout: '2:1' },
				content: [
					{
						type: 'column',
						content: [p(t('Sun', ['bold'])), { type: 'bulletList', content: [li(p(t('a')))] }]
					},
					{ type: 'column', content: [p()] }
				]
			},
			{
				type: 'columns',
				attrs: { layout: '1:1:1' },
				content: [
					{ type: 'column', content: [p(t('a'))] },
					{ type: 'column', content: [box('tip', p(t('b')))] },
					{ type: 'column', content: [p(img('/c.png', 'c'))] }
				]
			}
		]
	],
	[
		'folds',
		[
			{
				type: 'fold',
				attrs: { open: true },
				content: [
					{ type: 'foldTitle', content: [t('Soil <mix> & more')] },
					p(t('x')),
					{ type: 'bulletList', content: [li(p(t('y')))] }
				]
			},
			{ type: 'fold', attrs: { open: false }, content: [{ type: 'foldTitle' }, p()] }
		]
	],
	[
		'views and a page style',
		[
			{ type: 'pageStyle', attrs: { font: 'nunito', size: 'large' } },
			{
				type: 'viewEmbed',
				attrs: { id: '7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa', name: 'Garden, this month' }
			},
			p(t('between')),
			{ type: 'viewEmbed', attrs: { id: '7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa', name: '' } },
			{
				type: 'viewEmbed',
				attrs: { id: '7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa', name: 'Board', layout: 'board' }
			},
			{
				type: 'viewEmbed',
				attrs: { id: '7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa', name: 'a *star* [x]' }
			}
		]
	],
	[
		'reports',
		[
			{
				type: 'reportEmbed',
				attrs: { id: '7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa', chart: 'burnup', name: 'Launch' }
			},
			{
				type: 'reportEmbed',
				attrs: { id: '7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa', chart: 'dots', name: '' }
			},
			{
				type: 'reportEmbed',
				attrs: { id: '7f3a91c2-6a0e-4d1b-9c55-2e8b7d10f4aa', chart: 'cfd', name: 'a *star* [x]' }
			}
		]
	],
	[
		'images with a size, side and caption',
		[
			p({
				type: 'image',
				attrs: {
					src: '/b.png',
					alt: 'Beds',
					title: null,
					size: 50,
					side: 'right',
					caption: 'The beds'
				}
			}),
			p({ type: 'image', attrs: { src: '/c.png', alt: 'C', title: null, size: 25, side: 'left' } }),
			p(
				t('see '),
				{
					type: 'image',
					attrs: { src: '/d.png', alt: 'D', title: null, size: 100, side: 'center' }
				},
				t(' here')
			)
		]
	],
	[
		'groceries',
		[
			{ type: 'heading', attrs: { level: 2 }, content: [t('Groceries')] },
			{
				type: 'bulletList',
				content: [li(p(t('Milk'))), li(p(t('Eggs ', []), t('(free range)', ['italic'])))]
			},
			p(
				t('Ask '),
				t('Dana', ['bold']),
				t(' about the '),
				t('recipe', [{ type: 'link', attrs: { href: 'https://example.com/r' } }]),
				t('.')
			)
		]
	]
];

/** The Markdown cases: what Tiptap reads from each. */
export function parseCases() {
	const out: { name: string; md: string; json: JSONContent }[] = [];
	for (const [name, md] of HAND) out.push({ name, md, json: parseMarkdown(md) });
	for (const [name, content] of EDITOR) {
		const md = writeMarkdown({ type: 'doc', content });
		out.push({ name: `written by the editor: ${name}`, md, json: parseMarkdown(md) });
	}
	return out;
}

/**
 * The documents the server writes as Markdown: what the editor makes,
 * with the empty paragraphs Markdown cannot hold left out, since those
 * the server's Markdown drops.
 */
export function renderCases() {
	const out: { name: string; json: JSONContent }[] = [];
	const seen = new Set<string>();
	const add = (name: string, json: JSONContent) => {
		const key = JSON.stringify(json);
		if (seen.has(key)) return;
		seen.add(key);
		out.push({ name, json });
	};
	for (const [name, content] of EDITOR) add(name, { type: 'doc', content });
	for (const c of parseCases()) add(`read from ${c.name}`, c.json);
	return out;
}

/** A headless editor that writes to Yjs as Tiptap's Collaboration extension does. */
class YEditor {
	ydoc = new Y.Doc();
	frag: Y.XmlFragment;
	updates: Uint8Array[] = [];
	doc!: PMNode;
	meta!: ReturnType<typeof initProseMirrorDoc>['meta'];
	constructor(clientID: number) {
		this.ydoc.clientID = clientID;
		this.frag = this.ydoc.getXmlFragment(FRAGMENT);
		this.ydoc.on('update', (u: Uint8Array, origin: unknown) => {
			if (origin !== 'remote') this.updates.push(u);
		});
		this.resync();
	}
	resync() {
		const { doc, meta } = initProseMirrorDoc(this.frag, schema);
		this.doc = doc.content.size ? doc : schema.topNodeType.createAndFill()!;
		this.meta = meta;
	}
	edit(fn: (state: EditorState) => EditorState['tr']) {
		const state = EditorState.create({ doc: this.doc, schema });
		this.doc = fn(state).doc;
		this.ydoc.transact(
			() => updateYFragment(this.ydoc, this.frag, this.doc, this.meta),
			ySyncPluginKey
		);
	}
	apply(update: Uint8Array) {
		Y.applyUpdate(this.ydoc, update, 'remote');
		this.resync();
	}
	setContent(json: JSONContent) {
		this.edit((s) => s.tr.replaceWith(0, s.doc.content.size, schema.nodeFromJSON(json).content));
	}
	insertAt(needle: string, text: string, after = false) {
		this.edit((s) => {
			let at = -1;
			s.doc.descendants((n, pos) => {
				if (at >= 0 || !n.isText) return at < 0;
				const i = n.text!.indexOf(needle);
				if (i >= 0) at = pos + i + (after ? needle.length : 0);
				return false;
			});
			if (at < 0) throw new Error(`no ${needle}`);
			return s.tr.insertText(text, at);
		});
	}
	json() {
		return yXmlFragmentToProsemirrorJSON(this.frag) as JSONContent;
	}
}

const b64 = (u: Uint8Array) => Buffer.from(u).toString('base64');

/** Yjs updates as the editor writes them, alone and from two devices at once, and what they merge to. */
export function yjsCases() {
	const out: { name: string; updates: string[]; json: JSONContent }[] = [];
	const start = parseMarkdown(
		'## Plan\n\nBuy **cedar** boards and *screws*.\n\n- measure\n- cut\n\n```\nlength = 8ft\n```'
	);

	const one = new YEditor(101);
	one.setContent(start);
	one.insertAt('boards', 'long ', false);
	out.push({ name: 'one device', updates: one.updates.map(b64), json: one.json() });

	// Two devices start from the same document, each edits offline, then
	// they meet: both edits stay, including two at the same place.
	const a = new YEditor(201);
	a.setContent(start);
	const b = new YEditor(202);
	for (const u of a.updates) b.apply(u);
	const shared = a.updates.length;
	a.insertAt('Plan', ' for the beds', true);
	b.insertAt('Plan', ' (draft)', true);
	b.insertAt('screws', 'brass ', false);
	a.insertAt('length', 'max ', false);
	for (const u of b.updates.slice(0)) a.apply(u);
	for (const u of a.updates.slice(shared)) b.apply(u);
	if (JSON.stringify(a.json()) !== JSON.stringify(b.json()))
		throw new Error('the two devices did not converge');
	out.push({
		name: 'two devices offline',
		updates: [...a.updates, ...b.updates].map(b64),
		json: a.json()
	});

	// A document emptied and written again.
	const c = new YEditor(301);
	c.setContent(parseMarkdown('First draft.'));
	c.setContent(parseMarkdown('# Rewritten\n\nAll new.'));
	out.push({ name: 'rewritten', updates: c.updates.map(b64), json: c.json() });

	// Every kind of block, then text typed into a table cell.
	const d = new YEditor(401);
	d.setContent(
		parseMarkdown(
			'- [ ] seeds\n- [x] soil\n\n| Bed | Size |\n| :-- | --: |\n| North | 4 |\n\n![Bed](/files/bed.png "North bed")\n\n> [!WARNING]\n> Frost on Friday.\n\n```go\nx := 1\n```'
		)
	);
	d.insertAt('North', ' bed', true);
	out.push({ name: 'every kind of block', updates: d.updates.map(b64), json: d.json() });
	return out;
}

/** Everything in shared/doc-vectors.json except the Markdown the server writes, which the Go tests fill in. */
export function buildVectors() {
	return {
		_comment:
			"Live documents: the editor's answers, from web/src/lib/docs/vectors.ts. parse: Markdown and what Tiptap reads from it. render: documents and the Markdown the server writes for them (md, filled by the Go tests with -update), which Tiptap must read back as the same document. yjs: updates as y-tiptap writes them and the document they merge to. Compare documents after normalizing as y-tiptap stores them: node attributes with defaults and no nulls, mark attributes with defaults, marks by name, no empty content.",
		schema: schemaDefaults(),
		parse: parseCases(),
		render: renderCases(),
		yjs: yjsCases()
	};
}

/**
 * A document as y-tiptap stores it, so two that mean the same compare
 * equal (server/internal/ydoc Normalize does the same): node attributes
 * with defaults and no nulls, mark attributes with defaults, marks by
 * name, adjacent text with the same marks joined, no empty content.
 */
export function normalize(node: JSONContent): JSONContent {
	const defaults = schemaDefaults();
	const out: JSONContent = { type: node.type };
	if (node.type === 'text') {
		out.text = node.text;
		if (node.marks?.length)
			out.marks = node.marks
				.map((m) => {
					const attrs = { ...(defaults.marks[m.type!] ?? {}), ...(m.attrs ?? {}) };
					return Object.keys(attrs).length ? { type: m.type, attrs } : { type: m.type };
				})
				.sort((a, b) => (a.type! < b.type! ? -1 : 1));
		return out;
	}
	if (node.type !== 'doc') {
		const attrs: Record<string, unknown> = {
			...(defaults.nodes[node.type!] ?? {}),
			...(node.attrs ?? {})
		};
		for (const k of Object.keys(attrs)) if (attrs[k] === null) delete attrs[k];
		if (Object.keys(attrs).length) out.attrs = attrs;
	}
	const content: JSONContent[] = [];
	for (const c of node.content ?? []) {
		const n = normalize(c);
		if (n.type === 'text') {
			if (!n.text) continue;
			const last = content.at(-1);
			if (last?.type === 'text' && JSON.stringify(last.marks) === JSON.stringify(n.marks)) {
				last.text += n.text!;
				continue;
			}
		}
		content.push(n);
	}
	if (content.length) out.content = content;
	return out;
}

/** Stable JSON of a normalized document, keys in order. */
export function canonical(node: JSONContent): string {
	const sort = (v: unknown): unknown =>
		Array.isArray(v)
			? v.map(sort)
			: v && typeof v === 'object'
				? Object.fromEntries(
						Object.keys(v as object)
							.sort()
							.map((k) => [k, sort((v as Record<string, unknown>)[k])])
					)
				: v;
	return JSON.stringify(sort(normalize(node)));
}
