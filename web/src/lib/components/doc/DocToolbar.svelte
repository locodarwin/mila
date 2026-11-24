<script lang="ts">
	// A wiki page's formatting toolbar, loaded with the editor's tools
	// (DocEditor's toolbar()). In the page's bar it shows the Markdown basics
	// and Mila's blocks, folding groups away in a fixed order when the bar is
	// short of room (docs/toolbar.ts); on a phone it sits above the on-screen
	// keyboard while the page is being edited and scrolls sideways. Its
	// buttons never take the caret out of the text. Blocks and links go
	// through the editor's tools (DocTools), as the `/` menu's do.
	import Bold from '@lucide/svelte/icons/bold';
	import ChartColumn from '@lucide/svelte/icons/chart-column';
	import Check from '@lucide/svelte/icons/check';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import Code from '@lucide/svelte/icons/code';
	import Columns2 from '@lucide/svelte/icons/columns-2';
	import Columns3 from '@lucide/svelte/icons/columns-3';
	import Ellipsis from '@lucide/svelte/icons/ellipsis';
	import Heading2 from '@lucide/svelte/icons/heading-2';
	import ImageIcon from '@lucide/svelte/icons/image';
	import Info from '@lucide/svelte/icons/info';
	import Italic from '@lucide/svelte/icons/italic';
	import Layers from '@lucide/svelte/icons/layers';
	import Lightbulb from '@lucide/svelte/icons/lightbulb';
	import Link from '@lucide/svelte/icons/link';
	import List from '@lucide/svelte/icons/list';
	import ListCollapse from '@lucide/svelte/icons/list-collapse';
	import ListOrdered from '@lucide/svelte/icons/list-ordered';
	import ListTodo from '@lucide/svelte/icons/list-todo';
	import Minus from '@lucide/svelte/icons/minus';
	import Network from '@lucide/svelte/icons/network';
	import Plus from '@lucide/svelte/icons/plus';
	import Quote from '@lucide/svelte/icons/quote';
	import Shapes from '@lucide/svelte/icons/shapes';
	import SquareCode from '@lucide/svelte/icons/square-code';
	import Strikethrough from '@lucide/svelte/icons/strikethrough';
	import Table from '@lucide/svelte/icons/table';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import Undo2 from '@lucide/svelte/icons/undo-2';
	import type { Editor } from '@tiptap/core';
	import type { Component } from 'svelte';
	import { tick } from 'svelte';
	import { blockOf, folding, INSERT_GROUPS, styleName, TEXT_STYLES } from '../../docs/toolbar';
	import * as fit from '../../docs/toolbar';
	import type { DocToolsApi } from '../../docs/toolbar';
	import type { CommandId } from '../../keys/commands';
	import { keymap } from '../../keys/keymap.svelte';
	import { anchored } from '../../popover';

	interface Props {
		editor: Editor;
		/** Changes with every change to the editor's state, so what is shown follows it. */
		version: number;
		/** Whether the text has focus. */
		focused: boolean;
		tools: DocToolsApi;
		/** Tells the editor how to move focus here (Alt+F10), or null as it goes. */
		register?: (focus: (() => boolean) | null) => void;
		/** Above the on-screen keyboard, on a phone, rather than in the page's bar. */
		docked?: boolean;
		/** The width it has in the page's bar. */
		room?: number;
		/** Its width with nothing folded away, and with everything that can be. */
		widest?: number;
		narrowest?: number;
	}

	let {
		editor,
		version,
		focused,
		tools,
		register,
		docked = false,
		room = Infinity,
		// eslint-disable-next-line no-useless-assignment -- read by the wiki, through bind:widest
		widest = $bindable(0),
		// eslint-disable-next-line no-useless-assignment -- read by the wiki, through bind:narrowest
		narrowest = $bindable(0)
	}: Props = $props();
	const uid = $props.id();

	type Tool = {
		id: string;
		label: string;
		icon: Component<{ size?: number }>;
		run: () => void;
		/** Shown pressed while true: what the caret is in. */
		on?: () => boolean;
		/** Dimmed while false: where it can't apply. */
		can?: () => boolean;
		/** The command whose keys its hover text gives. */
		keys?: CommandId;
	};

	// Before the text has had the caret, a button acts at the end of the page.
	let touched = $state(false);
	$effect(() => {
		if (focused) touched = true;
	});
	// Tiptap's focus reaches the browser a frame later; the first time, the
	// text takes focus at once, so that nothing typed straight after is lost.
	function ready() {
		if (!touched && !editor.isFocused) {
			editor.view.focus();
			editor.commands.focus('end');
		}
		touched = true;
	}
	const chain = () => {
		ready();
		return editor.chain().focus();
	};
	const insert = (id: string) => {
		ready();
		tools.insert(id);
	};

	const MARKS: Tool[] = [
		{
			id: 'bold',
			label: 'Bold',
			icon: Bold,
			run: () => chain().toggleBold().run(),
			on: () => editor.isActive('bold'),
			can: () => editor.can().toggleBold(),
			keys: 'doc.bold'
		},
		{
			id: 'italic',
			label: 'Italic',
			icon: Italic,
			run: () => chain().toggleItalic().run(),
			on: () => editor.isActive('italic'),
			can: () => editor.can().toggleItalic(),
			keys: 'doc.italic'
		}
	];
	const MORE_MARKS: Tool[] = [
		{
			id: 'strike',
			label: 'Strikethrough',
			icon: Strikethrough,
			run: () => chain().toggleStrike().run(),
			on: () => editor.isActive('strike'),
			can: () => editor.can().toggleStrike(),
			keys: 'doc.strike'
		},
		{
			id: 'code',
			label: 'Code',
			icon: Code,
			run: () => chain().toggleCode().run(),
			on: () => editor.isActive('code'),
			can: () => editor.can().toggleCode(),
			keys: 'doc.code'
		},
		{
			id: 'link',
			label: 'Link',
			icon: Link,
			run: () => {
				ready();
				void tools.startLink();
			},
			on: () => editor.isActive('link'),
			can: () => !editor.isActive('codeBlock')
		}
	];
	const LISTS: Tool[] = [
		{
			id: 'bullets',
			label: 'Bulleted list',
			icon: List,
			run: () => chain().toggleBulletList().run(),
			on: () => editor.isActive('bulletList'),
			can: () => editor.can().toggleBulletList()
		},
		{
			id: 'numbers',
			label: 'Numbered list',
			icon: ListOrdered,
			run: () => chain().toggleOrderedList().run(),
			on: () => editor.isActive('orderedList'),
			can: () => editor.can().toggleOrderedList()
		},
		{
			id: 'checklist',
			label: 'Checklist',
			icon: ListTodo,
			run: () => chain().toggleTaskList().run(),
			on: () => editor.isActive('taskList'),
			can: () => editor.can().toggleTaskList()
		},
		{
			id: 'quote',
			label: 'Quote',
			icon: Quote,
			run: () => chain().toggleBlockquote().run(),
			on: () => editor.isActive('blockquote'),
			can: () => editor.can().toggleBlockquote()
		}
	];
	const inTable = () => editor.isActive('table');
	const BLOCK_TOOLS: Tool[] = [
		{ id: 'table', label: 'Table', icon: Table, run: () => insert('table'), can: () => !inTable() },
		{
			id: 'code-block',
			label: 'Code block',
			icon: SquareCode,
			run: () => chain().toggleCodeBlock().run(),
			on: () => editor.isActive('codeBlock'),
			can: () => editor.can().toggleCodeBlock()
		},
		{ id: 'rule', label: 'Divider', icon: Minus, run: () => insert('rule') }
	];
	const PICTURES: Tool[] = [
		{ id: 'image', label: 'Image or SVG file', icon: ImageIcon, run: () => insert('image') },
		{ id: 'diagram', label: 'Diagram', icon: Network, run: () => insert('diagram') },
		{ id: 'drawing', label: 'Drawing', icon: Shapes, run: () => insert('drawing') }
	];
	const UNDO: Tool = {
		id: 'undo',
		label: 'Undo',
		icon: Undo2,
		run: () => {
			ready();
			editor.commands.focus();
			(editor.commands as unknown as { undo?: () => boolean }).undo?.();
		}
	};
	/** What a phone's row holds; the rest is in Insert. */
	const DOCKED: Tool[] = [
		...MARKS,
		LISTS[0],
		LISTS[2],
		MORE_MARKS[2],
		PICTURES[0],
		PICTURES[1],
		UNDO
	];

	/** Its state, read again with every change to the editor's. */
	const look = (t: Tool) => {
		void version;
		return { on: t.on ? t.on() : undefined, off: t.can ? !t.can() : false };
	};
	const hover = (t: Tool) => {
		const key = t.keys ? keymap.label(t.keys) : null;
		return key ? `${t.label} (${key})` : t.label;
	};

	// The Insert menu: the `/` menu's blocks, grouped, each with its icon.
	const BLOCK_ICONS: Record<string, Component<{ size?: number }>> = {
		heading: Heading2,
		bullets: List,
		numbers: ListOrdered,
		checklist: ListTodo,
		quote: Quote,
		code: SquareCode,
		table: Table,
		rule: Minus,
		'columns-2': Columns2,
		'columns-3': Columns3,
		fold: ListCollapse,
		note: Info,
		tip: Lightbulb,
		warning: TriangleAlert,
		image: ImageIcon,
		diagram: Network,
		drawing: Shapes,
		view: Layers,
		report: ChartColumn
	};
	const NAMES: Record<string, string> = { image: 'Image or SVG file' };
	const insertGroups = INSERT_GROUPS.map((g) => ({
		name: g.name,
		items: g.ids.flatMap((id) => {
			const b = blockOf(id);
			return b ? [{ id, label: NAMES[id] ?? b.label, icon: BLOCK_ICONS[id] ?? Plus }] : [];
		})
	}));
	const blockOff = (id: string) => {
		void version;
		if (id === 'table') return inTable();
		if (id.startsWith('columns')) return editor.isActive('column') || inTable();
		return false;
	};

	const current = $derived.by(() => {
		void version;
		return styleName(editor);
	});
	const styleOff = (level: number) => {
		void version;
		return level
			? !editor.can().setHeading({ level: level as 1 | 2 | 3 })
			: !editor.can().setParagraph();
	};
	function setStyle(level: number) {
		if (level)
			chain()
				.setHeading({ level: level as 1 | 2 | 3 })
				.run();
		else chain().setParagraph().run();
	}

	// Folding to fit: each part's width, measured whether it shows or not.
	let w = $state({ style: 0, marks: 0, more: 0, moreMarks: 0, lists: 0, blocks: 0, pictures: 0 });
	/** Insert's part, its line and word included. */
	let insertWidth = $state(0);
	let labelWidth = $state(0);
	let folded = $state<Set<fit.Fold>>(new Set());
	const widths = $derived<fit.Widths>({
		base: w.style + w.marks + insertWidth + (folded.has('label') ? labelWidth : 0),
		blocks: w.blocks,
		pictures: w.pictures,
		lists: w.lists,
		marks: w.moreMarks,
		label: labelWidth,
		more: w.more
	});
	// Worked out from the natural widths, which folding leaves alone.
	$effect(() => {
		const next = docked || !w.style ? new Set<fit.Fold>() : folding(room, widths);
		if ([...next].join() !== [...folded].join()) folded = next;
	});
	$effect(() => {
		if (docked) return;
		widest = fit.widest(widths);
		narrowest = fit.narrowest(widths);
	});
	const moreShown = $derived(folded.has('lists') || folded.has('marks'));

	// One Tab stop, arrows along it, Escape back to the text.
	let bar = $state<HTMLElement>();
	let currentId = $state('style');
	function shownButtons(): HTMLButtonElement[] {
		return [...(bar?.querySelectorAll<HTMLButtonElement>('button[data-id]') ?? [])].filter(
			(b) => !b.closest('.folded')
		);
	}
	/** Which fold takes a button away, if any. */
	const FOLD_OF: Record<string, fit.Fold> = Object.fromEntries([
		...MORE_MARKS.map((t) => [t.id, 'marks']),
		...LISTS.map((t) => [t.id, 'lists']),
		...BLOCK_TOOLS.map((t) => [t.id, 'blocks']),
		...PICTURES.map((t) => [t.id, 'pictures'])
	]);
	const isShown = (id: string) =>
		docked
			? id === 'style' || id === 'insert' || DOCKED.some((t) => t.id === id)
			: id === 'more'
				? moreShown
				: !FOLD_OF[id] || !folded.has(FOLD_OF[id]);
	function focusHere(): boolean {
		const buttons = shownButtons();
		if (!buttons.length) return false;
		(buttons.find((b) => b.dataset.id === currentId) ?? buttons[0]).focus();
		return true;
	}
	$effect(() => {
		register?.(focusHere);
		return () => register?.(null);
	});
	function keys(e: KeyboardEvent) {
		const target = e.target as HTMLElement;
		if (!target.matches('button[data-id]')) return;
		if (e.key === 'Escape') {
			e.preventDefault();
			e.stopPropagation();
			editor.view.focus();
			return;
		}
		const buttons = shownButtons();
		const at = buttons.indexOf(target as HTMLButtonElement);
		let next = -1;
		if (e.key === 'ArrowRight') next = (at + 1) % buttons.length;
		else if (e.key === 'ArrowLeft') next = (at - 1 + buttons.length) % buttons.length;
		else if (e.key === 'Home') next = 0;
		else if (e.key === 'End') next = buttons.length - 1;
		if (next < 0) return;
		e.preventDefault();
		buttons[next].focus();
	}
	const tabindex = (id: string) => (id === (isShown(currentId) ? currentId : 'style') ? 0 : -1);

	// The menus: Text style, Insert, and the ⋯ that holds what folded away.
	const styleMenuId = `${uid}-style`;
	const insertMenuId = `${uid}-insert`;
	const moreMenuId = `${uid}-more`;
	let open = $state<string | null>(null);
	function toggled(e: ToggleEvent) {
		const menu = e.currentTarget as HTMLElement;
		open = e.newState === 'open' ? menu.id : open === menu.id ? null : open;
		if (e.newState !== 'open') return;
		// Opened from the keys: into the menu. A click leaves the caret in the text.
		const trigger = document.querySelector(`[popovertarget="${CSS.escape(menu.id)}"]`);
		if (trigger && document.activeElement === trigger)
			void tick().then(() =>
				menu.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()
			);
	}
	function menuKeys(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp' && e.key !== 'Home' && e.key !== 'End') return;
		const items = [
			...(e.currentTarget as HTMLElement).querySelectorAll<HTMLButtonElement>(
				'button:not(:disabled)'
			)
		];
		const at = items.indexOf(document.activeElement as HTMLButtonElement);
		const next =
			e.key === 'Home'
				? 0
				: e.key === 'End'
					? items.length - 1
					: (at + (e.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length;
		e.preventDefault();
		items[next]?.focus();
	}
	/** A choice from a menu: the menu closes and the change is made in the text. */
	function choose(menuId: string, run: () => void) {
		document.getElementById(menuId)?.hidePopover();
		run();
	}

	// On a phone: above the on-screen keyboard, while the page is being edited.
	let within = $state(false);
	let lift = $state(0);
	$effect(() => {
		if (!docked || !window.visualViewport) return;
		const vv = window.visualViewport;
		const place = () => (lift = Math.max(0, innerHeight - vv.height - vv.offsetTop));
		place();
		vv.addEventListener('resize', place);
		vv.addEventListener('scroll', place);
		return () => {
			vv.removeEventListener('resize', place);
			vv.removeEventListener('scroll', place);
		};
	});
	const shown = $derived(!docked || focused || within || open !== null);
</script>

{#snippet button(t: Tool)}
	{@const s = look(t)}
	<button
		type="button"
		class="tool"
		data-id={t.id}
		tabindex={tabindex(t.id)}
		aria-label={t.label}
		title={hover(t)}
		aria-keyshortcuts={t.keys ? keymap.aria(t.keys) : undefined}
		aria-pressed={s.on}
		aria-disabled={s.off || undefined}
		onfocus={() => (currentId = t.id)}
		onclick={() => {
			if (!look(t).off) t.run();
		}}><t.icon size={16} /></button
	>
{/snippet}

{#snippet styleButton(short: boolean)}
	<button
		type="button"
		class="tool style"
		data-id="style"
		tabindex={tabindex('style')}
		popovertarget={styleMenuId}
		aria-label={`Text style: ${current}`}
		title="Text style"
		aria-expanded={open === styleMenuId}
		onfocus={() => (currentId = 'style')}
		><span class="name">{short ? 'Aa' : current}</span><ChevronDown size={13} /></button
	>
{/snippet}

{#snippet insertButton()}
	<button
		type="button"
		class="tool insert"
		data-id="insert"
		tabindex={tabindex('insert')}
		popovertarget={insertMenuId}
		aria-label="Insert"
		title="Insert a block, as typing / does"
		aria-expanded={open === insertMenuId}
		onfocus={() => (currentId = 'insert')}
		><Plus size={16} /><span
			class="label"
			class:folded={docked || folded.has('label')}
			bind:offsetWidth={labelWidth}>Insert</span
		><ChevronDown size={13} /></button
	>
{/snippet}

{#if shown}
	<!-- Buttons act on the text without taking its focus from a pointer. -->
	<div
		class="toolbar"
		class:docked
		role="toolbar"
		aria-label="Format and insert"
		tabindex="-1"
		style:bottom={docked ? `${lift}px` : undefined}
		bind:this={bar}
		onkeydown={keys}
		onfocusin={() => (within = true)}
		onfocusout={(e) => {
			if (!bar?.contains(e.relatedTarget as Node)) within = false;
		}}
		onmousedown={(e) => e.preventDefault()}
	>
		{#if docked}
			{@render styleButton(true)}
			{#each DOCKED as t (t.id)}{@render button(t)}{/each}
			{@render insertButton()}
		{:else}
			<div class="part" bind:offsetWidth={w.style}>{@render styleButton(false)}</div>
			<div class="part sep" bind:offsetWidth={w.marks}>
				{#each MARKS as t (t.id)}{@render button(t)}{/each}
			</div>
			<div class="part" class:folded={folded.has('marks')} bind:offsetWidth={w.moreMarks}>
				{#each MORE_MARKS as t (t.id)}{@render button(t)}{/each}
			</div>
			<div class="part" class:folded={!moreShown} bind:offsetWidth={w.more}>
				<button
					type="button"
					class="tool"
					data-id="more"
					tabindex={tabindex('more')}
					popovertarget={moreMenuId}
					aria-label="More formatting"
					title="More formatting"
					aria-expanded={open === moreMenuId}
					onfocus={() => (currentId = 'more')}><Ellipsis size={16} /></button
				>
			</div>
			<div class="part sep" class:folded={folded.has('lists')} bind:offsetWidth={w.lists}>
				{#each LISTS as t (t.id)}{@render button(t)}{/each}
			</div>
			<div class="part sep" class:folded={folded.has('blocks')} bind:offsetWidth={w.blocks}>
				{#each BLOCK_TOOLS as t (t.id)}{@render button(t)}{/each}
			</div>
			<div class="part sep" class:folded={folded.has('pictures')} bind:offsetWidth={w.pictures}>
				{#each PICTURES as t (t.id)}{@render button(t)}{/each}
			</div>
			<div class="part sep" bind:offsetWidth={insertWidth}>{@render insertButton()}</div>
		{/if}
	</div>
{/if}

<div
	class="menu"
	popover
	id={styleMenuId}
	role="menu"
	aria-label="Text style"
	tabindex="-1"
	use:anchored
	ontoggle={toggled}
	onkeydown={menuKeys}
	onmousedown={(e) => e.preventDefault()}
>
	{#each TEXT_STYLES as s (s.level)}
		<button
			type="button"
			class="option"
			role="menuitemradio"
			aria-checked={current === s.label}
			disabled={styleOff(s.level)}
			onclick={() => choose(styleMenuId, () => setStyle(s.level))}
			><span class={`sample h${s.level}`}>{s.label}</span>{#if current === s.label}<Check
					size={15}
				/>{/if}</button
		>
	{/each}
</div>

<div
	class="menu insert-menu"
	popover
	id={insertMenuId}
	role="menu"
	aria-label="Insert"
	tabindex="-1"
	use:anchored
	ontoggle={toggled}
	onkeydown={menuKeys}
	onmousedown={(e) => e.preventDefault()}
>
	{#each insertGroups as g (g.name)}
		<div class="group" role="group" aria-labelledby={`${uid}-${g.name}`}>
			<p class="heading" id={`${uid}-${g.name}`}>{g.name}</p>
			{#each g.items as item (item.id)}
				<button
					type="button"
					class="option"
					role="menuitem"
					disabled={blockOff(item.id)}
					onclick={() => choose(insertMenuId, () => insert(item.id))}
					><item.icon size={15} />{item.label}</button
				>
			{/each}
			{#if g.name === 'From Mila'}
				<button
					type="button"
					class="option"
					role="menuitem"
					onclick={() =>
						choose(insertMenuId, () => {
							ready();
							tools.linkTo();
						})}><Link size={15} />Link to a page or task</button
				>
			{/if}
		</div>
	{/each}
</div>

{#if !docked}
	<div
		class="menu"
		popover
		id={moreMenuId}
		role="menu"
		aria-label="More formatting"
		tabindex="-1"
		use:anchored
		ontoggle={toggled}
		onkeydown={menuKeys}
		onmousedown={(e) => e.preventDefault()}
	>
		{#each [...(folded.has('marks') ? MORE_MARKS : []), ...(folded.has('lists') ? LISTS : [])] as t (t.id)}
			{@const s = look(t)}
			<button
				type="button"
				class="option"
				role={t.on ? 'menuitemcheckbox' : 'menuitem'}
				aria-checked={t.on ? !!s.on : undefined}
				disabled={s.off}
				title={hover(t)}
				onclick={() => choose(moreMenuId, t.run)}><t.icon size={15} />{t.label}</button
			>
		{/each}
	</div>
{/if}

<style>
	.toolbar {
		position: relative;
		display: flex;
		align-items: center;
		min-width: 0;
		overflow: hidden;
	}
	.part {
		flex: none;
		display: flex;
		align-items: center;
		gap: 2px;
	}
	/* The line between groups is drawn inside the part's own padding, so its
	   measured width (offsetWidth, which leaves margins out) is all it takes. */
	.part.sep {
		position: relative;
		padding-left: calc(2 * var(--space-1) + var(--border-width));
	}
	.part.sep::before {
		content: '';
		position: absolute;
		top: 4px;
		bottom: 4px;
		left: var(--space-1);
		width: var(--border-width);
		background: var(--border);
	}
	/* Out of the row, but still measured, so it can come back when there is room. */
	.folded {
		position: absolute;
		visibility: hidden;
		pointer-events: none;
	}
	/* As specific as .part.sep, and after it, so a folded part with a line leaves the row too. */
	.part.folded {
		position: absolute;
		top: 0;
		left: 0;
	}
	.tool {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 2px;
		min-width: max(var(--tap-min), 30px);
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-1);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		white-space: nowrap;
		cursor: pointer;
	}
	.tool:hover:not([aria-disabled='true']) {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.tool[aria-pressed='true'] {
		background: var(--accent-soft);
		color: var(--accent-text);
	}
	.tool[aria-expanded='true'] {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.tool[aria-disabled='true'] {
		opacity: 0.4;
		cursor: default;
	}
	.style {
		justify-content: space-between;
		min-width: calc(6.5em * var(--ui-scale, 1));
		padding: 0 var(--space-1) 0 var(--space-2);
		color: var(--text-primary);
	}
	.insert {
		color: var(--text-primary);
	}
	.label {
		padding-left: var(--space-1);
	}
	.docked {
		position: fixed;
		right: 0;
		left: 0;
		z-index: 50;
		gap: 2px;
		padding: var(--space-1) max(var(--space-2), env(safe-area-inset-right))
			calc(var(--space-1) + env(safe-area-inset-bottom))
			max(var(--space-2), env(safe-area-inset-left));
		overflow-x: auto;
		scrollbar-width: none;
		border-top: var(--border-width) solid var(--border);
		background: var(--surface-1);
	}
	.docked .tool {
		flex: none;
	}
	.docked .style {
		min-width: max(var(--tap-min), 30px);
		gap: 2px;
	}
	.menu .option {
		width: 100%;
		border: none;
		background: none;
		color: inherit;
		font: inherit;
		text-align: left;
	}
	.menu .option:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.menu .option[aria-checked='true'] {
		color: var(--accent-text);
	}
	.menu .option :global(svg) {
		flex: none;
		color: var(--text-secondary);
	}
	.menu .option:focus-visible {
		background: var(--surface-2);
	}
	.sample {
		flex: 1;
	}
	.sample.h1 {
		font-size: 1.4em;
		font-weight: 700;
	}
	.sample.h2 {
		font-size: 1.2em;
		font-weight: 650;
	}
	.sample.h3 {
		font-size: 1.05em;
		font-weight: 600;
	}
	.insert-menu {
		max-height: min(560px, 80vh);
	}
	@media (min-width: 721px) {
		.insert-menu {
			display: grid;
			grid-template-columns: 1fr 1fr;
			column-gap: var(--space-2);
			min-width: 440px;
		}
		.insert-menu:not(:popover-open) {
			display: none;
		}
	}
	.group {
		min-width: 0;
	}
	.heading {
		margin: 0;
	}
</style>
