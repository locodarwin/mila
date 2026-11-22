<script lang="ts">
	// The workspace's wiki; a project's is on the
	// project's Wiki tab. The page shown is in the address, ?page=<id>.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import Wiki from '#lib/components/wiki/Wiki.svelte';
	import { session } from '#lib/session.svelte.js';

	const ws = $derived(session.workspace?.id ?? '');
	const pageId = $derived(page.url.searchParams.get('page'));
	const guest = $derived(session.workspace?.role === 'guest');

	function open(id: string) {
		const u = new URL(page.url.href);
		u.searchParams.set('page', id);
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- same page, new query
		return goto(u, { reset: false });
	}
</script>

<svelte:head><title>Workspace Wiki · Mila</title></svelte:head>

{#if guest}
	<header class="page"><h1>Workspace Wiki</h1></header>
	<EmptyState
		title="This workspace's wiki is for its members"
		hint="The wikis of the projects shared with you are on their Project Wiki tabs."
	/>
{:else if ws}
	<div class="frame" data-screen>
		<Wiki {ws} projectId={null} {pageId} onopen={open} />
	</div>
{/if}

<style>
	/* One screen tall, less what the layout puts over it. */
	.frame {
		height: calc(100% - var(--above, 0px));
		min-height: 0;
	}
</style>
