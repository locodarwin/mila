<script lang="ts">
	// The sidebar's links, loaded apart: one current, one not, and the
	// collapsed sidebar's, which shows only the icon and names it in a
	// tooltip. Each links back to this page.
	import { resolve } from '$app/paths';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import Inbox from '@lucide/svelte/icons/inbox';
	import Sun from '@lucide/svelte/icons/sun';
	import SidebarLink from '#lib/components/SidebarLink.svelte';

	const here = resolve('/dev/ui');
</script>

<div class="side">
	<nav aria-label="Sample sidebar">
		<SidebarLink href={here} label="Inbox" icon={Inbox} current />
		<SidebarLink href={here} label="My Day" icon={Sun} />
		<SidebarLink href={here} label="Upcoming" icon={CalendarDays} />
	</nav>
	<nav class="collapsed" aria-label="Sample collapsed sidebar">
		<SidebarLink href={here} label="Inbox" icon={Inbox} current collapsed />
		<SidebarLink href={here} label="My Day" icon={Sun} collapsed />
	</nav>
</div>

<style>
	.side {
		display: flex;
		gap: var(--space-4);
		align-items: flex-start;
	}
	nav {
		display: grid;
		gap: 2px;
		width: 220px;
		padding: var(--space-2);
		border-radius: var(--radius-md);
		background: var(--sidebar-bg);
	}
	.collapsed {
		width: auto;
	}
	/* As the app layout hides them: out of sight, still the link's name. */
	.collapsed :global(.name) {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
</style>
