<script lang="ts">
	// An address from before Settings was grouped (/settings/labels, and the
	// like, which emails already sent link to): it goes on to the same page of
	// the workspace open in the sidebar, keeping its query and anchor.
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { session } from '../../session.svelte';
	import { workspaceSettings, type WorkspacePage } from '../../settingsAddress';

	let { to, hash }: { to: WorkspacePage | 'data/apps' | 'data/csv'; hash?: string } = $props();

	function target(ws: string): string {
		if (to === 'data/apps') return resolve('/(app)/settings/w/[ws]/data/apps', { ws });
		if (to === 'data/csv') return resolve('/(app)/settings/w/[ws]/data/csv', { ws });
		return workspaceSettings(ws, to);
	}

	let gone = false;
	$effect(() => {
		// A link naming its workspace (?ws=) has the app layout choose it and
		// take the parameter out first, as for any link into the app.
		if (page.url.searchParams.has('ws')) return;
		const ws = session.workspace?.id;
		if (gone || (!ws && !session.user)) return;
		gone = true;
		const rest = page.url.search + (page.url.hash || (hash ? `#${hash}` : ''));
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with its query
		void goto(ws ? target(ws) + rest : resolve('/(app)/settings'), { replaceState: true });
	});
</script>
