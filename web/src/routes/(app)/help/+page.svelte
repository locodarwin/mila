<script lang="ts">
	import { afterNavigate, goto, replaceState } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Bell from '@lucide/svelte/icons/bell';
	import LayoutGrid from '@lucide/svelte/icons/layout-grid';
	import MonitorSmartphone from '@lucide/svelte/icons/monitor-smartphone';
	import PenLine from '@lucide/svelte/icons/pen-line';
	import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
	import Zap from '@lucide/svelte/icons/zap';
	import { untrack, type Component } from 'svelte';
	import Keys from '#lib/components/help/Keys.svelte';
	import { announce } from '#lib/announce.svelte.js';
	import { HELP_GROUPS, topic } from '#lib/help/index.js';
	import { helpHref } from '#lib/help/links.js';
	import { searchHelp } from '#lib/help/search.js';
	import { command, type CommandId } from '#lib/keys/commands.js';

	// The query lives in ?q=, so Back comes back to the results.
	// Read from the address bar itself: coming Back to a query kept with
	// replaceState, the router's URL may not have it yet. Every arrival
	// reads it again (Help from the sidebar while results show starts
	// over); this page's own replaceState is no navigation, so typing is
	// left alone.
	const fromAddress = () => new URLSearchParams(location.search).get('q') ?? '';
	let query = $state(fromAddress());
	afterNavigate(() => {
		query = fromAddress();
	});
	const hits = $derived(searchHelp(query));

	$effect(() => {
		const q = query.trim();
		untrack(() => {
			const url = new URL(page.url.href);
			if ((url.searchParams.get('q') ?? '') === q) return;
			if (q) url.searchParams.set('q', q);
			else url.searchParams.delete('q');
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- this page, its query changed
			replaceState(url, page.state);
		});
	});

	// Only a new query or count is said, as in the palette.
	let counted = '';
	$effect(() => {
		const q = query.trim();
		const n = hits.length;
		if (!q) {
			counted = '';
			return;
		}
		if (`${q}\n${n}` === counted) return;
		counted = `${q}\n${n}`;
		announce(n ? `${n} ${n === 1 ? 'result' : 'results'}` : 'Nothing matches', 500);
	});

	function keys(e: KeyboardEvent) {
		if (e.key !== 'Enter' || e.isComposing || !hits[0]) return;
		e.preventDefault();
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved in helpHref, with a fragment
		void goto(helpHref(hits[0]));
	}

	// A few of the keys people use most, as they have them.
	const common: CommandId[] = ['quick-add', 'palette', 'undo'];

	const ICONS: Record<string, Component> = {
		in: Zap,
		seeing: LayoutGrid,
		writing: PenLine,
		step: Bell,
		yours: SlidersHorizontal,
		beyond: MonitorSmartphone
	};
</script>

<svelte:head><title>Help · Mila</title></svelte:head>

<header class="page"><h1>Help</h1></header>
<div class="welcome">
	<p class="ask">How can we help?</p>
	<input
		class="search"
		type="search"
		aria-label="Search help"
		placeholder="Search help"
		autocomplete="off"
		bind:value={query}
		onkeydown={keys}
	/>
	<section class="keys" aria-label="Keyboard shortcuts">
		<dl>
			{#each common as id (id)}
				<div>
					<dt>{command(id).name}</dt>
					<dd><Keys {id} /></dd>
				</div>
			{/each}
		</dl>
		<p class="more">
			<a class="link" href={resolve('/(app)/help/shortcuts')}>All shortcuts</a>
			<a class="link" href={resolve('/(app)/settings/keyboard')}>Change shortcuts</a>
		</p>
	</section>
</div>

{#if query.trim()}
	<div class="column">
		{#if hits.length}
			<ul class="results" aria-label="Help results">
				{#each hits as h (`${helpHref(h)} ${h.command ?? ''}`)}
					<li>
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved in helpHref, with a fragment -->
						<a href={helpHref(h)}>{h.title}</a>
						<span class="hint">{h.context}</span>
						{#if h.command}<Keys id={h.command} />{/if}
					</li>
				{/each}
			</ul>
		{:else}
			<p class="hint">Nothing in help matches. The command palette searches your tasks.</p>
		{/if}
	</div>
{:else}
	<div class="groups">
		{#each HELP_GROUPS as g (g.id)}
			{@const Icon = ICONS[g.id]}
			<section
				class="group"
				class:touch={g.topics.includes('touch-gestures')}
				aria-labelledby="help-{g.id}"
			>
				<h2 id="help-{g.id}">
					<span class="badge"><Icon size={16} /></span>
					{g.title}
				</h2>
				<ul>
					{#each g.topics.flatMap((slug) => topic(slug) ?? []) as t (t.slug)}
						<li class:touch={t.slug === 'touch-gestures'}>
							<a class="link" href={resolve('/(app)/help/[topic]', { topic: t.slug })}>{t.title}</a>
							<span class="hint">{t.summary}</span>
						</li>
					{/each}
				</ul>
			</section>
		{/each}
	</div>
{/if}

<style>
	.welcome {
		display: grid;
		justify-items: center;
		gap: var(--space-3);
		padding: var(--space-6) var(--space-5);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-1);
	}
	.ask {
		font-family: var(--font-heading);
		font-size: var(--text-xl);
		font-weight: 600;
		text-align: center;
	}
	.search {
		width: min(560px, 100%);
		height: max(var(--tap-min), calc(42px * var(--control-scale)));
		padding: 0 var(--space-4);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-size: max(16px, var(--text-md));
		box-shadow: var(--shadow-sm);
	}
	.search::placeholder {
		color: var(--input-placeholder);
	}
	.keys {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		align-items: center;
		gap: var(--space-2) var(--space-5);
	}
	dl {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: var(--space-2) var(--space-4);
		margin: 0;
	}
	dl div {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	dd {
		margin: 0;
	}
	.more {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2) var(--space-4);
		font-size: var(--text-sm);
	}
	/* Search results keep a readable line. */
	.column {
		max-width: 68ch;
	}
	.groups {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		align-items: start;
		gap: var(--space-4);
		max-width: 1200px;
		margin: 0 auto;
		padding: var(--space-5);
	}
	.group {
		display: grid;
		gap: var(--space-3);
		min-width: 0;
		padding: var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-0);
	}
	h2 {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-md);
	}
	.badge {
		display: inline-grid;
		place-items: center;
		width: 28px;
		height: 28px;
		flex: none;
		border-radius: var(--radius-md);
		background: var(--accent-soft);
		color: var(--accent-text);
	}
	.group ul,
	.results {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.group li,
	.results li {
		display: grid;
		gap: 2px;
	}
	/* A finger's height of link, with its words beside what's under them. */
	.group a {
		display: inline-flex;
		align-items: flex-end;
	}
	.results li {
		grid-template-columns: 1fr auto;
		align-items: center;
	}
	.results a {
		grid-column: 1 / -1;
		color: var(--accent-text);
		font-weight: 550;
	}
	/* On a touch screen, gestures first. */
	@media (pointer: coarse) {
		.touch {
			order: -1;
		}
	}
	@media (max-width: 720px) {
		.welcome {
			padding: var(--space-5) var(--space-4);
		}
		.groups {
			padding: var(--space-4);
		}
	}
</style>
