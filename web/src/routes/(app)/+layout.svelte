<script lang="ts">
	import { goto, onNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tick, untrack, type Component } from 'svelte';
	import BookOpen from '@lucide/svelte/icons/book-open';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import CircleQuestionMark from '@lucide/svelte/icons/circle-question-mark';
	import ChartColumn from '@lucide/svelte/icons/chart-column';
	import Clock from '@lucide/svelte/icons/clock';
	import Inbox from '@lucide/svelte/icons/inbox';
	import LogOut from '@lucide/svelte/icons/log-out';
	import Palette from '@lucide/svelte/icons/palette';
	import PanelLeft from '@lucide/svelte/icons/panel-left';
	import Plus from '@lucide/svelte/icons/plus';
	import Settings from '@lucide/svelte/icons/settings';
	import Sun from '@lucide/svelte/icons/sun';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import { messageOf } from '#lib/api/client.js';
	import NotificationBell from '#lib/components/NotificationBell.svelte';
	import NotificationPanel from '#lib/components/NotificationPanel.svelte';
	import { notifications } from '#lib/notifications.svelte.js';
	import SidebarLink from '#lib/components/SidebarLink.svelte';
	import SyncDot from '#lib/components/SyncDot.svelte';
	import WorkspaceSwitcher from '#lib/components/WorkspaceSwitcher.svelte';
	import ProjectNav from '#lib/components/ProjectNav.svelte';
	import ViewNav from '#lib/components/ViewNav.svelte';
	import TabBar from '#lib/components/TabBar.svelte';
	import { selection } from '#lib/selection.svelte.js';
	import Splitter from '#lib/components/Splitter.svelte';
	import { DETAIL, layout, SIDEBAR } from '#lib/layout.svelte.js';
	import { arrive, sheet } from '#lib/motion.js';
	import { pullDown } from '#lib/swipe.js';
	import { scheduler } from '#lib/schedule.svelte.js';
	import { contextMenu } from '#lib/contextMenu.svelte.js';
	import { typing, ui } from '#lib/ui.svelte.js';
	import { ACTIONS } from '#lib/keys/actions.js';
	import { keymap } from '#lib/keys/keymap.svelte.js';
	import { openTask } from '#lib/taskLink.js';
	import { appPath, isOpenMessage } from '#lib/pushNotice.js';
	import { preferences } from '#lib/preferences.svelte.js';
	import { session } from '#lib/session.svelte.js';
	import { canUseTemplates } from '#lib/roles.js';
	import { isDesktop, keepLinksInWindow, setDesktopShortcut } from '#lib/desktop.js';
	import { signOutHere, warnOfUnsent } from '#lib/signout.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { followOffered, followWorkspaceTheme, themeNotice } from '#lib/theme/follow.svelte.js';
	import { presets } from '#lib/theme/presets.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';
	import { announce } from '#lib/announce.svelte.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	// The sidebar's collapse and width, the task panel's width and whether
	// it fills the page are remembered per device (layout.svelte.ts).
	const collapsed = $derived(layout.collapsed);

	// The palette is not part of a first load: it comes when the browser has a
	// moment, so opening it is still immediate, or at once if asked for first.
	let CommandPalette = $state.raw<Component | null>(null);
	// The schedule chooser isn't part of a first load either: it comes when the
	// browser has a moment, or at once if a row asks for it first.
	let SchedulePicker = $state.raw<Component | null>(null);
	const loadSchedule = () =>
		void import('#lib/components/SchedulePicker.svelte').then((m) => (SchedulePicker = m.default));
	$effect(() => {
		if (scheduler.target) untrack(loadSchedule);
	});
	$effect(() => {
		if (typeof requestIdleCallback !== 'function') return void setTimeout(loadSchedule, 1000);
		const idle = requestIdleCallback(loadSchedule);
		return () => cancelIdleCallback(idle);
	});
	const loadPalette = () =>
		void import('#lib/components/CommandPalette.svelte').then((m) => (CommandPalette = m.default));
	$effect(() => {
		if (ui.overlay === 'palette') untrack(loadPalette);
	});
	$effect(() => {
		if (typeof requestIdleCallback !== 'function') return void setTimeout(loadPalette, 1000);
		const idle = requestIdleCallback(loadPalette);
		return () => cancelIdleCallback(idle);
	});
	// Quick add and the shortcut sheet the same way: when the browser has a
	// moment, or at once if a key or button asks for one first.
	let QuickAdd = $state.raw<Component | null>(null);
	let ShortcutHelp = $state.raw<Component | null>(null);
	const loadQuickAdd = () =>
		void import('#lib/components/QuickAdd.svelte').then((m) => (QuickAdd = m.default));
	const loadHelp = () =>
		void import('#lib/components/ShortcutHelp.svelte').then((m) => (ShortcutHelp = m.default));
	$effect(() => {
		if (ui.overlay === 'quickadd') untrack(loadQuickAdd);
		else if (ui.overlay === 'help') untrack(loadHelp);
	});
	$effect(() => {
		const both = () => {
			loadQuickAdd();
			loadHelp();
		};
		if (typeof requestIdleCallback !== 'function') return void setTimeout(both, 1000);
		const idle = requestIdleCallback(both);
		return () => cancelIdleCallback(idle);
	});
	let resizing = $state(false);
	const openId = $derived(page.url.searchParams.get('task'));
	// Whether the person has a timer running, in any workspace on the device.
	const timing = $derived.by(() => {
		const me = session.user?.id;
		if (!me) return false;
		for (const r of engine.replicas.values()) if (r.runningTimer(me)) return true;
		return false;
	});
	// A workspace theme in use follows its workspace's copy; the theme
	// notice is the workspace's suggestion, or that the theme went away.
	$effect(followWorkspaceTheme);
	$effect(followOffered);
	// A theme made from Omarchy's keeps this version's copy, and comes when chosen on another device.
	$effect(() => presets.follow(preferences.current.customTheme));
	const notice = $derived(themeNotice());
	// The task panel loads when a task opens, or once the app is idle, so
	// it never weighs on a page's first load and opening a task stays quick.
	let TaskDetail = $state.raw<typeof import('#lib/components/task/TaskDetail.svelte').default>();
	const loadDetail = () =>
		import('#lib/components/task/TaskDetail.svelte').then((m) => (TaskDetail = m.default));
	$effect(() => {
		if (openId) void loadDetail();
	});
	$effect(() => {
		const idle = window.requestIdleCallback ?? ((fn: () => void) => setTimeout(fn, 1500));
		idle(() => void loadDetail());
	});

	// Opening a task moves focus into it, unless focus is already there or
	// on a field of the page (typing in quick add's project link, say), so
	// the next Tab reaches its fields. When the task closes, focus that
	// went nowhere else comes back to its row, or else to what opened it
	// (a calendar entry, say); if that is gone too (deleted from the
	// panel), to the list's current row, or the page.
	let pane = $state<HTMLElement>();
	let shown: string | null = null;
	let opener: HTMLElement | null = null;
	$effect(() => {
		const id = openId;
		const into = pane;
		untrack(() => {
			if (id && into && id !== shown && !into.contains(document.activeElement)) {
				const at = document.activeElement;
				opener = at instanceof HTMLElement && at !== document.body ? at : null;
				const typing = at?.closest('input, textarea, select, [contenteditable="true"]');
				if (!typing || phone || layout.expanded) into.focus();
			}
			const closed = !id ? shown : null;
			shown = id;
			if (closed) void tick().then(() => backToRow(closed));
		});
	});

	function backToRow(taskId: string) {
		const from = opener;
		opener = null;
		const focus = document.activeElement;
		const lost =
			!focus || focus === document.body || !focus.isConnected || focus.closest('[inert]');
		if (!lost) return;
		const main = document.getElementById('main');
		// A deleted task's row may still be leaving (its collapse runs).
		const gone = !session.workspace || !engine.replica(session.workspace.id)?.task(taskId);
		const others = `[data-task-id]:not([data-task-id="${CSS.escape(taskId)}"])`;
		// Still in the page and showing: not in a closed dialog or the panel.
		const usable =
			from?.isConnected &&
			!from.closest('[inert]') &&
			!pane?.contains(from) &&
			from.getClientRects().length > 0;
		const target =
			(gone ? null : main?.querySelector<HTMLElement>(`[data-task-id="${CSS.escape(taskId)}"]`)) ??
			(!gone && usable ? from : null) ??
			main?.querySelector<HTMLElement>(`${others}[tabindex="0"], [data-card][tabindex="0"]`) ??
			main?.querySelector<HTMLElement>(others) ??
			main;
		target?.focus();
	}

	// The connection going and coming back is said once, not shown as a
	// toast: the sync dot shows it all the time.
	let lastState = engine.state;
	$effect(() => {
		const now = engine.state;
		untrack(() => {
			if (now === 'offline' && lastState !== 'offline')
				announce('Offline. Changes are kept here and sync when the connection is back.');
			// Paused is said by the update toast, which stays on screen.
			else if (now !== 'offline' && now !== 'paused' && lastState === 'offline')
				announce(
					engine.pending
						? 'Back online. Syncing your changes.'
						: 'Back online. Everything is synced.'
				);
			lastState = now;
		});
	});

	// Moving between views: the new page rises a little and fades in (only
	// fades with reduced motion; arrive in motion.ts). The sidebar, tab bar
	// and task panel stay put. Opening a task or tidying the query is not a
	// new view.
	onNavigate((nav) => {
		if (!nav.to || nav.from?.url.pathname === nav.to.url.pathname) return;
		// A selection belongs to the page it was made on.
		selection.clear();
		nav.complete.then(
			() => {
				const main = document.getElementById('main');
				if (main) arrive(main);
			},
			() => {}
		);
	});

	$effect(() => {
		if (session.workspace) session.loadMembers(session.workspace.id);
	});

	// Links from notifications, emails and calendars carry the workspace
	// (?task=<id>&ws=<id>): switch to it if the user belongs to it, then
	// drop it from the URL; the task opens from ?task as usual. One from a
	// push or an email also names its notification (&n=<id>), which opening
	// it marks read.
	const linkedWorkspace = $derived(page.url.searchParams.get('ws'));
	const linkedNotice = $derived(page.url.searchParams.get('n'));
	$effect(() => {
		const n = linkedNotice;
		if (n) untrack(() => void notifications.readOpened(n));
	});
	$effect(() => {
		const ws = linkedWorkspace;
		if (ws) untrack(() => void followWorkspaceLink(ws));
	});

	async function followWorkspaceLink(ws: string) {
		if (!session.workspaces.some((w) => w.id === ws)) {
			// Maybe joined since the list was loaded.
			await session.loadWorkspaces().catch(() => {});
		}
		if (session.workspaces.some((w) => w.id === ws)) session.select(ws);
		else toasts.error('That link is for a workspace you are not a member of.');
		const u = new URL(page.url.href);
		if (u.searchParams.get('ws') !== ws) return;
		u.searchParams.delete('ws');
		u.searchParams.delete('n');
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- same page, one query parameter fewer
		await goto(u, { replace: true, reset: false });
	}

	// A click on a push notification: the service worker focuses this
	// window and asks it to go to the push's link (pushNotice.ts).
	$effect(() => {
		const sw = navigator.serviceWorker;
		if (!sw) return;
		const onMessage = (e: MessageEvent) => {
			if (!isOpenMessage(e.data)) return;
			// appPath already kept it to a path in this app.
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- a link from a push, like one from an email
			void goto(new URL(appPath(e.data.url), location.origin));
		};
		sw.addEventListener('message', onMessage);
		return () => sw.removeEventListener('message', onMessage);
	});

	// The app's keyboard: the commands that work anywhere (COMMANDS in
	// keys/commands.ts), as the person has them. Keys typed into a field,
	// an overlay or a dialog belong to it, except the palette's; so do
	// keys a focused row or card took first.
	function keys(e: KeyboardEvent) {
		const field =
			typing(e) || ui.overlay !== null || !!(e.target as Element | null)?.closest?.('dialog[open]');
		const id = keymap.match('anywhere', e, { field, handled: e.defaultPrevented });
		const run = id && ACTIONS[id];
		if (run) {
			e.preventDefault();
			void run();
		}
	}

	// One workspace switcher at a time: in the sidebar, or on phones (where
	// the sidebar is hidden) in a bar above the page.
	const phoneQuery = matchMedia('(max-width: 720px)');
	let phone = $state(phoneQuery.matches);
	$effect(() => {
		const update = (e: MediaQueryListEvent) => (phone = e.matches);
		phoneQuery.addEventListener('change', update);
		return () => phoneQuery.removeEventListener('change', update);
	});

	// The task panel as drawn: its column (below) caps the chosen width so
	// the list keeps 360px, and the resize edge works from what is drawn,
	// or a drag or arrow key would first have to use up the hidden part.
	let viewport = $state(innerWidth);
	const detailCap = $derived(viewport - (collapsed ? 56 : layout.sidebarWidth) - 360);
	const detailDrawn = $derived(Math.max(280, Math.min(layout.detailWidth, detailCap)));

	const nav = [
		{ id: 'inbox', href: resolve('/(app)'), label: 'Inbox', icon: Inbox },
		{ id: 'today', href: resolve('/(app)/today'), label: 'My Day', icon: Sun },
		{ id: 'upcoming', href: resolve('/(app)/upcoming'), label: 'Upcoming', icon: CalendarDays },
		{ id: 'wiki', href: resolve('/(app)/wiki'), label: 'Workspace Wiki', icon: BookOpen },
		{ id: 'time', href: resolve('/(app)/time'), label: 'Time', icon: Clock },
		{ id: 'reports', href: resolve('/(app)/reports'), label: 'Reports', icon: ChartColumn }
	];
	// The sidebar's parts in the user's order, without the hidden ones;
	// neighboring links share one list.
	const sidebar = $derived.by(() => {
		const out: ({ kind: 'links'; items: typeof nav } | { kind: 'views' | 'projects' })[] = [];
		for (const id of preferences.current.sidebarOrder) {
			if (preferences.current.sidebarHidden.includes(id)) continue;
			// Guests never see the workspace's wiki, only their projects'.
			if (id === 'wiki' && session.workspace?.role === 'guest') continue;
			// Nor have they an inbox.
			if (id === 'inbox' && session.workspace?.role === 'guest') continue;
			if (id === 'views' || id === 'projects') {
				out.push({ kind: id });
				continue;
			}
			const item = nav.find((n) => n.id === id)!;
			const last = out.at(-1);
			if (last?.kind === 'links') last.items.push(item);
			else out.push({ kind: 'links', items: [item] });
		}
		return out;
	});
	// On a phone the task covers the screen like a dialog: what is under it
	// is out of reach for Tab and screen readers until it closes.
	const sheetOpen = $derived(phone && openId !== null);
	const selecting = $derived(
		!!session.workspace &&
			selection.on(session.workspace.id) &&
			(selection.picking || selection.shown().length > 0)
	);
	const settingsHref = resolve('/(app)/settings');
	const inSettings = $derived(page.url.pathname.startsWith(settingsHref));
	const helpHref = resolve('/(app)/help');
	const inHelp = $derived(page.url.pathname.startsWith(helpHref));

	$effect(keepLinksInWindow);

	// The desktop app's shortcut follows the person's keys, and opens Quick Add.
	$effect(() => {
		if (!isDesktop()) return;
		void setDesktopShortcut(keymap.active('desktop.quick-add')[0] ?? '').catch(() => {
			toasts.error(
				"That shortcut can't be used. It needs two modifier keys, and no other program using it. Try another in Settings, Keyboard."
			);
		});
		const open = () => ui.open('quickadd');
		window.addEventListener('mila-quick-add', open);
		return () => window.removeEventListener('mila-quick-add', open);
	});

	async function signOut(anyway = false) {
		try {
			if (!anyway && (await warnOfUnsent(() => void signOut(true)))) return;
			await signOutHere();
			await goto(resolve('/(auth)/login'), { replaceState: true });
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}
</script>

<svelte:window onkeydown={keys} bind:innerWidth={viewport} />
{#if CommandPalette}<CommandPalette />{/if}

<!-- The first stop for Tab: past the sidebar, straight to the page. -->
<a
	class="skip"
	href="#main"
	inert={sheetOpen}
	onclick={(e) => {
		// Focus without the router or a #main left in the address.
		e.preventDefault();
		document.getElementById('main')?.focus();
	}}>Skip to content</a
>
<!-- After the skip link, which must come before the page's first link
     (the sheet's All shortcuts, quick add's help link) for screen readers
     and axe to treat it as one. -->
{#if QuickAdd}<QuickAdd />{/if}
{#if ShortcutHelp}<ShortcutHelp />{/if}

<div
	class="shell"
	class:collapsed
	class:detail-open={openId}
	class:expanded={layout.expanded && !phone}
	class:resizing
	style:--sidebar-width={collapsed ? undefined : `${layout.sidebarWidth}px`}
	style:--detail-width={`${layout.detailWidth}px`}
>
	<nav class="sidebar" id="sidebar" aria-label="Main" inert={sheetOpen}>
		<!-- The part that scrolls; the resize edge stays put beside it. -->
		<div class="sidebar-body">
			<div class="top">
				{#if !phone}<WorkspaceSwitcher />{/if}
				<button
					class="icon"
					type="button"
					aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
					aria-expanded={!collapsed}
					onclick={() => layout.toggleSidebar()}
				>
					<PanelLeft size={16} />
				</button>
			</div>

			<div class="new-row">
				<button
					class="new-task"
					type="button"
					title={collapsed ? 'New task' : undefined}
					aria-keyshortcuts={keymap.aria('quick-add')}
					onclick={() => ui.open('quickadd')}
				>
					<Plus size={16} /><span class="name">New task</span>
					{#if keymap.label('quick-add') && !collapsed}<kbd aria-hidden="true"
							>{keymap.label('quick-add')}</kbd
						>{/if}
				</button>
				{#if !collapsed && session.workspace && canUseTemplates(session.workspace.role)}
					<!-- Loaded after the page, so no page's first load pays for it. -->
					{#await import('#lib/components/templates/TaskTemplateMenu.svelte') then { default: TaskTemplateMenu }}
						<TaskTemplateMenu ws={session.workspace.id} />
					{/await}
				{/if}
			</div>

			{#snippet bell()}
				{#if !phone}
					<li><NotificationBell unread={notifications.unread} target="notifications" /></li>
				{/if}
			{/snippet}
			<!-- The bell ends the first list of links, or stands alone when all are hidden. -->
			{#if !sidebar.some((p) => p.kind === 'links')}
				<ul class="links">{@render bell()}</ul>
			{/if}

			{#each sidebar as part, i (part.kind === 'links' ? `links-${i}` : part.kind)}
				{#if part.kind === 'links'}
					<ul class="links">
						{#each part.items as item (item.href)}
							<li>
								<SidebarLink
									href={item.href}
									label={item.label}
									icon={item.icon}
									current={page.url.pathname === item.href}
									{collapsed}
								/>
							</li>
						{/each}
						{#if part === sidebar.find((p) => p.kind === 'links')}{@render bell()}{/if}
					</ul>
				{:else if part.kind === 'views'}
					<ViewNav />
				{:else}
					<ProjectNav />
				{/if}
			{/each}

			{#if timing && !phone}
				<!-- Loaded only while a timer runs. -->
				{#await import('#lib/components/RunningTimer.svelte') then { default: RunningTimer }}
					<RunningTimer {collapsed} />
				{/await}
			{/if}

			<footer>
				{#if !phone}<SyncDot state={engine.state} pending={engine.pending} />{/if}
				<a
					class="footer-link"
					href={settingsHref}
					aria-current={inSettings ? 'page' : undefined}
					title={collapsed ? 'Settings' : session.user?.email}
					><Settings size={16} /><span class="name">{session.user?.name ?? 'Settings'}</span></a
				>
				<a
					class="footer-link"
					href={helpHref}
					aria-current={inHelp ? 'page' : undefined}
					title={collapsed ? 'Help' : undefined}
					><CircleQuestionMark size={16} /><span class="name">Help</span></a
				>
				<a
					class="footer-link"
					href={resolve('/(app)/trash')}
					title={collapsed ? 'Trash' : undefined}
					><Trash2 size={16} /><span class="name">Trash</span></a
				>
				{#if !session.local}
					<button
						class="footer-link"
						type="button"
						title={collapsed ? 'Sign out' : undefined}
						onclick={() => signOut()}
					>
						<LogOut size={16} /><span class="name">Sign out</span>
					</button>
				{/if}
				{#if import.meta.env.DEV}
					<a class="footer-link" href={resolve('/dev/ui')}
						><Palette size={16} /><span class="name">Style guide</span></a
					>
				{/if}
			</footer>
		</div>
		{#if !phone && !collapsed}
			<div class="sidebar-edge">
				<Splitter
					label="Resize sidebar"
					controls="sidebar"
					value={layout.sidebarWidth}
					min={SIDEBAR.min}
					max={SIDEBAR.max}
					grows="right"
					onresize={(px) => layout.setSidebarWidth(px)}
					ondragging={(on) => (resizing = on)}
				/>
			</div>
		{/if}
	</nav>

	<!-- tabindex: the skip link's target, and where focus goes when what had it is gone. -->
	<main id="main" tabindex="-1" inert={sheetOpen}>
		{#if phone}
			<div class="phone-bar">
				<WorkspaceSwitcher />
				<SyncDot state={engine.state} pending={engine.pending} compact />
				<NotificationBell unread={notifications.unread} target="notifications" variant="icon" />
				<a class="gear" href={helpHref} aria-label="Help" aria-current={inHelp ? 'page' : undefined}
					><CircleQuestionMark size={20} /></a
				>
				<a
					class="gear"
					href={settingsHref}
					aria-label="Settings"
					aria-current={inSettings ? 'page' : undefined}><Settings size={20} /></a
				>
			</div>
		{/if}
		{#if notice}
			{#await import('#lib/components/themes/ThemeNotice.svelte') then { default: ThemeNotice }}
				<ThemeNotice {notice} />
			{/await}
		{/if}
		{@render children()}
		{#if session.workspace && selecting}
			<!-- Selecting several tasks: the bar loads once something is selected. -->
			{#await import('#lib/components/SelectionBar.svelte') then { default: SelectionBar }}
				<SelectionBar ws={session.workspace.id} {phone} />
			{/await}
		{/if}
	</main>

	{#if openId && session.workspace}
		<!-- The landmark, and where focus goes when a task opens: its fields
		     come next for Tab. The resize edge is inside it, as part of it. -->
		<!-- Escape on the panel closes it before its code has loaded (below). -->
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<aside
			class="detail-pane"
			id="task-panel"
			aria-label="Task details"
			tabindex="-1"
			data-panel-focus
			bind:this={pane}
			in:sheet={{ phone }}
			out:sheet={{ phone }}
			use:pullDown={{ handle: '[data-grab]', onclose: () => openTask(null) }}
			onkeydown={(e) => {
				// Until the panel has loaded, Escape closes it here; then the panel does.
				if (e.key === 'Escape' && !TaskDetail && !e.defaultPrevented) void openTask(null);
			}}
		>
			{#if !phone && !layout.expanded}
				<Splitter
					label="Resize task panel"
					controls="task-panel"
					value={detailDrawn}
					min={Math.min(DETAIL.min, detailDrawn)}
					max={Math.max(detailDrawn, Math.min(DETAIL.max, detailCap))}
					grows="left"
					onresize={(px) => layout.setDetailWidth(px)}
					ondragging={(on) => (resizing = on)}
				/>
			{/if}
			{#if TaskDetail}
				<TaskDetail
					ws={session.workspace.id}
					taskId={openId}
					expanded={!phone && layout.expanded}
					onexpand={phone ? undefined : () => layout.toggleExpanded()}
					onclose={() => openTask(null)}
					onopen={(id) => openTask(id)}
				/>
			{/if}
		</aside>
	{/if}
	{#if SchedulePicker}<SchedulePicker />{/if}
	{#if contextMenu.open}
		<!-- Right-click menus: drawn the first time one opens. -->
		{#await import('#lib/components/ContextMenu.svelte') then { default: ContextMenu }}
			<ContextMenu open={contextMenu.open} />
		{/await}
	{/if}

	<NotificationPanel
		id="notifications"
		items={notifications.items}
		unread={notifications.unread}
		hasMore={notifications.next !== null}
		onshow={() => void notifications.refresh()}
		onopen={(n) => void notifications.read(n.id)}
		onreadall={() => void notifications.readAll()}
		onclear={(n) => void notifications.clear(n.id).catch((err) => toasts.error(messageOf(err)))}
		onclearall={() => void notifications.clearAll().catch((err) => toasts.error(messageOf(err)))}
		onmore={() => void notifications.more().catch((err) => toasts.error(messageOf(err)))}
	/>

	<!-- Out of the way while a sheet covers the page. -->
	<button
		class="fab"
		class:hidden={scheduler.target !== null || (phone && selecting)}
		type="button"
		aria-label="Add a task"
		onclick={() => ui.open('quickadd')}
	>
		<Plus size={24} />
	</button>

	{#if timing && phone && !openId}
		<!-- A landmark of its own: above the tabs it is in no other, where a
		     screen reader moving by landmarks would never come to it. -->
		<aside aria-label="Running timer">
			{#await import('#lib/components/RunningTimer.svelte') then { default: RunningTimer }}
				<RunningTimer variant="phone" />
			{/await}
		</aside>
	{/if}
	<TabBar inert={sheetOpen || (phone && selecting)} />
</div>

<style>
	.shell {
		--sidebar-width: 248px;
		position: relative;
		display: grid;
		grid-template-columns: var(--sidebar-width) 1fr;
		/* One screen tall: each column scrolls on its own, never the page. */
		grid-template-rows: minmax(0, 1fr);
		height: 100dvh;
		/* A page arriving slides up from a little below (arrive in motion.ts), and
		   what is moved still counts toward the page's size: unclipped, the page grew
		   by those pixels for the length of the move, and Windows drew a scrollbar
		   for it. */
		overflow: clip;
		transition: grid-template-columns var(--dur-panel) var(--ease);
	}
	/* Columns that change width move: under reduced motion they jump. */
	@media (prefers-reduced-motion: reduce) {
		.shell {
			transition: none;
		}
	}
	/* The panel keeps its chosen width, but always leaves the list some room. */
	.shell.detail-open {
		grid-template-columns:
			var(--sidebar-width) minmax(0, 1fr)
			clamp(280px, var(--detail-width), calc(100% - var(--sidebar-width) - 360px));
	}
	/* Expanded, the task takes the list's place. */
	.shell.detail-open.expanded {
		grid-template-columns: var(--sidebar-width) minmax(0, 1fr);
	}
	.shell.detail-open.expanded main {
		display: none;
	}
	.shell.resizing {
		transition: none;
		cursor: col-resize;
		user-select: none;
	}
	.sidebar-edge {
		position: absolute;
		top: 0;
		right: 0;
		bottom: 0;
	}
	.detail-pane {
		position: relative;
		min-width: 0;
		min-height: 0;
		border-left: var(--border-width) solid var(--border);
		box-shadow: var(--shadow-md);
	}
	/* Focused only to hold the place; its controls show their own rings. */
	.detail-pane:focus-visible {
		outline: none;
		box-shadow: var(--shadow-md);
	}
	.expanded .detail-pane {
		border-left: none;
		box-shadow: none;
	}
	.shell.collapsed {
		--sidebar-width: 56px;
	}
	.sidebar {
		position: relative;
		display: flex;
		min-height: 0;
		background: var(--sidebar-bg);
		border-right: var(--border-width) solid var(--sidebar-border);
	}
	.sidebar-body {
		/* Holds the sync dot's hidden text, which otherwise hangs off the
		   page once enough projects push the footer below the fold. */
		position: relative;
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: var(--space-4);
		min-width: 0;
		padding: var(--space-3) var(--space-2);
		overflow: hidden auto;
	}
	.top {
		display: flex;
		align-items: center;
		gap: var(--space-1);
	}
	.icon:hover {
		background: var(--sidebar-item-hover);
	}
	.icon {
		display: grid;
		place-items: center;
		width: 28px;
		height: 28px;
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--sidebar-text);
		cursor: pointer;
	}
	.links {
		display: grid;
		gap: 2px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.footer-link {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--space-3);
		height: calc(32px * var(--control-scale));
		padding: 0 var(--space-2);
		border-radius: var(--radius-md);
		color: var(--sidebar-text);
		white-space: nowrap;
		transition: background-color var(--dur-micro) var(--ease);
	}
	.footer-link:hover,
	.footer-link[aria-current='page'] {
		background: var(--sidebar-item-hover);
		color: var(--sidebar-text-active);
	}
	footer {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		margin-top: auto;
		padding: 0 var(--space-2);
	}
	.footer-link {
		width: 100%;
		margin: 0 calc(-1 * var(--space-2));
		border: none;
		background: none;
		font-size: var(--text-sm);
		cursor: pointer;
	}
	/* Collapsed, the names go out of sight but not out of the page: screen
	   readers and voice control still find each link by them, and a
	   tooltip shows them on hover. The sections' headings go the same way. */
	.collapsed .sidebar :global(.name),
	.collapsed .sidebar :global(.section h2),
	.collapsed .sidebar :global(.section > p),
	.collapsed :global(.sync .text) {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
	.collapsed :global(.trigger svg) {
		display: none;
	}
	/* Collapsed, the sidebar is too narrow for the workspace button beside
	   the one that expands it: that one goes on top, the workspace's below
	   (GitHub issue #3). The order in the page stays, so the button keeps
	   focus as it is pressed. */
	.collapsed .top {
		flex-direction: column-reverse;
		align-items: center;
		gap: var(--space-2);
	}
	.collapsed .top :global(.trigger) {
		flex: none;
		justify-content: center;
	}
	.collapsed .sidebar :global(.section .heading) {
		justify-content: center;
		padding: 0;
	}
	/* Everything on one line down the middle of the collapsed sidebar. */
	.collapsed footer {
		padding: 0;
	}
	.collapsed .footer-link {
		margin: 0;
	}
	.collapsed .footer-link,
	.collapsed .new-task,
	.collapsed :global(.sidebar-link),
	.collapsed .sidebar :global(.bell),
	.collapsed .sidebar :global(.section a),
	.collapsed .sidebar :global(.section form) {
		justify-content: center;
		padding: 0;
	}
	.collapsed :global(.sync) {
		justify-content: center;
	}
	.skip {
		position: fixed;
		top: var(--space-2);
		left: var(--space-2);
		z-index: 300;
		padding: var(--space-2) var(--space-4);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		color: var(--accent-text);
		font-weight: 600;
		transform: translateY(calc(-100% - var(--space-4)));
	}
	/* The shadow only while it shows: hidden above the page, its blur
	   still reached down into the corner. */
	.skip:focus-visible {
		box-shadow: var(--shadow-lg);
		transform: none;
	}
	/* Focused only as a starting point; what is in it shows its own focus. */
	main:focus-visible {
		outline: none;
		box-shadow: none;
	}
	main {
		position: relative;
		min-width: 0;
		overflow: auto;
		/* A focused row is scrolled clear of the toasts at the bottom. */
		scroll-padding-bottom: calc(var(--space-7) + var(--space-5));
	}
	.fab {
		display: none;
	}
	.new-row {
		display: flex;
		align-items: center;
		gap: 2px;
	}
	.new-task {
		flex: 1;
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--space-3);
		height: calc(32px * var(--control-scale));
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--accent-text);
		font-weight: 600;
		cursor: pointer;
	}
	.new-task:hover {
		background: var(--sidebar-item-hover);
	}
	.new-task kbd {
		margin-left: auto;
		font-family: var(--font-ui);
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
	.phone-bar {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-3) 0;
		padding-top: calc(var(--space-2) + env(safe-area-inset-top));
	}
	.gear {
		display: grid;
		flex: none;
		place-items: center;
		width: var(--touch-target);
		height: var(--touch-target);
		border-radius: var(--radius-md);
		color: var(--text-secondary);
	}
	.gear[aria-current='page'] {
		color: var(--accent-text);
	}

	/* Phone: the sidebar becomes a bottom tab bar. */
	@media (max-width: 720px) {
		/* On a phone the details cover the screen, sliding up like a sheet. */
		.shell.detail-open {
			grid-template-columns: 1fr;
		}
		.detail-pane {
			position: fixed;
			inset: 0;
			z-index: 50;
			border: none;
			/* Springs back when a pull down (swipe.ts) falls short. */
			transition: transform var(--dur-panel) var(--ease);
		}
		/* Clear of the FAB, so the last row's controls can be reached, and a
		   focused row is scrolled clear of it. */
		main {
			padding-bottom: calc(56px + 2 * var(--space-4));
			scroll-padding-bottom: calc(56px + 2 * var(--space-4));
		}
		.shell,
		.shell.collapsed {
			grid-template-columns: 1fr;
			grid-template-rows: minmax(0, 1fr) auto;
		}
		.sidebar {
			display: none;
		}
		.fab {
			position: fixed;
			right: var(--space-4);
			bottom: calc(var(--tabbar-height) + var(--space-4) + env(safe-area-inset-bottom));
			z-index: 20;
			display: grid;
			place-items: center;
			width: 56px;
			height: 56px;
			border: none;
			border-radius: 50%;
			background: var(--accent);
			color: var(--accent-contrast);
			box-shadow: var(--shadow-lg);
			cursor: pointer;
		}
		.detail-open .fab,
		.fab.hidden {
			display: none;
		}
	}
</style>
