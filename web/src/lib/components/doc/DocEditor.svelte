<script lang="ts">
	// A live document in the editor: a task's notes or a wiki page's body,
	// a Yjs document that merges what everyone types (lib/sync/docs.ts),
	// with the others who have it open shown by their faces and cursors.
	// Text from before live documents shows as it is until the server has
	// made its document, which arrives with the next sync. In the Markdown
	// view the document is edited as Markdown, which goes into it as an
	// edit like any other.
	import type { Editor } from '@tiptap/core';
	import { goto } from '$app/navigation';
	import { untrack, type Component } from 'svelte';
	import '../../styles/doctext.css';
	import { DRAFT, REDRAW_PINS } from '../../docs/commentPins.svelte';
	import { linksIn, linkTarget } from '../../links';
	import { openTask } from '../../taskLink';
	import { session } from '../../session.svelte';
	import type { CommentPins } from '../../docs/commentPins.svelte';
	import type { DocPresence, DocTarget, LiveDoc } from '../../sync/docs';
	import { editable, FRAGMENT } from '../../sync/liveNotes';
	import { engine } from '../../sync/engine.svelte';
	import { PALETTE } from '../../theme/palette';
	import Avatar from '../task/Avatar.svelte';

	interface Props {
		ws: string;
		target: DocTarget;
		/** The document as Markdown, rendered by the server. */
		markdown: string;
		/** The editor's name for screen readers: "Notes", "Page". */
		label: string;
		placeholder: string;
		/** A page's body, with a page's type; a task's notes are smaller. */
		page?: boolean;
		/** Shows the document as Markdown, to edit as text. */
		source?: boolean;
		/** Who else has the document open: their user IDs. */
		here?: string[];
		/** Whether the line of faces shows above the editor (a page shows them in its bar). */
		showHere?: boolean;
		/** The comments pinned to its text: a wiki page's. */
		pins?: CommentPins;
		/** Whether the document is empty, once it has drawn. */
		empty?: boolean;
	}

	let {
		ws,
		target,
		markdown,
		label,
		placeholder,
		page = false,
		source = $bindable(false),
		here = $bindable([]),
		showHere = true,
		pins,
		// eslint-disable-next-line no-useless-assignment -- read by the page, through bind:empty
		empty = $bindable(false)
	}: Props = $props();

	/** Fills an empty document with Markdown, as an edit; false if it holds anything. */
	export function fill(md: string): boolean {
		if (!editor || editor.isDestroyed || !editor.isEmpty) return false;
		return editor.commands.setContent(md, { contentType: 'markdown' });
	}

	/** The document as it stands, as Markdown; null until it has drawn. */
	export function currentMarkdown(): string | null {
		return editor && !editor.isDestroyed ? editor.getMarkdown() : null;
	}

	/** Empties the document, as an edit. */
	export function clear() {
		if (editor && !editor.isDestroyed) editor.commands.clearContent();
	}

	/** The page's own font and size, which its first block keeps (docs/formatting.ts). */
	let style = $state<{ font: string | null; size: string | null }>({ font: null, size: null });
	export function pageStyle() {
		return style;
	}

	/** Gives the page its own font and size, or takes them away, as an edit. */
	export function setPageStyle(font: string | null, size: string | null) {
		const ed = editor;
		if (!ed || ed.isDestroyed) return;
		const { state } = ed;
		let at = -1;
		state.doc.forEach((n, pos) => {
			if (at < 0 && n.type.name === 'pageStyle') at = pos;
		});
		const tr = state.tr;
		if (!font && !size) {
			if (at >= 0) tr.delete(at, at + 1);
		} else if (at >= 0) tr.setNodeMarkup(at, undefined, { font, size });
		else tr.insert(0, state.schema.nodes.pageStyle.create({ font, size }));
		ed.view.dispatch(tr);
	}
	let stacks: Record<string, string> = {};
	let pageScale: Record<string, number> = {};
	/** The first page style in a document, if it has one. */
	function styleOf(ed: Editor) {
		let found: { font: string | null; size: string | null } = { font: null, size: null };
		let seen = false;
		ed.state.doc.forEach((n) => {
			if (!seen && n.type.name === 'pageStyle') {
				seen = true;
				found = { font: n.attrs.font ?? null, size: n.attrs.size ?? null };
			}
		});
		if (found.font !== style.font || found.size !== style.size) style = found;
	}
	const docFont = $derived(style.font ? stacks[style.font] : undefined);
	const docSize = $derived(style.size ? String(pageScale[style.size] ?? 1) : undefined);
	// By value: a parent that passes a new object for the same document
	// must not rebuild the editor under someone typing in it.
	const kind = $derived(target.kind);
	const docId = $derived(target.id);
	const rows = $derived.by(() => {
		const r = engine.replica(ws);
		return (kind === 'task' ? r?.docUpdates(docId) : r?.pageUpdates(docId)) ?? [];
	});
	const live = $derived(editable(markdown, rows));
	let element = $state<HTMLDivElement>();
	let frame = $state<HTMLDivElement>();
	let doc: LiveDoc | undefined;
	let editor: Editor | undefined;
	// The editor's tools (the formatting bar, the / menu, images), loaded
	// with it; version follows the editor's state for them.
	let shownEditor = $state.raw<Editor>();
	let Tools =
		$state.raw<Component<Record<string, unknown>, { keydown(e: KeyboardEvent): boolean }>>();
	let tools = $state<{ keydown(e: KeyboardEvent): boolean }>();
	let version = $state(0);
	let focused = $state(false);
	const people = $derived(
		here.filter((id) => id !== session.user?.id).map((id) => ({ id, ...session.personOf(ws, id) }))
	);

	/** What to say after a link to a task or page: the task's status, or where it went. */
	function noteFor(href: string): string | null {
		const t = linkTarget(href);
		const r = engine.replica(ws);
		if (!t || !r) return null;
		const row = r.get(t.kind, t.id);
		if (!row) return 'Not available here';
		if (row.values.deleted_at != null) return 'In the trash';
		if (t.kind === 'page') return null;
		const task = r.task(t.id);
		return (task?.statusId && r.status(task.statusId)?.name) || (task?.completedAt ? 'Done' : null);
	}

	/** Follows a link: a task opens beside the page, a page in place, anything else in a new tab. */
	function follow(href: string) {
		const t = linkTarget(href);
		if (t?.kind === 'task') void openTask(t.id);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- a page's address, made by pageHref
		else if (t) void goto(href);
		else window.open(href, '_blank', 'noopener,noreferrer');
	}

	// The notes after links follow what they point at: when a linked task
	// changes status, or a page goes to the trash, they are drawn again.
	const notes = $derived(
		linksIn(markdown)
			.map((id) => `${id}:${noteFor(`/?task=${id}`) ?? ''}:${noteFor(`/wiki?page=${id}`) ?? ''}`)
			.join()
	);

	/** A person's color for their cursor: one of the palette, the same everywhere. */
	function colorOf(id: string): string {
		let h = 0;
		for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
		return PALETTE[h % PALETTE.length];
	}

	/** Another person's cursor: a line in their color with their name. */
	function caret(user: Record<string, unknown>): HTMLElement {
		const id = typeof user.id === 'string' ? user.id : '';
		const el = document.createElement('span');
		el.className = 'peer-caret';
		// A name in the middle of the text would be read as if typed; the
		// faces say who is here.
		el.setAttribute('aria-hidden', 'true');
		// One's own cursor in another window is not drawn.
		if (id && id === session.user?.id) {
			el.hidden = true;
			return el;
		}
		el.style.setProperty('--peer', `var(--palette-${colorOf(id)})`);
		const name = document.createElement('span');
		name.className = 'peer-name';
		name.textContent = id ? session.personOf(ws, id).name : 'Someone';
		el.append(name);
		return el;
	}

	// The editor loads only when a document opens, so it never weighs on
	// the app's first load. One document per task or page: another builds
	// another, and closing sends what was typed.
	$effect(() => {
		if (!element || !live) return;
		const el = element;
		const t: DocTarget = { kind, id: docId };
		let d: LiveDoc | undefined;
		let presence: DocPresence | undefined;
		let made: Editor | undefined;
		let destroyed = false;
		Promise.all([
			import('@tiptap/core'),
			import('../../docs/extensions'),
			import('@tiptap/extensions'),
			import('@tiptap/markdown'),
			import('@tiptap/extension-collaboration'),
			import('@tiptap/extension-collaboration-caret'),
			import('../../sync/docs'),
			import('./DocTools.svelte'),
			import('../../docs/linkview'),
			import('../../docs/commentMarks'),
			import('../../docs/nodeviews.svelte'),
			import('../../docs/formatting'),
			import('../../docs/diagrams')
		]).then(
			([
				{ Editor },
				{ docExtensions },
				{ Placeholder },
				{ Markdown },
				{ default: Collaboration },
				{ default: CollaborationCaret },
				{ LiveDoc, DocPresence },
				{ default: DocTools },
				{ LinkView },
				{ CommentMarks, pinSelection, revealMark },
				{
					foldView,
					FoldReveal,
					imageView,
					viewEmbedView,
					reportEmbedView,
					codeBlockView,
					drawingEmbedView
				},
				{ FONT_STACKS, PAGE_SCALE },
				{ DiagramEditing }
			]) => {
				if (destroyed) return;
				stacks = FONT_STACKS;
				pageScale = PAGE_SCALE;
				d = doc = new LiveDoc(ws, t);
				d.sync(untrack(() => rows));
				presence = new DocPresence(ws, d);
				const p = presence;
				p.awareness.on('change', () => (here = p.others()));
				made = editor = new Editor({
					element: el,
					extensions: [
						// Yjs keeps the undo history, one person's typing at a time.
						...docExtensions({
							nodeViews: {
								fold: foldView,
								image: imageView,
								viewEmbed: viewEmbedView(ws),
								reportEmbed: reportEmbedView(ws),
								codeBlock: codeBlockView,
								drawingEmbed: drawingEmbedView(ws, kind === 'page' ? docId : null)
							}
						}),
						FoldReveal,
						DiagramEditing,
						LinkView.configure({ noteFor, follow }),
						CommentMarks.configure({ pins: untrack(() => pins) ?? null }),
						Markdown,
						Placeholder.configure({ placeholder }),
						Collaboration.configure({ document: d.ydoc, field: FRAGMENT }),
						CollaborationCaret.configure({
							provider: { awareness: p.awareness },
							user: { id: session.user?.id ?? '' },
							render: caret,
							selectionRender: (user) => {
								const id = typeof user.id === 'string' ? user.id : '';
								return id && id === session.user?.id
									? {}
									: {
											class: 'peer-selection',
											style: `--peer: var(--palette-${colorOf(id)}-soft)`
										};
							}
						})
					],
					editorProps: {
						attributes: {
							'aria-label': label,
							role: 'textbox',
							'aria-multiline': 'true',
							class: 'doc-body'
						},
						handleKeyDown: (_view, e) => tools?.keydown(e) ?? false
					},
					onTransaction: ({ editor: ed }) => {
						version++;
						empty = ed.isEmpty;
						styleOf(ed);
					},
					onCreate: ({ editor: ed }) => styleOf(ed),
					onFocus: () => (focused = true),
					onBlur: () => (focused = false)
				});
				Tools = DocTools as never;
				shownEditor = made;
				const ed = made;
				const held = untrack(() => pins);
				if (held) {
					held.begin = () => {
						const pin = pinSelection(ed.state);
						if (!pin) return false;
						held.draft = pin;
						held.active = DRAFT;
						held.onshow();
						return true;
					};
					held.reveal = (id) => revealMark(ed.view, id);
				}
			}
		);
		return () => {
			destroyed = true;
			empty = false;
			const held = untrack(() => pins);
			if (held) {
				held.begin = null;
				held.reveal = null;
			}
			if (editor === made) editor = undefined;
			if (shownEditor === made) shownEditor = undefined;
			focused = false;
			made?.destroy();
			presence?.close();
			here = [];
			if (doc === d) doc = undefined;
			void d?.close();
		};
	});

	// The highlights follow the threads, and which of them is picked.
	$effect(() => {
		void pins?.pins;
		void pins?.active;
		void pins?.draft;
		untrack(() => {
			const ed = shownEditor;
			if (pins && ed && !ed.isDestroyed) ed.view.dispatch(ed.state.tr.setMeta(REDRAW_PINS, true));
		});
	});

	$effect(() => {
		void notes;
		untrack(() => {
			const ed = shownEditor;
			if (ed && !ed.isDestroyed) ed.view.dispatch(ed.state.tr.setMeta('linkNotes', true));
		});
	});

	// Rows from other devices, and the server's merges of them.
	$effect(() => {
		const r = rows;
		untrack(() => doc?.sync(r));
	});

	// Text waiting for its document, shown as it is.
	$effect(() => {
		if (!element || live) return;
		const el = element;
		const md = markdown;
		let made: Editor | undefined;
		let destroyed = false;
		Promise.all([
			import('@tiptap/core'),
			import('../../docs/extensions'),
			import('@tiptap/markdown'),
			import('../../docs/linkview'),
			import('../../docs/nodeviews.svelte'),
			import('../../docs/formatting')
		]).then(([{ Editor }, { docExtensions }, { Markdown }, { LinkView }, views, formatting]) => {
			if (destroyed) return;
			stacks = formatting.FONT_STACKS;
			pageScale = formatting.PAGE_SCALE;
			made = new Editor({
				element: el,
				onCreate: ({ editor: ed }) => styleOf(ed),
				extensions: [
					...docExtensions({
						undo: true,
						nodeViews: {
							fold: views.foldView,
							image: views.imageView,
							viewEmbed: views.viewEmbedView(ws),
							reportEmbed: views.reportEmbedView(ws),
							codeBlock: views.codeBlockView,
							drawingEmbed: views.drawingEmbedView(ws, kind === 'page' ? docId : null)
						}
					}),
					LinkView.configure({ noteFor, follow }),
					Markdown
				],
				content: md,
				contentType: 'markdown',
				editable: false,
				editorProps: {
					attributes: { 'aria-label': label, 'aria-readonly': 'true', class: 'doc-body' }
				}
			});
		});
		return () => {
			destroyed = true;
			made?.destroy();
		};
	});

	// The Markdown view: the document as Markdown, from the editor as it
	// is; what is changed goes back into the document when the view closes
	// or typing stops, and nothing does if nothing changed.
	let text = $state('');
	let shown = '';
	let timer: ReturnType<typeof setTimeout> | undefined;
	function apply() {
		clearTimeout(timer);
		if (!editor || text === shown) return;
		shown = text;
		editor.commands.setContent(text, { contentType: 'markdown' });
	}
	$effect(() => {
		if (!source) return;
		untrack(() => {
			// Without the blank lines the editor's empty last paragraph leaves.
			text = shown = (editor?.getMarkdown() ?? markdown).replace(/\n+$/, '');
		});
		return () => apply();
	});
</script>

{#if showHere && people.length}
	<p class="here">
		<span class="faces" aria-hidden="true">
			{#each people as p (p.id)}<Avatar name={p.name} src={p.avatar} size={20} />{/each}
		</span>
		{people.map((p) => p.name).join(' and ')}
		{people.length === 1 ? 'is' : 'are'} here
	</p>
{/if}
<div class="frame" bind:this={frame}>
	<div
		class="doc doc-text"
		class:paper={page}
		class:hidden={source}
		style:--doc-font={docFont}
		style:--doc-heading-font={docFont}
		style:--doc-size={docSize}
		bind:this={element}
	></div>
	{#if Tools && shownEditor && frame && !source}
		<Tools
			bind:this={tools}
			{ws}
			{target}
			editor={shownEditor}
			{version}
			{focused}
			{frame}
			oncomment={pins ? () => pins.begin?.() : undefined}
		/>
	{/if}
</div>
{#if source}
	<textarea
		class="source"
		class:paper={page}
		aria-label={`${label} as Markdown`}
		spellcheck="false"
		bind:value={text}
		disabled={!live}
		oninput={() => {
			clearTimeout(timer);
			timer = setTimeout(apply, 800);
		}}
		onblur={apply}></textarea>
{/if}
{#if !live}
	<p class="waiting">This becomes editable once this device has synced.</p>
{/if}

<style>
	/* A document's own type: the theme's page font, or
	   the page's own. Its Markdown (headings, links, lists, code, quotes, tables)
	   is styles/doctext.css, which the theme preview shares. */
	.doc {
		container-type: inline-size;
	}
	.doc :global(.doc-body) {
		min-height: 80px;
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid transparent;
		border-radius: var(--radius-md);
		line-height: 1.55;
		outline: none;
	}
	.doc :global(.doc-body:hover) {
		border-color: var(--border);
	}
	.doc :global(.doc-body:focus) {
		border-color: var(--accent);
	}
	.doc :global(.doc-body[aria-readonly='true']:hover) {
		border-color: transparent;
	}
	.here {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: 0 var(--space-3) var(--space-1);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.faces {
		display: inline-flex;
	}
	.faces > :global(* + *) {
		margin-left: -5px;
	}
	/* Someone else's cursor and selection, in their color. */
	.doc :global(.peer-caret) {
		position: relative;
		margin: 0 -1px;
		border-left: 2px solid var(--peer);
		word-break: normal;
		pointer-events: none;
	}
	.doc :global(.peer-name) {
		position: absolute;
		left: -2px;
		bottom: 100%;
		padding: 0 4px;
		border-radius: var(--radius-sm) var(--radius-sm) var(--radius-sm) 0;
		background: var(--peer);
		color: var(--surface-0);
		font-size: var(--text-xs);
		font-weight: 600;
		line-height: 1.4;
		white-space: nowrap;
		user-select: none;
	}
	.doc :global(.peer-selection) {
		background: var(--peer);
	}
	.doc :global(.comment-mark) {
		background: var(--comment-mark);
		border-bottom: 2px solid var(--comment-mark-edge);
		cursor: pointer;
	}
	.doc :global(.comment-mark.active) {
		background: var(--comment-mark-active);
	}
	.hidden {
		display: none;
	}
	.source {
		display: block;
		width: 100%;
		min-height: 240px;
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--radius-md);
		background: var(--surface-1);
		color: var(--text-primary);
		font-family: var(--font-mono);
		font-size: var(--text-sm);
		line-height: 1.6;
		resize: vertical;
		field-sizing: content;
	}
	.source:focus-visible {
		border-color: var(--accent);
		outline: none;
	}
	/* A page reads as a document: larger type and room around it. */
	.doc.paper :global(.doc-body) {
		min-height: 50vh;
		padding: var(--space-2) 0;
		border: none;
		font-size: calc(var(--type-page) * var(--ui-scale) * var(--doc-size, 1));
		line-height: var(--leading-page);
	}
	.doc.paper :global(.doc-body:focus) {
		outline: none;
	}
	/* A chart's own headings, in a report in the page, stay its own size. */
	.doc :global(.report-embed h2) {
		font-size: var(--text-md);
		font-family: var(--font-ui);
		margin: 0;
	}
	.doc.paper :global(.doc-body > * + *) {
		margin-top: 0.8em;
	}
	.waiting {
		padding: 0 var(--space-3);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.doc :global(.doc-body > * + *) {
		margin-top: 0.6em;
	}
	/* A heading starts a new part, so it has more room above it than a paragraph. */
	.doc :global(.doc-body > :is(h1, h2, h3, h4, h5, h6):not(:first-child)) {
		margin-top: 1.1em;
	}
	.doc :global(p.is-editor-empty:first-child::before) {
		content: attr(data-placeholder);
		float: left;
		height: 0;
		color: var(--text-muted);
		pointer-events: none;
	}
	/* What a link to a task or page points at now: its status, or the trash. */
	.doc :global(.link-note) {
		margin-left: 0.3em;
		padding: 0 0.4em;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		color: var(--text-secondary);
		font-size: 0.8em;
		white-space: nowrap;
		user-select: none;
	}
	/* The bar and the / menu are placed inside this. */
	.frame {
		position: relative;
	}
	/* Checklists: the box beside its text. */
	.doc :global(ul[data-type='taskList']) {
		padding-left: 0.2em;
		list-style: none;
	}
	.doc :global(ul[data-type='taskList'] > li) {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
	}
	.doc :global(ul[data-type='taskList'] > li > label) {
		display: flex;
		flex: none;
	}
	.doc :global(ul[data-type='taskList'] > li) {
		padding: 2px 0;
	}
	.doc :global(ul[data-type='taskList'] > li > label input) {
		/* At least 24px, the smallest target WCAG allows here. */
		width: max(var(--tap-min), 24px);
		height: max(var(--tap-min), 24px);
		margin: 0;
		accent-color: var(--accent);
	}
	.doc :global(ul[data-type='taskList'] > li > div) {
		flex: 1;
		min-width: 0;
	}
	.doc :global(ul[data-type='taskList'] > li[data-checked='true'] > div > p) {
		color: var(--text-secondary);
		text-decoration: line-through;
	}
	/* Tables are styles/doctext.css's; a selected cell is the editor's. */
	.doc :global(.selectedCell) {
		background: var(--accent-soft);
	}
	/* Images: inline, never wider than the text; given a size, a share of its width,
	   and given a side, with text beside it. Full width where there's no room. */
	.doc :global(img) {
		max-width: 100%;
		height: auto;
		border-radius: var(--radius-md);
		vertical-align: bottom;
	}
	.doc :global(img.ProseMirror-selectednode),
	.doc :global(.doc-image.ProseMirror-selectednode img) {
		outline: var(--focus-ring-width) solid var(--focus-ring-color);
	}
	.doc :global(.doc-image) {
		display: inline-block;
		max-width: 100%;
		vertical-align: bottom;
	}
	.doc :global(.doc-image[data-size]) {
		display: block;
	}
	.doc :global(.doc-image[data-size] img) {
		width: 100%;
	}
	.doc :global(.doc-image[data-size='25']) {
		width: 25%;
	}
	.doc :global(.doc-image[data-size='50']) {
		width: 50%;
	}
	.doc :global(.doc-image[data-size='75']) {
		width: 75%;
	}
	.doc :global(.doc-image[data-side='center']) {
		margin-inline: auto;
	}
	.doc :global(.doc-image[data-side='left']) {
		float: left;
		margin: 0.2em var(--space-4) var(--space-2) 0;
	}
	.doc :global(.doc-image[data-side='right']) {
		float: right;
		margin: 0.2em 0 var(--space-2) var(--space-4);
	}
	.doc :global(.doc-image-caption) {
		display: block;
		margin-top: var(--space-1);
		font-size: 0.85em;
		color: var(--text-secondary);
	}
	.doc :global(.doc-body::after) {
		content: '';
		display: block;
		clear: both;
	}
	/* Highlights, in the theme's five colors. */
	.doc :global(mark.highlight) {
		padding: 0 0.1em;
		border-radius: var(--radius-sm);
		background: var(--highlight-yellow);
		color: inherit;
	}
	@media (forced-colors: active) {
		.doc :global(mark.highlight) {
			background: Mark;
			color: MarkText;
		}
	}
	/* Words in another size, measured from the text around them. */
	.doc :global(.text-style[data-size='small']) {
		font-size: 0.85em;
	}
	.doc :global(.text-style[data-size='large']) {
		font-size: 1.3em;
	}
	.doc :global(.text-style[data-size='huge']) {
		font-size: 1.7em;
		line-height: 1.25;
	}
	/* Columns: side by side where there's room, one under another where there isn't. */
	.doc :global(.columns) {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
		gap: var(--space-4);
	}
	.doc :global(.columns[data-layout='2:1']) {
		grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
	}
	.doc :global(.columns[data-layout='1:2']) {
		grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
	}
	.doc :global(.columns[data-layout='1:1:1']) {
		grid-template-columns: repeat(3, minmax(0, 1fr));
	}
	.doc :global(.column) {
		min-width: 0;
	}
	.doc :global(.column > * + *) {
		margin-top: 0.6em;
	}
	/* While it can be edited, a column shows where it is. */
	.doc :global(.doc-body[contenteditable='true'] .column) {
		padding: var(--space-1) var(--space-2);
		border: var(--border-width) dashed var(--border);
		border-radius: var(--radius-sm);
	}
	@container (max-width: 520px) {
		.doc :global(.columns),
		.doc :global(.columns[data-layout]) {
			grid-template-columns: minmax(0, 1fr);
		}
		.doc :global(.doc-image[data-size]) {
			width: 100%;
			float: none;
			margin: 0;
		}
	}
	/* Foldable sections: a button, the title, and what's under it while open. */
	.doc :global(.fold) {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		align-items: start;
		gap: 0 var(--space-1);
	}
	.doc :global(.fold-toggle) {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: max(var(--tap-min), 1.6em);
		height: max(var(--tap-min), 1.6em);
		padding: 0;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.doc :global(.fold-toggle:hover) {
		background: var(--surface-2);
	}
	.doc :global(.fold-toggle svg) {
		transition: transform var(--dur-micro) var(--ease);
	}
	.doc :global(.fold.open > .fold-toggle svg) {
		transform: rotate(90deg);
	}
	.doc :global(.fold-title) {
		font-weight: 600;
		min-height: 1.6em;
	}
	.doc :global(.fold-content > * + *) {
		margin-top: 0.5em;
	}
	.doc :global(.fold:not(.open) > .fold-content > :not(.fold-title)) {
		display: none;
	}
	.doc :global(.view-embed),
	.doc :global(.report-embed) {
		margin: 0.2em 0;
	}
	/* A diagram: its drawing, and its source while the caret is in it, under
	   the drawing, or beside it where the block is wide enough. */
	.doc :global(.diagram-block) {
		container-type: inline-size;
		margin: 0.2em 0;
	}
	.doc :global(.diagram-inner) {
		display: grid;
		gap: var(--space-2);
	}
	.doc :global(.diagram-source) {
		display: none;
		margin: 0;
	}
	.doc :global(.diagram-block.editing .diagram-source) {
		display: block;
	}
	@container (min-width: 720px) {
		.doc :global(.diagram-block.editing .diagram-inner) {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
			align-items: start;
		}
		.doc :global(.diagram-block.editing .diagram-source) {
			grid-row: 1;
			grid-column: 1;
		}
	}
	.doc :global(.drawing-embed) {
		margin: 0.2em 0;
	}
	.doc :global(.drawing-embed.ProseMirror-selectednode) {
		outline: var(--focus-ring-width) solid var(--focus-ring-color);
		border-radius: var(--radius-md);
	}
	.doc :global(.diagram-block.ProseMirror-selectednode),
	.doc :global(.view-embed.ProseMirror-selectednode),
	.doc :global(.report-embed.ProseMirror-selectednode) {
		outline: var(--focus-ring-width) solid var(--focus-ring-color);
		border-radius: var(--radius-md);
	}
	/* Boxes: GitHub's note, tip, important, warning and caution. */
	.doc :global(aside.callout) {
		--box: var(--palette-blue);
		--box-soft: var(--palette-blue-soft);
		padding: var(--space-2) var(--space-3);
		border-left: 3px solid var(--box);
		border-radius: var(--radius-sm);
		background: var(--box-soft);
	}
	.doc :global(aside.callout::before) {
		content: attr(aria-label);
		display: block;
		margin-bottom: var(--space-1);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.doc :global(aside.callout[data-kind='tip']) {
		--box: var(--palette-green);
		--box-soft: var(--palette-green-soft);
	}
	.doc :global(aside.callout[data-kind='important']) {
		--box: var(--palette-purple);
		--box-soft: var(--palette-purple-soft);
	}
	.doc :global(aside.callout[data-kind='warning']) {
		--box: var(--palette-amber);
		--box-soft: var(--palette-amber-soft);
	}
	.doc :global(aside.callout[data-kind='caution']) {
		--box: var(--palette-red);
		--box-soft: var(--palette-red-soft);
	}
	.doc :global(aside.callout > * + *) {
		margin-top: 0.5em;
	}
	/* Code: highlighted with the text's own colors, so it reads in every theme. */
	.doc :global(.hljs-keyword),
	.doc :global(.hljs-built_in),
	.doc :global(.hljs-literal),
	.doc :global(.hljs-number) {
		color: var(--accent-text);
	}
	.doc :global(.hljs-title),
	.doc :global(.hljs-section),
	.doc :global(.hljs-type) {
		font-weight: 600;
	}
	.doc :global(.hljs-string),
	.doc :global(.hljs-regexp) {
		color: var(--text-secondary);
	}
	.doc :global(.hljs-comment),
	.doc :global(.hljs-quote) {
		color: var(--text-secondary);
		font-style: italic;
	}
</style>
