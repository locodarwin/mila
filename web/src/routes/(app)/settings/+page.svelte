<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { MediaQuery } from 'svelte/reactivity';

	// Settings itself is the list of its pages on a phone (the layout draws
	// it); beside the list, on a wider screen, it opens on Profile.
	const phone = new MediaQuery('max-width: 720px');
	$effect(() => {
		if (!phone.current) void goto(resolve('/(app)/settings/profile'), { replaceState: true });
	});
</script>

<svelte:head><title>Settings · Mila</title></svelte:head>
