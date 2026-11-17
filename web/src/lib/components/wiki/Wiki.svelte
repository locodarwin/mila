<script lang="ts">
	import { takeTemplateFill } from '../../templateFill';
	import type { MenuSource } from '../../contextMenu.svelte';
	import Splitter from '../Splitter.svelte';
	import { layout, WIKI_PAGE, WIKI_SIDE_MIN, WIKI_TREE } from '../../layout.svelte';
	import { goto } from '$app/navigation';
	import { page as route } from '$app/state';
	import { slugOf } from '../../mentions';
	// A wiki: the workspace's, or a project's on its
	// Wiki tab. Its pages in a tree beside the page shown, which folds away;
	// the page's path, the people in it, its Markdown view and its menu
	// above it; its title and its live document below; and, in the space
	// left over on its right, its contents or the tree again.
	import Ellipsis from '@lucide/svelte/icons/ellipsis';
	import FileCode from '@lucide/svelte/icons/file-code';
	import History from '@lucide/svelte/icons/history';
	import Type from '@lucide/svelte/icons/type';
	import LayoutTemplate from '@lucide/svelte/icons/layout-template';
	import MessageSquare from '@lucide/svelte/icons/message-square';
	import Link from '@lucide/svelte/icons/link';
	import PanelLeft from '@lucide/svelte/icons/panel-left';
	import PanelRight from '@lucide/svelte/icons/panel-right';
	import Plus from '@lucide/svelte/icons/plus';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import X from '@lucide/svelte/icons/x';
	import { tick, untrack } from 'svelte';
	import { copyText } from '../../clipboard';
	import { CommentPins } from '../../docs/commentPins.svelte';
	import { revealInFolds } from '../../docs/folds';
	import { keymap } from '../../keys/keymap.svelte';
	import { canUseTemplates } from '../../roles';
	import { anchored } from '../../popover';
	import { session } from '../../session.svelte';
	import {
		createPage,
		deletePage,
		ensureMainPage,
		setPageTitle,
		subpagesOf
	} from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import { wikiMain } from '../../sync/pages';
	import { toasts } from '../../toasts.svelte';
	import { typing } from '../../ui.svelte';
	import { uuid } from '../../uuid';
	import { pageHref, wikiName } from '../../wiki';
	import DocEditor from '../doc/DocEditor.svelte';
	import LinkedFrom from '../doc/LinkedFrom.svelte';
	import Avatar from '../task/Avatar.svelte';
	import WikiContents from './WikiContents.svelte';
	import WikiTree from './WikiTree.svelte';

	interface Props {
		ws: string;
		/** The project whose wiki this is, or null for the workspace's. */
		projectId: string | null;
		/** The page asked for; the main page when it is none, or not here. */
		pageId: string | null;
		/** The page's title is the page's h1, or an h2 under a project's. */
		heading?: 'h1' | 'h2';
		/** Shows a page; resolves once it is shown. */
		onopen: (pageId: string) => Promise<void> | void;
	}

	let { ws, projectId, pageId, heading = 'h1', onopen }: Props = $props();

	// The page's width: as dragged, up to the space there is, so the edge never
	// runs off the screen. A phone always uses all of it.
	let leafWidth = $state(0);
	const widest = $derived(Math.max(WIKI_PAGE.min, leafWidth || WIKI_PAGE.max));
	// With the panel on its right hidden, all the room there is, until dragged.
	const pageWidth = $derived(
		Math.min(layout.wikiSideOpen ? layout.wikiPageWidth : (layout.wikiPageAlone ?? widest), widest)
	);
	function resizePage(px: number) {
		const wanted = Math.min(px, widest);
		if (layout.wikiSideOpen) layout.setWikiPageWidth(wanted);
		else layout.setWikiPageAlone(wanted >= widest ? null : wanted);
	}
	let leafHeight = $state(0);
	let barHeight = $state(0);
	let leaf = $state<HTMLElement>();
	let doc = $state<HTMLElement>();

	// A link to a section of a page (`/wiki` in a comment) names its heading after
	// the "#": once the page has drawn, go there.
	$effect(() => {
		const wanted = route.url.hash.slice(1);
		void pageId;
		if (!wanted) return;
		let left = 40;
		const look = setInterval(() => {
			const found = [
				...document.querySelectorAll<HTMLElement>('.wiki .doc-body :is(h1, h2, h3, h4, h5, h6)')
			].find((h) => slugOf(h.textContent ?? '') === decodeURIComponent(wanted));
			if (found) {
				revealInFolds(found);
				found.scrollIntoView({ block: 'start' });
			}
			if (found || --left === 0) clearInterval(look);
		}, 100);
		return () => clearInterval(look);
	});
	const replica = $derived(engine.replica(ws));
	const project = $derived(projectId ? replica?.project(projectId) : null);
	const mainId = $derived(wikiMain(ws, projectId));
	const mainTitle = $derived(wikiName(project));
	const pages = $derived(replica?.pages(projectId) ?? []);
	const page = $derived(pages.find((p) => p.id === pageId) ?? pages.find((p) => p.id === mainId));
	const currentId = $derived(page?.id ?? mainId);
	const isMain = $derived(currentId === mainId);
	const titleOf = (id: string) => {
		if (id === mainId) return mainTitle;
		return pages.find((p) => p.id === id)?.title.trim() || 'Untitled';
	};

	// As far as anyone can tell, a wiki's main page exists from the start.
	$effect(() => {
		if (ws && replica && !replica.get('page', mainId)) void ensureMainPage(ws, projectId);
	});

	// The path to the page, from the main page down.
	const path = $derived.by(() => {
		const out: string[] = [];
		for (let at: string | null = currentId; at && out.length < 64;) {
			out.unshift(at);
			at = pages.find((p) => p.id === at)?.parentId ?? null;
		}
		return out;
	});

	// The tree, folded away or not, remembered on this device; on a phone
	// it opens as a sheet instead.
	const TREE = 'mila.wiki.tree';
	let treeOpen = $state(read() !== 'closed');
	function read(): string | null {
		try {
			return localStorage.getItem(TREE);
		} catch {
			return null;
		}
	}
	const phoneQuery = matchMedia('(max-width: 720px)');
	let phone = $state(phoneQuery.matches);
	$effect(() => {
		const update = (e: MediaQueryListEvent) => (phone = e.matches);
		phoneQuery.addEventListener('change', update);
		return () => phoneQuery.removeEventListener('change', update);
	});
	// The panel on the page's right takes what is left over, if that is enough.
	const sideShown = $derived(
		!phone && layout.wikiSideOpen && leafWidth - pageWidth >= WIKI_SIDE_MIN
	);
	// Offered whenever it is not showing and could: showing it makes the room.
	const sideOffered = $derived(!phone && !sideShown && leafWidth >= WIKI_PAGE.min + WIKI_SIDE_MIN);
	let showSideButton = $state<HTMLButtonElement>();
	let hideSideButton = $state<HTMLButtonElement>();
	async function toggleSide() {
		if (sideShown) {
			layout.setWikiSideOpen(false);
			await tick();
			showSideButton?.focus();
			return;
		}
		layout.setWikiSideOpen(true);
		if (leafWidth - layout.wikiPageWidth < WIKI_SIDE_MIN) {
			layout.setWikiPageWidth(leafWidth - WIKI_SIDE_MIN);
		}
		await tick();
		hideSideButton?.focus();
	}
	// The comments on the page: beside it when there is
	// room, over its edge when not, and in a sheet on a phone.
	const pins = new CommentPins(showComments);
	let commentsOver = $state(false);
	let commentsSheet = $state<HTMLDialogElement>();
	let commentsSheetOpen = $state(false);
	const commentsShown = $derived(
		commentsOver || (sideShown && layout.wikiSide === 'comments') || commentsSheetOpen
	);
	function showComments() {
		if (phone) {
			commentsSheetOpen = true;
			void tick().then(() => {
				if (commentsSheet && !commentsSheet.open) commentsSheet.showModal();
			});
		} else if (leafWidth - layout.wikiPageWidth >= WIKI_SIDE_MIN) {
			layout.setWikiSideOpen(true);
			layout.setWikiSide('comments');
		} else {
			commentsOver = true;
		}
	}
	function toggleComments() {
		if (commentsOver) commentsOver = false;
		else if (sideShown && layout.wikiSide === 'comments') layout.setWikiSide('contents');
		else showComments();
	}
	const openThreads = $derived.by(() => {
		const r = replica;
		if (!r) return [];
		const live = new Set(r.pageComments(currentId).map((c) => c.threadId));
		return r.pageThreads(currentId).filter((t) => !t.resolvedAt && live.has(t.id));
	});
	// Each page's threads are drawn in its text; another page starts afresh.
	let pinsOf = '';
	$effect(() => {
		const id = currentId;
		const list = openThreads;
		untrack(() => {
			if (id !== pinsOf) {
				pinsOf = id;
				pins.reset();
				commentsOver = false;
				threadAsked = '';
			}
			pins.pins = list.map((t) => ({ id: t.id, anchor: t.anchor, quote: t.quote }));
		});
	});
	// A link from a notification names a thread: open it once its page is showing.
	let threadAsked = '';
	$effect(() => {
		const wanted = route.url.searchParams.get('thread');
		if (!wanted || wanted === threadAsked || !replica) return;
		if (!replica.pageThreads(currentId).some((t) => t.id === wanted)) return;
		const ready = pins.ready;
		untrack(() => {
			pins.active = wanted;
			if (threadAsked !== wanted) showComments();
			if (ready) {
				threadAsked = wanted;
				pins.reveal?.(wanted);
			}
		});
	});
	// Page templates: an empty page offers them.
	let pageEditor = $state<{
		fill(md: string): boolean;
		clear(): void;
		currentMarkdown(): string | null;
		pageStyle(): { font: string | null; size: string | null };
		setPageStyle(font: string | null, size: string | null): void;
	}>();
	// The page's own font and size, from its menu.
	let typeOpen = $state(false);
	let typeNow = $state<{ font: string | null; size: string | null }>({ font: null, size: null });
	let pageEmpty = $state(false);
	let savingTemplate = $state(false);
	const mayUseTemplates = $derived(!!session.workspace && canUseTemplates(session.workspace.role));
	const pageTemplates = $derived(
		mayUseTemplates ? (replica?.templates().filter((t) => t.kind === 'page') ?? []) : []
	);
	async function useTemplate(templateId: string) {
		const editor = pageEditor;
		if (!editor) return;
		const { fillPageFromTemplate } = await import('../../templateActions');
		await fillPageFromTemplate(ws, templateId, {
			pageId: currentId,
			projectId,
			isMain,
			title: draft,
			fill: (md) => editor.fill(md),
			clear: () => editor.clear(),
			current: () => editor.currentMarkdown()
		});
	}
	async function newPageFrom(templateId: string) {
		templatesMenu?.hidePopover();
		const { newPageForTemplate } = await import('../../templateActions');
		const address = await newPageForTemplate(ws, templateId, { projectId, parentId: currentId });
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- a page's address, made by pageHref
		await goto(address);
	}
	// A page made from a template arrives with the template in its address: fill it
	// once its editor is ready, and take the template out of the address.
	let templateUsed = '';
	$effect(() => {
		const wanted = route.url.searchParams.get('template');
		if (!wanted || !pageEmpty || !pageEditor) return;
		if (route.url.searchParams.get('page') !== currentId || templateUsed === currentId) return;
		untrack(() => {
			templateUsed = currentId;
			// Only a page this tab just made for the template fills from it.
			if (takeTemplateFill(currentId, wanted)) void useTemplate(wanted);
			const next = new URL(route.url.href);
			next.searchParams.delete('template');
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- this page's own address
			void goto(next.pathname + next.search, { replaceState: true });
		});
	});
	const templatesMenuId = `page-templates-${uuid()}`;
	let templatesMenu = $state<HTMLElement>();
	let sheet = $state<HTMLDialogElement>();
	let showButton = $state<HTMLButtonElement>();
	let hideButton = $state<HTMLButtonElement>();
	async function toggleTree() {
		if (phone) {
			if (sheet?.open) sheet.close();
			else sheet?.showModal();
			return;
		}
		treeOpen = !treeOpen;
		try {
			localStorage.setItem(TREE, treeOpen ? 'open' : 'closed');
		} catch {
			// Kept for this visit only.
		}
		await tick();
		(treeOpen ? hideButton : showButton)?.focus();
	}
	function keys(e: KeyboardEvent) {
		if (
			keymap.match('anywhere', e, { field: typing(e), handled: e.defaultPrevented }) === 'wiki.tree'
		) {
			e.preventDefault();
			void toggleTree();
		}
	}

	function open(id: string) {
		if (sheet?.open) sheet.close();
		return onopen(id);
	}

	// The title, saved as it is typed.
	let draft = $state('');
	let titleInput = $state<HTMLInputElement>();
	let saving: ReturnType<typeof setTimeout> | undefined;
	// It follows the page shown, and a rename from another device while
	// the title is not being edited; never what is being typed.
	let titleOfShown = '';
	$effect(() => {
		const id = page?.id ?? '';
		const title = page?.title ?? '';
		untrack(() => {
			if (id !== titleOfShown) {
				// Another page: what was typed for the last one is saved first.
				if (saving) saveTitle();
				draft = title;
			} else if (document.activeElement !== titleInput) draft = title;
			titleOfShown = id;
			typedFor = id;
		});
	});
	// The page the title being typed belongs to, whatever is shown by the
	// time it is saved.
	let typedFor = '';
	function saveTitle() {
		clearTimeout(saving);
		const p = pages.find((x) => x.id === typedFor);
		if (p && p.id !== mainId && draft.trim() !== p.title) void setPageTitle(ws, p.id, draft);
	}

	let source = $state(false);
	// The page's history, in a dialog loaded when it opens.
	let historyOpen = $state(false);
	let here = $state<string[]>([]);
	const people = $derived(
		here.filter((id) => id !== session.user?.id).map((id) => ({ id, ...session.personOf(ws, id) }))
	);

	async function newPage() {
		const id = await createPage(ws, projectId, currentId, '');
		// The title to type is the new page's, once it is shown.
		await open(id);
		await tick();
		titleInput?.focus();
	}

	const menuId = `page-menu-${uuid()}`;
	let menu = $state<HTMLElement>();
	async function copyLink() {
		menu?.hidePopover();
		await copyText(location.origin + pageHref({ id: currentId, projectId }));
		toasts.show('Link copied');
	}
	/** A page's right-click menu, in the tree: its items are menus.ts's. */
	function pageMenu(id: string): MenuSource {
		const show = async (then?: () => void) => {
			await open(id);
			await tick();
			then?.();
		};
		return () =>
			import('../../menus').then((m) =>
				m.pageMenu(ws, {
					id,
					main: id === mainId,
					title: titleOf(id),
					href: pageHref({ id, projectId }),
					templates: mayUseTemplates,
					open: () => show(),
					newUnder: async () => {
						const made = await createPage(ws, projectId, id, '');
						await open(made);
						await tick();
						titleInput?.focus();
					},
					rename: () => show(() => (titleInput?.focus(), titleInput?.select())),
					history: () => show(() => (historyOpen = true)),
					template: () => show(() => (savingTemplate = true)),
					remove: () => show(() => void remove())
				})
			);
	}

	async function remove() {
		menu?.hidePopover();
		const p = page;
		if (!p || isMain) return;
		const name = titleOf(p.id);
		const under = subpagesOf(pages, p.id).length;
		const back = p.parentId ?? mainId;
		const { done, undo } = engine.track(() => deletePage(ws, p.id));
		await done;
		open(back);
		toasts.show(
			under
				? `Deleted "${name}" and ${under} ${under === 1 ? 'subpage' : 'subpages'}`
				: `Deleted "${name}"`,
			{
				onundo: () =>
					void undo().then((ok) => {
						if (ok) open(p.id);
					})
			}
		);
	}
</script>

<svelte:window onkeydown={keys} />

<div
	class="wiki"
	class:folded={!treeOpen || phone}
	style:grid-template-columns={treeOpen && !phone
		? `${layout.wikiTreeWidth}px minmax(0, 1fr)`
		: null}
>
	{#if treeOpen && !phone}
		<nav class="pane" id="wiki-pages" aria-label={`${mainTitle} pages`}>
			<div class="edge">
				<Splitter
					label="Resize the list of pages"
					controls="wiki-pages"
					value={layout.wikiTreeWidth}
					min={WIKI_TREE.min}
					max={WIKI_TREE.max}
					grows="right"
					onresize={(px) => layout.setWikiTreeWidth(px)}
				/>
			</div>
			<div class="panehead">
				<span class="name">{mainTitle}</span>
				<button
					class="icon"
					type="button"
					aria-label={`New page under ${titleOf(currentId)}`}
					title="New page"
					onclick={newPage}><Plus size={15} /></button
				>
				{#if pageTemplates.length}
					<button
						class="icon"
						type="button"
						popovertarget={templatesMenuId}
						aria-label="New page from a template"
						title="New page from a template"><LayoutTemplate size={15} /></button
					>
				{/if}
				<button
					class="icon"
					type="button"
					aria-label="Hide pages"
					title="Hide pages"
					aria-expanded="true"
					bind:this={hideButton}
					onclick={toggleTree}><PanelLeft size={15} /></button
				>
			</div>
			<div class="tree">
				<WikiTree
					{ws}
					{pages}
					{mainId}
					{mainTitle}
					current={currentId}
					onopen={open}
					menuOf={pageMenu}
					label="Pages"
				/>
			</div>
			<p class="hint">Drag pages to move them, or nest one under another.</p>
		</nav>
	{/if}
	<section
		class="leaf"
		aria-label={titleOf(currentId)}
		bind:this={leaf}
		bind:clientWidth={leafWidth}
		bind:clientHeight={leafHeight}
	>
		<div class="bar" bind:offsetHeight={barHeight}>
			{#if !treeOpen || phone}
				<button
					class="icon"
					type="button"
					aria-label="Show pages"
					title="Show pages"
					aria-expanded="false"
					bind:this={showButton}
					onclick={toggleTree}><PanelLeft size={15} /></button
				>
			{/if}
			<nav class="crumbs" aria-label="Page path">
				{#each path as id, i (id)}
					{#if i < path.length - 1}
						<button type="button" onclick={() => open(id)}>{titleOf(id)}</button>
						<span class="sep" aria-hidden="true">/</span>
					{:else}
						<span class="here-crumb" aria-current="page">{titleOf(id)}</span>
					{/if}
				{/each}
			</nav>
			{#if people.length}
				<span class="people">
					<span class="faces" aria-hidden="true">
						{#each people as p (p.id)}<Avatar name={p.name} src={p.avatar} size={22} />{/each}
					</span>
					<span class="said">
						{people.map((p) => p.name).join(' and ')}
						{people.length === 1 ? 'is' : 'are'} here
					</span>
				</span>
			{/if}
			<button
				class="tool"
				type="button"
				aria-pressed={commentsShown}
				aria-label={openThreads.length
					? `Comments on this page, ${openThreads.length} open`
					: 'Comments on this page'}
				title="Show the comments on this page"
				onclick={toggleComments}
				><MessageSquare size={15} />{#if openThreads.length}<span>{openThreads.length}</span
					>{/if}</button
			>
			<button
				class="tool"
				type="button"
				aria-pressed={source}
				title="Show the page as Markdown"
				onclick={() => (source = !source)}><FileCode size={15} /><span>Markdown</span></button
			>
			<button
				class="icon"
				type="button"
				popovertarget={menuId}
				aria-label="More for this page"
				title="More"><Ellipsis size={16} /></button
			>
			{#if sideOffered}
				<button
					class="icon"
					type="button"
					aria-label="Show contents and pages"
					title="Show contents and pages"
					aria-expanded="false"
					bind:this={showSideButton}
					onclick={toggleSide}><PanelRight size={15} /></button
				>
			{/if}
		</div>
		<div class="beside">
			<div
				class="doc"
				id="wiki-page"
				bind:this={doc}
				style:flex={phone ? null : `0 1 ${pageWidth}px`}
			>
				{#if !phone}
					<div class="edge">
						<Splitter
							label="Resize the page"
							controls="wiki-page"
							value={pageWidth}
							min={WIKI_PAGE.min}
							max={widest}
							grows="right"
							grip
							reset={layout.wikiSideOpen ? WIKI_PAGE.initial : widest}
							onresize={resizePage}
						/>
					</div>
				{/if}
				{#if isMain}
					<svelte:element this={heading} class="title main"
						>{mainTitle}<span class="tag">Main page</span></svelte:element
					>
				{:else}
					<svelte:element this={heading} class="visually-hidden"
						>{titleOf(currentId)}</svelte:element
					>
					<input
						class="title"
						aria-label="Page title"
						placeholder="Untitled"
						maxlength="200"
						bind:this={titleInput}
						bind:value={draft}
						oninput={() => {
							clearTimeout(saving);
							saving = setTimeout(saveTitle, 600);
						}}
						onblur={saveTitle}
						onkeydown={(e) => {
							if (e.key === 'Enter') {
								e.preventDefault();
								saveTitle();
								document.querySelector<HTMLElement>('.wiki .doc-body')?.focus();
							}
						}}
					/>
				{/if}
				{#key currentId}
					<DocEditor
						{ws}
						target={{ kind: 'page', id: currentId }}
						markdown={page?.body ?? ''}
						label="Page"
						placeholder="Start writing, or type / for a list, table or image."
						page
						showHere={false}
						bind:source
						bind:here
						bind:empty={pageEmpty}
						bind:this={pageEditor}
						{pins}
					/>
				{/key}
				{#if pageEmpty && !source && pageTemplates.length}
					<div class="starters" role="group" aria-label="Start from a template">
						<span class="startlabel">Start from a template</span>
						{#each pageTemplates as t (t.id)}
							<button
								type="button"
								class="starter"
								title={`Fill this page from the template ${t.name}`}
								onclick={() => void useTemplate(t.id)}
							>
								<LayoutTemplate size={15} /><span>{t.name}</span>
							</button>
						{/each}
					</div>
				{/if}
				<LinkedFrom {ws} id={currentId} />
			</div>
			{#if sideShown}
				<div
					class="side"
					role="group"
					aria-label="Beside the page"
					style:top={`${barHeight}px`}
					style:height={`${leafHeight - barHeight}px`}
				>
					<div class="sidehead">
						<button
							class="tool"
							type="button"
							aria-pressed={layout.wikiSide === 'contents'}
							onclick={() => layout.setWikiSide('contents')}>Contents</button
						>
						<button
							class="tool"
							type="button"
							aria-pressed={layout.wikiSide === 'pages'}
							onclick={() => layout.setWikiSide('pages')}>Pages</button
						>
						<button
							class="tool"
							type="button"
							aria-pressed={layout.wikiSide === 'comments'}
							onclick={() => layout.setWikiSide('comments')}
							>Comments{#if openThreads.length}<span class="count">{openThreads.length}</span
								>{/if}</button
						>
						<button
							class="icon hideside"
							type="button"
							aria-label="Hide contents and pages"
							title="Hide contents and pages"
							aria-expanded="true"
							bind:this={hideSideButton}
							onclick={toggleSide}><PanelRight size={15} /></button
						>
					</div>
					<div class="sidebody">
						{#if layout.wikiSide === 'contents'}
							<WikiContents root={doc} scroller={leaf} {barHeight} {source} />
						{:else if layout.wikiSide === 'comments'}
							{#await import('./PageComments.svelte') then { default: PageComments }}
								<PageComments {ws} pageId={currentId} {projectId} {pins} />
							{/await}
						{:else}
							<WikiTree
								{ws}
								{pages}
								{mainId}
								{mainTitle}
								current={currentId}
								onopen={open}
								menuOf={pageMenu}
								label="Pages"
							/>
						{/if}
					</div>
				</div>
			{/if}
			{#if commentsOver && !sideShown}
				<div
					class="side over"
					role="group"
					aria-label="Comments"
					style:top={`${barHeight}px`}
					style:height={`${leafHeight - barHeight}px`}
				>
					<div class="sidehead">
						<span class="overname">Comments</span>
						<button
							class="icon hideside"
							type="button"
							aria-label="Close comments"
							title="Close comments"
							onclick={() => (commentsOver = false)}><X size={15} /></button
						>
					</div>
					<div class="sidebody">
						{#await import('./PageComments.svelte') then { default: PageComments }}
							<PageComments {ws} pageId={currentId} {projectId} {pins} />
						{/await}
					</div>
				</div>
			{/if}
		</div>
	</section>
</div>

<div class="menu" popover id={menuId} bind:this={menu} use:anchored>
	<button
		type="button"
		class="option"
		onclick={() => {
			menu?.hidePopover();
			void newPage();
		}}><Plus size={15} />New page under this one</button
	>
	<button type="button" class="option" onclick={copyLink}><Link size={15} />Copy link</button>
	{#if mayUseTemplates}
		<button
			type="button"
			class="option"
			onclick={() => {
				menu?.hidePopover();
				savingTemplate = true;
			}}><LayoutTemplate size={15} />Save as a template</button
		>
	{/if}
	<button
		type="button"
		class="option"
		onclick={() => {
			menu?.hidePopover();
			typeNow = pageEditor?.pageStyle() ?? { font: null, size: null };
			typeOpen = true;
		}}><Type size={15} />Font and size</button
	>
	<button
		type="button"
		class="option"
		onclick={() => {
			menu?.hidePopover();
			historyOpen = true;
		}}><History size={15} />History</button
	>
	{#if isMain}
		<p class="note">The main page goes with its {projectId ? 'project' : 'workspace'}.</p>
	{:else}
		<button type="button" class="option danger" onclick={remove}><Trash2 size={15} />Delete</button>
	{/if}
</div>

<div class="menu" popover id={templatesMenuId} bind:this={templatesMenu} use:anchored>
	{#each pageTemplates as t (t.id)}
		<button type="button" class="option" onclick={() => void newPageFrom(t.id)}
			><LayoutTemplate size={15} />{t.name}</button
		>
	{/each}
</div>

{#if savingTemplate}
	{#await import('../templates/SaveTemplate.svelte') then { default: SaveTemplate }}
		<SaveTemplate
			bind:open={savingTemplate}
			{ws}
			kind="page"
			pageText={() => pageEditor?.currentMarkdown() ?? null}
			sourceId={currentId}
			sourceName={titleOf(currentId)}
		/>
	{/await}
{/if}

{#if typeOpen}
	{#await import('./PageType.svelte') then { default: PageType }}
		<PageType
			bind:open={typeOpen}
			font={typeNow.font}
			size={typeNow.size}
			onchange={(font, size) => {
				typeNow = { font, size };
				pageEditor?.setPageStyle(font, size);
			}}
		/>
	{/await}
{/if}

{#if historyOpen}
	{#await import('./PageHistory.svelte') then { default: PageHistory }}
		<PageHistory {ws} pageId={currentId} title={titleOf(currentId)} bind:open={historyOpen} />
	{/await}
{/if}

<dialog class="sheet" bind:this={sheet} aria-label={`${mainTitle} pages`}>
	<div class="sheethead">
		<h2>Pages</h2>
		<button class="icon" type="button" aria-label="Close" onclick={() => sheet?.close()}
			><X size={16} /></button
		>
	</div>
	<WikiTree {ws} {pages} {mainId} {mainTitle} current={currentId} onopen={open} label="Pages" />
	<button class="add" type="button" onclick={newPage}><Plus size={15} />New page</button>
</dialog>

{#if commentsSheetOpen}
	<dialog
		class="sheet"
		bind:this={commentsSheet}
		aria-label="Comments"
		onclose={() => (commentsSheetOpen = false)}
	>
		<div class="sheethead">
			<h2>Comments</h2>
			<button class="icon" type="button" aria-label="Close" onclick={() => commentsSheet?.close()}
				><X size={16} /></button
			>
		</div>
		{#await import('./PageComments.svelte') then { default: PageComments }}
			<PageComments {ws} pageId={currentId} {projectId} {pins} />
		{/await}
	</dialog>
{/if}

<style>
	.wiki {
		display: grid;
		grid-template-columns: 240px minmax(0, 1fr);
		width: 100%;
		height: 100%;
		min-height: 0;
	}
	.wiki.folded {
		grid-template-columns: minmax(0, 1fr);
	}
	.pane {
		position: relative;
		display: flex;
		flex-direction: column;
		min-height: 0;
		border-right: var(--border-width) solid var(--border);
	}
	.panehead {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		padding: var(--space-3) var(--space-2) var(--space-1) var(--space-4);
	}
	.name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.tree {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
	}
	.hint {
		padding: var(--space-2) var(--space-4) var(--space-3);
		border-top: var(--border-width) solid var(--border);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.leaf {
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		overflow-y: auto;
	}
	.bar {
		position: sticky;
		top: 0;
		/* Over the page's edge, which runs the page's whole length. */
		z-index: 6;
		flex: none;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-height: calc(46px * var(--control-scale));
		padding: var(--space-1) var(--space-3) var(--space-1) var(--space-5);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-0);
	}
	.wiki.folded .bar {
		padding-left: var(--space-3);
	}
	.crumbs {
		flex: 1;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 2px;
		min-width: 0;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.crumbs button {
		min-height: max(var(--tap-min), 26px);
		padding: 0 var(--space-1);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: inherit;
		font: inherit;
		cursor: pointer;
	}
	.crumbs button:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.sep {
		color: var(--text-muted);
	}
	.here-crumb {
		padding: 0 var(--space-1);
		color: var(--text-primary);
		font-weight: 550;
	}
	.people {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.faces {
		display: inline-flex;
	}
	.faces > :global(* + *) {
		margin-left: -5px;
	}
	.icon,
	.tool {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-1);
		min-width: max(var(--tap-min), 30px);
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-1);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.tool {
		padding: 0 var(--space-2);
	}
	.icon:hover,
	.tool:hover,
	.tool[aria-pressed='true'] {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	/* Where a splitter sits: on the right edge of what it resizes. */
	.edge {
		position: absolute;
		top: 0;
		right: 0;
		bottom: 0;
	}
	/* The page has an edge you can see: the space beyond it is a step darker. */
	.beside {
		display: flex;
		flex: 1 0 auto;
		align-items: stretch;
		min-width: 0;
		background: var(--surface-1);
	}
	.doc {
		position: relative;
		flex: 1 1 auto;
		min-width: 0;
		padding: var(--space-6) var(--space-7) var(--space-7);
		border-right: var(--border-width) solid var(--border-strong);
		background: var(--surface-0);
	}
	/* A heading gone to stops clear of the bar. */
	.doc :global(.doc-body :is(h1, h2, h3, h4, h5, h6)) {
		scroll-margin-top: calc(46px * var(--control-scale) + var(--space-3));
	}
	.side {
		position: sticky;
		display: flex;
		flex: 1 1 0;
		flex-direction: column;
		align-self: flex-start;
		min-width: 0;
		overflow: hidden;
	}
	.starters {
		display: grid;
		gap: var(--space-1);
		margin-top: var(--space-4);
		padding-top: var(--space-3);
		border-top: var(--border-width) solid var(--border);
	}
	.startlabel {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.starter {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-primary);
		font-size: var(--text-sm);
		text-align: left;
		cursor: pointer;
	}
	.starter:hover {
		background: var(--surface-2);
	}
	/* Over the page's edge: there is no room beside it. */
	.side.over {
		flex: 0 0 min(340px, 100%);
		margin-left: calc(-1 * min(340px, 100%));
		z-index: 5;
		border-left: var(--border-width) solid var(--border-strong);
		background: var(--surface-1);
	}
	.overname {
		align-self: center;
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.count {
		margin-left: var(--space-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.sidehead {
		display: flex;
		flex: none;
		flex-wrap: wrap;
		gap: var(--space-1);
		padding: var(--space-3) var(--space-2) var(--space-2);
	}
	/* Three tabs and the hide button fit a narrow panel on one line. */
	.sidehead .tool {
		padding: 0 var(--space-1);
	}
	.hideside {
		margin-left: auto;
	}
	.sidebody {
		flex: 1;
		max-width: 360px;
		min-height: 0;
		padding: 0 var(--space-3) var(--space-3) var(--space-2);
		overflow-y: auto;
	}
	.title {
		display: block;
		width: 100%;
		margin-bottom: var(--space-3);
		padding: 0;
		border: none;
		background: none;
		color: var(--text-primary);
		font-family: var(--font-heading);
		font-size: var(--text-3xl);
		font-weight: var(--heading-weight);
		line-height: 1.2;
	}
	.title:focus-visible {
		outline: none;
		box-shadow: 0 2px 0 var(--accent);
	}
	.title.main {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
	}
	.tag {
		padding: 2px var(--space-2);
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		color: var(--text-secondary);
		font-family: var(--font-ui);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	.menu .option {
		width: 100%;
		border: none;
		background: none;
		color: var(--text-primary);
		font-size: var(--text-sm);
		text-align: left;
		min-height: max(var(--tap-min), 30px);
	}
	.menu .danger {
		color: var(--danger);
	}
	.menu .note {
		max-width: 240px;
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.sheet {
		width: 100%;
		max-width: 100%;
		max-height: 80vh;
		margin: auto 0 0;
		padding: var(--space-3) 0 var(--space-4);
		border: none;
		border-radius: var(--radius-lg) var(--radius-lg) 0 0;
		background: var(--surface-0);
		color: var(--text-primary);
	}
	.sheet::backdrop {
		background: var(--scrim-sheet);
	}
	.sheethead {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 var(--space-3) var(--space-2) var(--space-5);
	}
	.sheethead h2 {
		font-size: var(--text-md);
	}
	.add {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--tap-min);
		margin: var(--space-2) var(--space-4) 0;
		padding: 0 var(--space-2);
		border: none;
		background: none;
		color: var(--accent-text);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	@media (max-width: 720px) {
		.doc {
			padding: var(--space-4);
		}
		.tool span {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
			white-space: nowrap;
		}
		.said {
			display: none;
		}
	}
</style>
