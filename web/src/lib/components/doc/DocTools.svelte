<script lang="ts">
	// The editor's tools, loaded with the editor:
	// the formatting bar over selected text (and in a table, the table's
	// actions), which Alt+F10 reaches from the keyboard; the `/` menu at
	// the start of a line and the `[[` menu of links; and the image picker. DocEditor passes key
	// presses in the text through keydown() first.
	import ALargeSmall from '@lucide/svelte/icons/a-large-small';
	import Bold from '@lucide/svelte/icons/bold';
	import Captions from '@lucide/svelte/icons/captions';
	import ChevronsDownUp from '@lucide/svelte/icons/chevrons-down-up';
	import Columns2 from '@lucide/svelte/icons/columns-2';
	import Highlighter from '@lucide/svelte/icons/highlighter';
	import ListCollapse from '@lucide/svelte/icons/list-collapse';
	import Scaling from '@lucide/svelte/icons/scaling';
	import TextAlignCenter from '@lucide/svelte/icons/text-align-center';
	import Type from '@lucide/svelte/icons/type';
	import Ungroup from '@lucide/svelte/icons/ungroup';
	import { Fragment } from '@tiptap/pm/model';
	import BetweenHorizontalEnd from '@lucide/svelte/icons/between-horizontal-end';
	import BetweenVerticalEnd from '@lucide/svelte/icons/between-vertical-end';
	import Code from '@lucide/svelte/icons/code';
	import Heading from '@lucide/svelte/icons/heading';
	import Italic from '@lucide/svelte/icons/italic';
	import Link from '@lucide/svelte/icons/link';
	import Strikethrough from '@lucide/svelte/icons/strikethrough';
	import TableCellsMerge from '@lucide/svelte/icons/table-cells-merge';
	import TableColumnsSplit from '@lucide/svelte/icons/table-columns-split';
	import TableRowsSplit from '@lucide/svelte/icons/table-rows-split';
	import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
	import ChartLine from '@lucide/svelte/icons/chart-line';
	import { REPORT_CHARTS, chartOf, type ReportTab } from '../../reportKinds';
	import { viewReport } from '../../reportSetup.svelte';
	import type { Editor } from '@tiptap/core';
	import type { Component } from 'svelte';
	import { tick, untrack } from 'svelte';
	import MessageSquarePlus from '@lucide/svelte/icons/message-square-plus';
	import { announce } from '../../announce.svelte';
	import { keymap } from '../../keys/keymap.svelte';
	import { findBlocks, insertDiagram, slashAt, type Block } from '../../docs/blocks';
	import type { Starter } from '../../docs/diagrams';
	import { linkAt, linkChoices } from '../../docs/mentions';
	import { createPage } from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import { wikiMain, wikiOfMain } from '../../sync/pages';
	import { toasts } from '../../toasts.svelte';
	import { pageHref } from '../../wiki';
	import type { DocTarget } from '../../sync/docs';
	import ImagePicker from './ImagePicker.svelte';
	import DiagramPicker from './DiagramPicker.svelte';
	import ViewPicker from './ViewPicker.svelte';
	import {
		FONT_NAMES,
		FONT_STACKS,
		HIGHLIGHT_COLORS,
		IMAGE_SIDES,
		IMAGE_SIZES,
		PAGE_FONTS,
		WORD_SIZES
	} from '../../docs/formatting';

	interface Props {
		ws: string;
		target: DocTarget;
		editor: Editor;
		/** Changes with every change to the editor's state, so what is shown follows it. */
		version: number;
		/** Whether the text has focus. */
		focused: boolean;
		/** The box the bar and menu are placed in. */
		frame: HTMLElement;
		/** Begins a comment on the selected words: a wiki page's. */
		oncomment?: () => void;
	}

	let { ws, target, editor, version, focused, frame, oncomment }: Props = $props();
	const uid = $props.id();

	// The menus in the text: `/` at the start of a line inserts a block,
	// `[[` links to a page or task. Each stays open until Escape.
	type Item = { id: string; label: string; detail?: string; choose: () => void };
	const slash = $derived.by(() => {
		void version;
		return focused ? slashAt(editor) : null;
	});
	const mention = $derived.by(() => {
		void version;
		return focused && !slash ? linkAt(editor) : null;
	});
	const at = $derived(slash ?? mention);
	const kind = $derived(slash ? 'blocks' : 'links');
	const replica = $derived(engine.replica(ws));
	let dismissed = $state<number | null>(null);
	const items = $derived.by((): Item[] => {
		if (slash)
			return (
				findBlocks(slash.query)
					// Columns don't go inside columns.
					.filter((b) => !(b.id.startsWith('columns') && editor.isActive('column')))
					.map((b) => ({
						id: b.id,
						label: b.label,
						choose: () => insertBlock(b)
					}))
			);
		if (!mention || !replica) return [];
		const r = replica;
		const found = linkChoices(
			ws,
			mention.query,
			r.allPages(),
			r.allTasks(),
			(id) => (id ? r.project(id) : undefined),
			target.id
		).map((c): Item => ({
			id: `${c.kind}-${c.id}`,
			label: c.title,
			detail: `${c.kind === 'page' ? 'Page' : 'Task'} in ${c.where}`,
			choose: () => insertLink(c.title, c.href)
		}));
		const title = mention.query.trim();
		if (title)
			found.push({
				id: 'new-page',
				label: `New page “${title}”`,
				detail: target.kind === 'page' ? 'Under this page' : "In the project's wiki",
				choose: () => void linkNewPage(title)
			});
		return found;
	});
	const menuOpen = $derived(!!at && at.from !== dismissed && items.length > 0);
	let active = $state(0);
	$effect(() => {
		if (!at) dismissed = null;
	});
	// The list is made again on every change to the editor's state, a save
	// or someone's cursor included; the choice goes back to the top only
	// when what the list holds changes.
	const listKey = $derived(`${kind}\n${items.map((i) => i.id).join('\n')}`);
	$effect(() => {
		void listKey;
		active = 0;
	});
	const optionId = (i: number) => `${uid}-item-${i}`;
	const menuName = $derived(kind === 'blocks' ? 'Insert a block' : 'Link to a page or task');

	// What the text says about the menu, for screen readers.
	$effect(() => {
		const dom = editor.view.dom;
		if (menuOpen) {
			dom.setAttribute('aria-controls', `${uid}-menu`);
			dom.setAttribute('aria-activedescendant', optionId(active));
		} else {
			dom.removeAttribute('aria-controls');
			dom.removeAttribute('aria-activedescendant');
		}
	});
	// The chosen item stays in view as the arrows move past the menu's edge.
	$effect(() => {
		if (menuOpen) document.getElementById(optionId(active))?.scrollIntoView({ block: 'nearest' });
	});
	let wasOpen = false;
	$effect(() => {
		if (menuOpen && !wasOpen) {
			const what = kind === 'blocks' ? 'blocks to insert' : 'pages and tasks to link';
			announce(`${items.length} ${what}. ${items[active]?.label}`);
		}
		wasOpen = menuOpen;
	});

	let picking = $state(false);
	/** Takes out the `/` or `[[` typed, and what followed it. */
	function clearTyped() {
		if (at) editor.chain().focus().deleteRange({ from: at.from, to: at.to }).run();
	}
	function insertBlock(block: Block) {
		clearTyped();
		if (block.insert) block.insert(editor);
		else if (block.id === 'image') picking = true;
		else if (block.id === 'view') pickingView = true;
		else if (block.id === 'report') pickingReport = true;
		else if (block.id === 'diagram') pickingDiagram = true;
		announce(
			block.id === 'image'
				? 'Choose an image'
				: block.id === 'view'
					? 'Choose a view'
					: block.id === 'report'
						? 'Choose a report'
						: block.id === 'diagram'
							? 'Choose a kind of diagram'
							: `Inserted ${block.label.toLowerCase()}`
		);
	}
	function insertLink(title: string, href: string) {
		clearTyped();
		editor
			.chain()
			.focus()
			.insertContent([
				{ type: 'text', text: title, marks: [{ type: 'link', attrs: { href } }] },
				{ type: 'text', text: ' ' }
			])
			.run();
		announce(`Linked to ${title}`);
	}
	/** A new subpage, linked: under this page, or in the task's project's wiki. */
	async function linkNewPage(title: string) {
		const r = replica;
		if (!r) return;
		let projectId: string | null;
		let parentId: string;
		if (target.kind === 'page') {
			// A wiki's main page may not be saved on this device yet; its ID
			// says whose it is, and making the subpage makes it too.
			const page = r.page(target.id);
			const wiki = page
				? page.projectId
				: wikiOfMain(
						ws,
						target.id,
						r.projects().map((p) => p.id)
					);
			if (wiki === undefined) {
				toasts.error('This page is not on this device yet, so a page cannot go under it.');
				return;
			}
			projectId = wiki;
			parentId = target.id;
		} else {
			projectId = r.task(target.id)?.projectId ?? null;
			parentId = wikiMain(ws, projectId);
		}
		const id = await createPage(ws, projectId, parentId, title);
		insertLink(title, pageHref({ id, projectId }));
	}

	function insertImage(src: string, alt: string) {
		editor.chain().focus().insertContent({ type: 'image', attrs: { src, alt } }).run();
	}
	let pickingView = $state(false);
	function insertView(id: string, name: string) {
		editor.chain().focus().insertContent({ type: 'viewEmbed', attrs: { id, name } }).run();
		announce(`Inserted the view ${name}`);
	}
	let pickingDiagram = $state(false);
	function insertStarter(starter: Starter) {
		editor.commands.focus();
		insertDiagram(editor, starter.source);
		announce(`Inserted ${starter.label.toLowerCase()}`);
	}
	// A report shows the chart its view opens on first; Chart changes it.
	const FIRST_CHART: Record<ReportTab, string> = {
		status: 'burnup',
		workload: 'by-person',
		flow: 'made-done'
	};
	let pickingReport = $state(false);
	function insertReport(id: string, name: string) {
		const view = replica?.view(id);
		const chart = FIRST_CHART[view ? viewReport(view).tab : 'status'];
		editor.chain().focus().insertContent({ type: 'reportEmbed', attrs: { id, name, chart } }).run();
		announce(`Inserted ${chartOf(chart)?.label ?? 'a chart'} from ${name}`);
	}

	/** The nearest block of a kind around the caret, and where it starts. */
	function around(name: string) {
		const from = editor.state.selection.$from;
		for (let d = from.depth; d > 0; d--)
			if (from.node(d).type.name === name) return { node: from.node(d), pos: from.before(d) };
		return null;
	}

	// The formatting bar: over selected text, in a table, or brought by
	// Alt+F10, and while it has focus.
	let bar = $state<HTMLElement>();
	let barFocus = $state(false);
	let summoned = $state(false);
	const sel = $derived.by(() => {
		void version;
		const s = editor.state.selection;
		return {
			empty: s.empty,
			from: s.from,
			inCode: editor.isActive('codeBlock'),
			inTable: editor.isActive('table'),
			image: editor.isActive('image'),
			view: editor.isActive('viewEmbed'),
			report: editor.isActive('reportEmbed'),
			columns: around('columns')?.node.childCount ?? 0,
			fold: around('fold'),
			inFoldTitle: editor.isActive('foldTitle')
		};
	});
	const barShown = $derived(
		!menuOpen &&
			!picking &&
			!pickingView &&
			!pickingReport &&
			!pickingDiagram &&
			(barFocus ||
				summoned ||
				(focused && !sel.inCode && (!sel.empty || sel.inTable || sel.inFoldTitle)))
	);

	/** Where something goes under or over the caret, inside the frame. */
	function placeAt(pos: number, above: boolean, height = 0) {
		try {
			const c = editor.view.coordsAtPos(pos);
			const f = frame.getBoundingClientRect();
			const left = Math.max(0, Math.min(c.left - f.left, f.width - 280));
			const top = above ? c.top - f.top - height - 6 : c.bottom - f.top + 4;
			return { left: `${left}px`, top: `${above && top < 0 ? c.bottom - f.top + 4 : top}px` };
		} catch {
			return { left: '0px', top: '0px' };
		}
	}
	let barHeight = $state(36);
	const barPlace = $derived.by(() => {
		void version;
		return barShown ? placeAt(sel.from, true, barHeight) : null;
	});
	const menuPlace = $derived.by(() => {
		void version;
		return menuOpen && at ? placeAt(at.to, false) : null;
	});

	type Tool = {
		id: string;
		label: string;
		icon: Component<{ size?: number }>;
		run: () => void;
		pressed?: () => boolean;
	};
	const run = () => editor.chain().focus();
	const FORMAT: Tool[] = [
		{
			id: 'bold',
			label: 'Bold',
			icon: Bold,
			run: () => run().toggleBold().run(),
			pressed: () => editor.isActive('bold')
		},
		{
			id: 'italic',
			label: 'Italic',
			icon: Italic,
			run: () => run().toggleItalic().run(),
			pressed: () => editor.isActive('italic')
		},
		{
			id: 'strike',
			label: 'Strikethrough',
			icon: Strikethrough,
			run: () => run().toggleStrike().run(),
			pressed: () => editor.isActive('strike')
		},
		{
			id: 'code',
			label: 'Code',
			icon: Code,
			run: () => run().toggleCode().run(),
			pressed: () => editor.isActive('code')
		},
		{
			id: 'link',
			label: 'Link',
			icon: Link,
			run: () => startLink(),
			pressed: () => editor.isActive('link')
		},
		{
			id: 'heading',
			label: 'Heading',
			icon: Heading,
			run: () => run().toggleHeading({ level: 2 }).run(),
			pressed: () => editor.isActive('heading')
		}
	];
	const TABLE: Tool[] = [
		{
			id: 'row',
			label: 'Add a row',
			icon: BetweenHorizontalEnd,
			run: () => run().addRowAfter().run()
		},
		{
			id: 'column',
			label: 'Add a column',
			icon: BetweenVerticalEnd,
			run: () => run().addColumnAfter().run()
		},
		{
			id: 'delete-row',
			label: 'Delete this row',
			icon: TableRowsSplit,
			run: () => run().deleteRow().run()
		},
		{
			id: 'delete-column',
			label: 'Delete this column',
			icon: TableColumnsSplit,
			run: () => run().deleteColumn().run()
		},
		{
			id: 'delete-table',
			label: 'Delete the table',
			icon: TableCellsMerge,
			run: () => run().deleteTable().run()
		}
	];
	// Font, size and highlight open a short list of their own in the bar. A choice
	// changes the text without the focus command, which would put the selection
	// back a moment later, over whatever key came next; closing the bar focuses it.
	const edit = () => editor.chain();
	type Choice = { id: string; label: string; pressed: boolean; style?: string; run: () => void };
	let choosing = $state<
		| null
		| 'font'
		| 'size'
		| 'highlight'
		| 'image-size'
		| 'image-side'
		| 'widths'
		| 'layout'
		| 'chart'
	>(null);
	const word = () =>
		editor.getAttributes('textStyle') as { font?: string | null; size?: string | null };
	function setStyle(change: { font?: string | null; size?: string | null }) {
		const next = { font: word().font ?? null, size: word().size ?? null, ...change };
		if (!next.font && !next.size) edit().unsetMark('textStyle').run();
		else edit().setMark('textStyle', next).run();
	}
	const image = () =>
		editor.getAttributes('image') as {
			size?: number | null;
			side?: string | null;
			caption?: string | null;
		};
	const choices = $derived.by((): Choice[] => {
		void version;
		// A choice made, back to the text, as setting a link is.
		const pick = (fn: () => void) => () => {
			fn();
			closeBar();
		};
		switch (choosing) {
			case 'font':
				return [
					{
						id: 'theme',
						label: "Theme's",
						pressed: !word().font,
						run: pick(() => setStyle({ font: null }))
					},
					...PAGE_FONTS.map((f) => ({
						id: f,
						label: FONT_NAMES[f],
						style: `font-family: ${FONT_STACKS[f]}`,
						pressed: word().font === f,
						run: pick(() => setStyle({ font: f }))
					}))
				];
			case 'size':
				return [
					{
						id: 'normal',
						label: 'Normal',
						pressed: !word().size,
						run: pick(() => setStyle({ size: null }))
					},
					...WORD_SIZES.map((z) => ({
						id: z,
						label: z === 'small' ? 'Small' : z === 'large' ? 'Large' : 'Huge',
						pressed: word().size === z,
						run: pick(() => setStyle({ size: z }))
					}))
				];
			case 'highlight':
				return [
					...HIGHLIGHT_COLORS.map((c) => ({
						id: c,
						label: `${c[0].toUpperCase()}${c.slice(1)}`,
						style: `background: var(--highlight-${c})`,
						pressed: editor.isActive('highlight', { color: c }),
						run: pick(() => edit().setMark('highlight', { color: c }).run())
					})),
					{
						id: 'none',
						label: 'No highlight',
						pressed: false,
						run: pick(() => edit().unsetMark('highlight').run())
					}
				];
			case 'image-size':
				return [
					...IMAGE_SIZES.map((n) => ({
						id: String(n),
						label:
							n === 100
								? 'Full width'
								: n === 25
									? 'A quarter'
									: n === 50
										? 'Half'
										: 'Three quarters',
						pressed: image().size === n,
						run: pick(() =>
							edit()
								.updateAttributes('image', { size: n, side: image().side ?? 'center' })
								.run()
						)
					})),
					{
						id: 'natural',
						label: 'As it is',
						pressed: image().size == null,
						run: pick(() =>
							edit().updateAttributes('image', { size: null, side: null, caption: null }).run()
						)
					}
				];
			case 'image-side':
				return IMAGE_SIDES.map((d) => ({
					id: d,
					label: d === 'left' ? 'On the left' : d === 'right' ? 'On the right' : 'In the middle',
					pressed: image().side === d,
					run: pick(() =>
						edit()
							.updateAttributes('image', { side: d, size: image().size ?? 100 })
							.run()
					)
				}));
			case 'layout': {
				// A saved view in the page: as the view lays it out, or as asked.
				const at = editor.getAttributes('viewEmbed') as { id?: string; layout?: string | null };
				return [
					['', 'As the view'],
					['list', 'List'],
					['board', 'Board'],
					['table', 'Table']
				].map(([layout, label]) => ({
					id: layout || 'view',
					label,
					pressed: (at.layout ?? '') === layout,
					run: pick(() =>
						edit()
							.updateAttributes('viewEmbed', { layout: layout || null })
							.run()
					)
				}));
			}
			case 'chart': {
				// A report in the page: any of its charts, in the tabs' order.
				const at = editor.getAttributes('reportEmbed') as { chart?: string };
				return REPORT_CHARTS.map((c) => ({
					id: c.key,
					label: c.label,
					pressed: at.chart === c.key,
					run: pick(() => edit().updateAttributes('reportEmbed', { chart: c.key }).run())
				}));
			}
			case 'widths':
				return [
					['1:1', 'Equal'],
					['2:1', 'Left wider'],
					['1:2', 'Right wider']
				].map(([layout, label]) => ({
					id: layout,
					label,
					pressed: around('columns')?.node.attrs.layout === layout,
					run: pick(() => edit().updateAttributes('columns', { layout }).run())
				}));
		}
		return [];
	});
	async function choose(kind: NonNullable<typeof choosing>) {
		choosing = kind;
		summoned = true;
		await tick();
		const buttons = bar?.querySelectorAll<HTMLButtonElement>('button');
		const on = [...(buttons ?? [])].findIndex((b) => b.getAttribute('aria-pressed') === 'true');
		buttons?.[Math.max(0, on)]?.focus();
	}
	const STYLE: Tool[] = [
		{
			id: 'font',
			label: 'Font',
			icon: Type,
			run: () => void choose('font'),
			pressed: () => !!word().font
		},
		{
			id: 'size',
			label: 'Size',
			icon: ALargeSmall,
			run: () => void choose('size'),
			pressed: () => !!word().size
		},
		{
			id: 'highlight',
			label: 'Highlight',
			icon: Highlighter,
			run: () => void choose('highlight'),
			pressed: () => editor.isActive('highlight')
		}
	];
	const VIEW: Tool[] = [
		{ id: 'layout', label: 'Show as', icon: LayoutDashboard, run: () => void choose('layout') }
	];
	const REPORT: Tool[] = [
		{ id: 'chart', label: 'Chart', icon: ChartLine, run: () => void choose('chart') }
	];
	const IMAGE: Tool[] = [
		{ id: 'image-size', label: 'Image size', icon: Scaling, run: () => void choose('image-size') },
		{
			id: 'image-side',
			label: 'Where it sits',
			icon: TextAlignCenter,
			run: () => void choose('image-side')
		},
		{
			id: 'caption',
			label: 'Caption',
			icon: Captions,
			run: () => void startCaption(),
			pressed: () => !!image().caption
		}
	];
	/** Takes the blocks out of the columns around the caret, in order. */
	function takeOutColumns() {
		const at = around('columns');
		if (!at) return;
		const blocks: import('@tiptap/pm/model').Node[] = [];
		at.node.forEach((col) => col.forEach((b) => void blocks.push(b)));
		editor.view.dispatch(
			editor.state.tr.replaceWith(at.pos, at.pos + at.node.nodeSize, Fragment.fromArray(blocks))
		);
		editor.commands.focus();
		announce('Took out the columns');
	}
	const COLUMNS = $derived<Tool[]>([
		...(sel.columns === 2
			? [{ id: 'widths', label: 'Column widths', icon: Columns2, run: () => void choose('widths') }]
			: []),
		{ id: 'take-out', label: 'Take out the columns', icon: Ungroup, run: takeOutColumns }
	]);
	const FOLD: Tool[] = [
		{
			id: 'fold-start',
			label: 'Starts open',
			icon: ListCollapse,
			run: () => {
				const at = around('fold');
				if (at) run().updateAttributes('fold', { open: !at.node.attrs.open }).run();
			},
			pressed: () => !!around('fold')?.node.attrs.open
		},
		{
			id: 'fold-toggle',
			label: 'Open or close it here',
			icon: ChevronsDownUp,
			run: () => {
				const at = around('fold');
				const el = at ? editor.view.nodeDOM(at.pos) : null;
				if (el instanceof HTMLElement)
					el.querySelector<HTMLButtonElement>(':scope > .fold-toggle')?.click();
			}
		}
	];

	// An image's caption, typed in the bar like a link's address.
	let captioning = $state(false);
	let caption = $state('');
	let captionInput = $state<HTMLInputElement>();
	async function startCaption() {
		caption = image().caption ?? '';
		captioning = true;
		summoned = true;
		await tick();
		captionInput?.focus();
		captionInput?.select();
	}
	function applyCaption() {
		const text = caption.trim();
		run()
			.updateAttributes('image', {
				caption: text || null,
				size: image().size ?? 100,
				side: image().side ?? 'center'
			})
			.run();
		closeBar();
	}

	const COMMENT: Tool = {
		id: 'comment',
		label: 'Comment',
		icon: MessageSquarePlus,
		run: () => oncomment?.()
	};
	const tools = $derived(
		sel.image
			? IMAGE
			: sel.view
				? VIEW
				: sel.report
					? REPORT
					: sel.inFoldTitle
						? FOLD
						: [
								...FORMAT,
								...STYLE,
								...(sel.inTable ? TABLE : []),
								...(sel.columns ? COLUMNS : []),
								...(sel.fold ? FOLD : []),
								...(oncomment && !sel.empty ? [COMMENT] : [])
							]
	);
	/** Where a line goes between the bar's kinds of tools. */
	const SEPARATE = new Set(['font', 'row', 'widths', 'take-out', 'fold-start', 'comment']);
	let current = $state(0);
	const pressed = (t: Tool) => {
		void version;
		return t.pressed?.();
	};

	// A link's address, typed in the bar.
	let linking = $state(false);
	let href = $state('');
	let linkInput = $state<HTMLInputElement>();
	async function startLink() {
		href = (editor.getAttributes('link').href as string | undefined) ?? '';
		linking = true;
		summoned = true;
		await tick();
		linkInput?.focus();
		linkInput?.select();
	}
	function applyLink() {
		const url = href.trim();
		const chain = run().extendMarkRange('link');
		if (url) chain.setLink({ href: url }).run();
		else chain.unsetLink().run();
		closeBar();
	}
	function closeBar() {
		linking = false;
		captioning = false;
		choosing = null;
		summoned = false;
		// At once, before the bar's buttons go, so that no key typed next is lost,
		// and without putting back a selection that a key since has changed.
		editor.view.focus();
	}

	async function focusBar() {
		summoned = true;
		await tick();
		const buttons = bar?.querySelectorAll<HTMLButtonElement>('button');
		buttons?.[Math.min(current, buttons.length - 1)]?.focus();
	}

	function barKeys(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			// Out of a short list, back to the tools; from the tools, back to the text.
			if (choosing) {
				choosing = null;
				void focusBar();
			} else closeBar();
			return;
		}
		if (linking || captioning) return;
		const buttons = [...(bar?.querySelectorAll<HTMLButtonElement>('button') ?? [])];
		const at = buttons.indexOf(document.activeElement as HTMLButtonElement);
		let next = -1;
		if (e.key === 'ArrowRight') next = (at + 1) % buttons.length;
		else if (e.key === 'ArrowLeft') next = (at - 1 + buttons.length) % buttons.length;
		else if (e.key === 'Home') next = 0;
		else if (e.key === 'End') next = buttons.length - 1;
		if (next < 0) return;
		e.preventDefault();
		current = next;
		buttons[next].focus();
	}

	/** Keys in the text, before the editor's own: the menu's, and Alt+F10. */
	export function keydown(e: KeyboardEvent): boolean {
		if (menuOpen) {
			const n = items.length;
			switch (e.key) {
				case 'ArrowDown':
				case 'ArrowUp':
					e.preventDefault();
					active = (active + (e.key === 'ArrowDown' ? 1 : n - 1)) % n;
					announce(items[active].label);
					return true;
				case 'Enter':
				case 'Tab':
					e.preventDefault();
					items[active].choose();
					return true;
				case 'Escape':
					e.preventDefault();
					dismissed = at?.from ?? null;
					return true;
			}
		}
		if (
			oncomment &&
			keymap.match('elsewhere', e, { field: true, handled: false }) === 'doc.comment'
		) {
			e.preventDefault();
			oncomment();
			return true;
		}
		if (e.key === 'F10' && e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
			e.preventDefault();
			void focusBar();
			return true;
		}
		return false;
	}

	$effect(() => {
		// A bar summoned with nothing to act on goes when focus leaves it.
		if (!barFocus && !linking && !captioning && !choosing) untrack(() => (summoned = false));
	});
</script>

{#if barShown && barPlace}
	<!-- The bar never takes the text's focus from a pointer: buttons act on the selection. -->
	<div
		class="bar"
		role="toolbar"
		aria-label="Formatting"
		tabindex="-1"
		bind:this={bar}
		bind:clientHeight={barHeight}
		style:left={barPlace?.left}
		style:top={barPlace?.top}
		onkeydown={barKeys}
		onfocusin={() => (barFocus = true)}
		onfocusout={(e) => {
			if (!bar?.contains(e.relatedTarget as Node)) barFocus = false;
		}}
		onmousedown={(e) => {
			if (!(e.target as HTMLElement).closest('input')) e.preventDefault();
		}}
	>
		{#if linking}
			<input
				class="href"
				type="url"
				aria-label="Link address"
				placeholder="https://"
				bind:this={linkInput}
				bind:value={href}
				onkeydown={(e) => {
					if (e.key === 'Enter') {
						e.preventDefault();
						applyLink();
					}
				}}
			/>
			<button type="button" class="text" onclick={applyLink}
				>{href.trim() ? 'Apply' : 'Remove'}</button
			>
		{:else if captioning}
			<input
				class="href"
				type="text"
				aria-label="Caption"
				placeholder="What's in the picture"
				maxlength="300"
				bind:this={captionInput}
				bind:value={caption}
				onkeydown={(e) => {
					if (e.key === 'Enter') {
						e.preventDefault();
						applyCaption();
					}
				}}
			/>
			<button type="button" class="text" onclick={applyCaption}
				>{caption.trim() ? 'Apply' : 'Remove'}</button
			>
		{:else if choosing}
			<div
				class="choices"
				role="group"
				aria-label={choosing === 'highlight'
					? 'Highlight color'
					: choosing === 'font'
						? 'Font'
						: choosing === 'layout'
							? 'Show as'
							: choosing === 'chart'
								? 'Chart'
								: 'Choices'}
			>
				{#each choices as c (c.id)}
					<button
						type="button"
						class="text"
						class:swatch={choosing === 'highlight' && c.id !== 'none'}
						aria-pressed={c.pressed}
						title={c.label}
						aria-label={c.label}
						style={c.style}
						onclick={c.run}
						>{#if choosing !== 'highlight' || c.id === 'none'}{c.label}{/if}</button
					>
				{/each}
			</div>
		{:else}
			{#each tools as t, i (t.id)}
				{#if i > 0 && SEPARATE.has(t.id)}<span class="sep" aria-hidden="true"></span>{/if}
				<button
					type="button"
					tabindex={i === current ? 0 : -1}
					aria-label={t.label}
					title={t.label}
					aria-pressed={t.pressed ? !!pressed(t) : undefined}
					onclick={() => {
						current = i;
						t.run();
					}}><t.icon size={15} /></button
				>
			{/each}
		{/if}
	</div>
{/if}

{#if menuOpen && menuPlace}
	<ul
		class="blocks"
		id={`${uid}-menu`}
		role="listbox"
		aria-label={menuName}
		style:left={menuPlace?.left}
		style:top={menuPlace?.top}
	>
		{#each items as item, i (item.id)}
			<!-- Chosen with the keys in the text, or a click that keeps the text's focus. -->
			<!-- svelte-ignore a11y_click_events_have_key_events -->
			<li
				id={optionId(i)}
				role="option"
				aria-selected={i === active}
				class:active={i === active}
				onmousedown={(e) => e.preventDefault()}
				onclick={() => item.choose()}
			>
				<span class="label">{item.label}</span>
				{#if item.detail}<span class="detail">{item.detail}</span>{/if}
			</li>
		{/each}
	</ul>
{/if}

{#if picking}
	<ImagePicker {ws} {target} bind:open={picking} onpick={insertImage} />
{/if}
{#if pickingView}
	<ViewPicker {ws} bind:open={pickingView} onpick={insertView} />
{/if}
{#if pickingReport}
	<ViewPicker {ws} kind="report" bind:open={pickingReport} onpick={insertReport} />
{/if}
{#if pickingDiagram}
	<DiagramPicker bind:open={pickingDiagram} onpick={insertStarter} />
{/if}

<style>
	.bar {
		position: absolute;
		z-index: 5;
		display: flex;
		align-items: center;
		gap: 2px;
		padding: 3px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--menu-bg);
		box-shadow: var(--shadow-md);
	}
	.bar button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: max(var(--tap-min), 28px);
		min-height: max(var(--tap-min), 28px);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-primary);
		cursor: pointer;
	}
	.bar button:hover {
		background: var(--surface-2);
	}
	.bar button[aria-pressed='true'] {
		background: var(--accent-soft);
		color: var(--accent-text);
	}
	.bar button.text {
		padding: 0 var(--space-2);
		font-size: var(--text-sm);
	}
	.choices {
		display: flex;
		flex-wrap: wrap;
		gap: 2px;
		max-width: min(440px, calc(100vw - 48px));
	}
	.bar button.swatch {
		min-width: max(var(--tap-min), 28px);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-full);
	}
	.bar button.swatch[aria-pressed='true'] {
		outline: 2px solid var(--focus-ring-color);
		outline-offset: 1px;
	}
	.sep {
		width: 1px;
		align-self: stretch;
		margin: 2px 3px;
		background: var(--border);
	}
	.href {
		width: 220px;
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}
	.blocks {
		position: absolute;
		z-index: 5;
		min-width: 200px;
		max-height: 280px;
		margin: 0;
		padding: 4px;
		overflow-y: auto;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--menu-bg);
		box-shadow: var(--shadow-md);
	}
	.blocks li {
		display: flex;
		align-items: center;
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border-radius: var(--radius-sm);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.blocks li {
		justify-content: space-between;
		gap: var(--space-3);
	}
	.blocks .label {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.blocks .detail {
		flex: none;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.blocks li.active {
		background: var(--accent-soft);
		color: var(--accent-text);
	}
</style>
