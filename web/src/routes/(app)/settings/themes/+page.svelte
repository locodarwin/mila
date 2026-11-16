<script lang="ts">
	// Your own themes moved to Appearance; a workspace's to Workspace themes.
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';

	$effect(() => {
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with its query
		void goto(resolve('/(app)/settings/appearance') + page.url.search + page.url.hash, {
			replaceState: true
		});
	});
</script>
