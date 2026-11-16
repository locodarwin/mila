<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import RailIcon from '#lib/components/settings/RailIcon.svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import { session } from '#lib/session.svelte.js';
	import {
		settingsNav,
		workspaceSettings,
		type NavGroup,
		type WorkspacePage
	} from '#lib/settingsNav.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const phone = new MediaQuery('max-width: 720px');
	// Settings' own workspace: the one a workspace page's address names, or
	// the last one picked here, or else the sidebar's. Picking one changes
	// Settings alone; the rest of the app stays where it was.
	const routeWs = $derived(page.params.ws);
	let chosen = $state<string | null>(null);
	$effect(() => {
		if (routeWs) chosen = routeWs;
	});
	const picked = $derived(
		session.workspaces.find((w) => w.id === (routeWs ?? chosen)) ?? session.workspace
	);
	const groups = $derived(
		settingsNav({
			workspace: picked,
			local: session.local,
			instanceAdmin: Boolean(session.user?.isInstanceAdmin)
		})
	);
	const home = $derived(page.route.id === '/(app)/settings');

	/** The workspace page in view, so picking another workspace keeps to it where it can. */
	const shownPage = $derived(
		(page.route.id?.match(/\/settings\/w\/\[ws\]\/([a-z]+)/)?.[1] ?? null) as WorkspacePage | null
	);

	async function pick(id: string) {
		chosen = id;
		if (!routeWs) return;
		const w = session.workspaces.find((x) => x.id === id) ?? null;
		const offered = settingsNav({ workspace: w, local: session.local, instanceAdmin: false })
			.flatMap((g) => g.items)
			.some((i) => i.page === shownPage);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- workspaceSettings resolves
		await goto(workspaceSettings(id, shownPage && offered ? shownPage : 'general'));
	}

	const section = (id: NavGroup['id']) => groups.filter((g) => g.id === id);
	const workspaceGroups = $derived(groups.filter((g) => g.id === 'everyone' || g.id === 'yours'));

	// A page stays marked on the pages under it (Import and export's CSV import).
	const here = (href: string) =>
		page.url.pathname === href || page.url.pathname.startsWith(href + '/');
</script>

{#snippet list(g: NavGroup)}
	<ul aria-labelledby={g.listLabel ? undefined : `settings-${g.id}`} aria-label={g.listLabel}>
		{#each g.items as item (item.href)}
			<li>
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- settingsNav resolves -->
				<a href={item.href} aria-current={here(item.href) ? 'page' : undefined}
					><RailIcon name={item.icon} />{item.label}</a
				>
			</li>
		{/each}
	</ul>
{/snippet}

<header class="page top">
	<h1>Settings</h1>
</header>
<div class="body">
	{#if !phone.current || home}
		<!-- Sections are bold headings with a rule between them; pages have icons,
		     so neither is taken for the other. -->
		<nav class="rail" aria-label="Settings">
			{#each section('you') as g (g.id)}
				<div class="section">
					<p class="section-name" id="settings-{g.id}">{g.label}</p>
					{@render list(g)}
				</div>
			{/each}
			{#if session.workspaces.length}
				<div class="section">
					<div class="section-head">
						<label class="section-name" for="settings-workspace">Workspace</label>
						<select
							id="settings-workspace"
							value={picked?.id ?? ''}
							title="The workspace whose settings show under it"
							onchange={(e) => void pick(e.currentTarget.value)}
						>
							{#each session.workspaces as w (w.id)}<option value={w.id}>{w.name}</option>{/each}
						</select>
					</div>
					{#each workspaceGroups as g (g.id)}
						<p class="caption" aria-hidden="true">{g.label}</p>
						{@render list(g)}
					{/each}
				</div>
			{/if}
			{#each section('mila') as g (g.id)}
				<div class="section">
					<p class="section-name" id="settings-{g.id}">{g.label}</p>
					{@render list(g)}
				</div>
			{/each}
		</nav>
	{/if}
	{#if !(phone.current && home)}
		<div class="content">
			{#if phone.current}
				<div class="crumb">
					<a class="back" href={resolve('/(app)/settings')}><ChevronLeft size={16} />Settings</a>
					<!-- The picker is on the list; a workspace page says which it is about. -->
					{#if routeWs && picked}<p class="in">Workspace: {picked.name}</p>{/if}
				</div>
			{/if}
			<!-- Wide, a page's sections marked half pair up two to a row; the rest
			     keep the whole width (docs/PLAN.md section 3, Settings). -->
			<div class="grid">{@render children()}</div>
		</div>
	{/if}
</div>

<style>
	/* The settings: up to 1,120 px, and from 1,000 px two columns, which the
	   sections a page marks half share (Shawn's option C, 2026-11-15). */
	.content {
		display: grid;
		gap: var(--space-6);
		max-width: 1120px;
		padding: var(--space-5);
		container-type: inline-size;
	}
	.grid {
		display: grid;
		gap: var(--space-6);
		align-items: start;
	}
	@container (min-width: 1000px) {
		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			column-gap: var(--space-7);
		}
		.grid > :global(*) {
			grid-column: 1 / -1;
		}
		.grid > :global(.half) {
			grid-column: auto;
		}
	}
	/* Words stay a readable width however wide the page is. */
	.content :global(.hint) {
		max-width: 72ch;
	}
	.content :global(section) {
		display: grid;
		gap: var(--space-3);
	}
	.content :global(h2) {
		font-size: var(--text-lg);
	}
	.content :global(.hint) {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.content :global(.rows) {
		margin: 0;
		padding: 0;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	.content :global(.rows > li) {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: var(--pad-y) var(--space-4);
	}
	.content :global(.rows > li + li) {
		border-top: 1px solid var(--border);
	}
	.content :global(.grow) {
		flex: 1;
		min-width: 160px;
	}
	.content :global(.sub) {
		display: block;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.content :global(.badge) {
		padding: 1px var(--space-2);
		border-radius: var(--radius-full);
		background: var(--accent-soft);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	.content :global(.inline-form) {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		align-items: flex-end;
	}
	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2) var(--space-4);
	}
	/* The room between the list and the settings grows with the window: with
	   the list's and the page's own padding, 5% of it, from 40 to 120 px. */
	.body {
		display: grid;
		grid-template-columns: 220px minmax(0, 1fr);
		column-gap: clamp(8px, calc(5vw - var(--space-3) - var(--space-5)), 88px);
		align-items: start;
	}
	.rail {
		position: sticky;
		top: 0;
		display: grid;
		gap: var(--space-3);
		padding: var(--space-4) var(--space-3) var(--space-5) var(--space-5);
	}
	.section {
		display: grid;
		gap: 2px;
	}
	.section + .section {
		padding-top: var(--space-3);
		border-top: 1px solid var(--border);
	}
	.section-name {
		padding: 0 var(--space-2) var(--space-1);
		font-size: var(--text-sm);
		font-weight: 700;
		color: var(--text-primary);
	}
	.section-head {
		display: grid;
		gap: var(--space-1);
		padding-bottom: var(--space-1);
	}
	.section-head .section-name {
		padding-bottom: 0;
	}
	.section-head select {
		min-width: 0;
		min-height: max(var(--tap-min), calc(32px * var(--control-scale)));
		margin-left: var(--space-2);
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-1);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.caption {
		padding: var(--space-2) var(--space-2) 2px;
		font-size: var(--text-xs);
		font-weight: 500;
		color: var(--text-muted);
	}
	.rail ul {
		display: grid;
		gap: 2px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.rail a {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), calc(30px * var(--control-scale)));
		padding: 0 var(--space-2);
		border-radius: var(--radius-md);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		font-weight: 450;
	}
	.rail a :global(svg) {
		flex: none;
		color: var(--text-muted);
	}
	.rail a[aria-current='page'] :global(svg) {
		color: var(--accent-text);
	}
	.rail a:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.rail a[aria-current='page'] {
		background: var(--accent-soft);
		color: var(--text-primary);
	}
	.crumb {
		display: grid;
		justify-items: start;
	}
	.in {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.back {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		justify-self: start;
		min-height: var(--tap-min);
		color: var(--accent-text);
		font-size: var(--text-sm);
		font-weight: 550;
	}
	@media (max-width: 720px) {
		.top,
		.content {
			padding-inline: var(--space-4);
		}
		.body {
			grid-template-columns: minmax(0, 1fr);
		}
		/* On a phone Settings opens on the list, and each page on its own. */
		.rail {
			position: static;
			padding: var(--space-3) var(--space-4) var(--space-5);
		}
		.rail a,
		.section-head select {
			font-size: var(--text-md);
		}
		.section-name {
			font-size: var(--text-md);
		}
	}
</style>
