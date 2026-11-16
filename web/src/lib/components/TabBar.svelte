<script lang="ts">
	import { session } from '../session.svelte';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import FolderKanban from '@lucide/svelte/icons/folder-kanban';
	import Inbox from '@lucide/svelte/icons/inbox';
	import Search from '@lucide/svelte/icons/search';
	import Sun from '@lucide/svelte/icons/sun';
	import { ui } from '../ui.svelte';

	// The phone's main navigation, at the bottom of the screen. The app
	// layout shows it only on a phone; the style guide always (inline).
	// inert while the task sheet covers the screen (app layout).
	let { inline = false, inert = false }: { inline?: boolean; inert?: boolean } = $props();

	const projects = resolve('/(app)/projects');
	const tabs = $derived([
		// A guest has no inbox.
		...(session.workspace?.role === 'guest'
			? []
			: [{ href: resolve('/(app)'), label: 'Inbox', icon: Inbox }]),
		{ href: resolve('/(app)/today'), label: 'My Day', icon: Sun },
		{ href: resolve('/(app)/upcoming'), label: 'Upcoming', icon: CalendarDays },
		{ href: projects, label: 'Projects', icon: FolderKanban }
	]);
	const current = (href: string) =>
		href === projects ? page.url.pathname.startsWith(href) : page.url.pathname === href;
</script>

<nav class="tabs" class:inline aria-label="Main" {inert}>
	{#each tabs as tab (tab.href)}
		<a href={tab.href} aria-current={current(tab.href) ? 'page' : undefined}>
			<tab.icon size={20} />
			<span>{tab.label}</span>
		</a>
	{/each}
	<!-- Search is the palette, which finds tasks, projects and views and
	     runs every command. -->
	<button type="button" onclick={() => ui.open('palette')}>
		<Search size={20} />
		<span>Search</span>
	</button>
</nav>

<style>
	.tabs {
		display: none;
		grid-auto-flow: column;
		grid-auto-columns: 1fr;
		padding-bottom: env(safe-area-inset-bottom);
		background: var(--tabbar-bg);
		border-top: var(--border-width) solid var(--border);
	}
	.tabs.inline {
		display: grid;
		padding-bottom: 0;
	}
	@media (max-width: 720px) {
		.tabs {
			display: grid;
		}
	}
	a,
	button {
		display: grid;
		justify-items: center;
		align-content: start;
		gap: 2px;
		min-height: var(--tabbar-height);
		padding: var(--space-2) 0 0;
		border: none;
		background: none;
		font-size: var(--text-xs);
		color: var(--text-secondary);
		cursor: pointer;
	}
	a[aria-current='page'] {
		color: var(--tabbar-active);
		font-weight: 650;
	}
</style>
