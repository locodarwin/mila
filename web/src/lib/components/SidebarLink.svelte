<script lang="ts">
	import type { Component } from 'svelte';

	// One of the sidebar's links (Inbox, My Day, Upcoming): an icon and a
	// name, which the collapsed sidebar hides (the app layout) and shows as
	// a tooltip. The theme editor's preview draws it too.
	interface Props {
		href: string;
		label: string;
		icon: Component<{ size?: number }>;
		current?: boolean;
		collapsed?: boolean;
	}

	let { href, label, icon: Icon, current = false, collapsed = false }: Props = $props();
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- callers pass a resolved path -->
<a
	class="sidebar-link"
	{href}
	title={collapsed ? label : undefined}
	aria-current={current ? 'page' : undefined}
>
	<Icon size={16} />
	<span class="name">{label}</span>
</a>

<!-- eslint-enable svelte/no-navigation-without-resolve -->

<style>
	.sidebar-link {
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
	.sidebar-link:hover {
		background: var(--sidebar-item-hover);
		color: var(--sidebar-text-active);
	}
	.sidebar-link[aria-current='page'] {
		background: var(--sidebar-item-active);
		color: var(--sidebar-text-active);
		font-weight: 550;
	}
</style>
