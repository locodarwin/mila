<script lang="ts">
	import { untrack } from 'svelte';
	import { announcer } from '../announce.svelte';
	import { toasts } from '../toasts.svelte';

	// A modal dialog makes the rest of the page inert, and inert live
	// regions are not read. So each modal dialog holds a copy of its own
	// (inDialog) while it is open: what is announced, and the toasts, that
	// arrive after it opened. The page's regions are inert meanwhile, so
	// nothing is read twice.
	let { inDialog = false }: { inDialog?: boolean } = $props();

	const since = untrack(() => Math.max(0, ...toasts.items.map((t) => t.id)));
	const fresh = $derived(toasts.items.filter((t) => t.id > since));
	const infos = $derived(fresh.filter((t) => t.tone !== 'error'));
	const errors = $derived(fresh.filter((t) => t.tone === 'error'));

	// Starts empty: a message said before the dialog opened is not new.
	let said = $state('');
	let first = true;
	$effect(() => {
		const message = announcer.message;
		if (first) first = false;
		else said = message;
	});
</script>

{#if inDialog}
	<div class="visually-hidden live" role="status">
		{#if said}<p>{said}</p>{/if}
		{#each infos as t (t.id)}<p>{t.message}</p>{/each}
	</div>
	<div class="visually-hidden live" role="alert">
		{#each errors as t (t.id)}<p>{t.message}</p>{/each}
	</div>
{:else}
	<!-- Always in the page, so a screen reader is already listening when a
	     message arrives (announce.svelte.ts). -->
	<div id="announcer" class="visually-hidden live" role="status">{announcer.message}</div>
{/if}

<style>
	/* Pinned to the top: where the page's flow would put it, below an app as tall as the
	   window, it made the page a pixel taller, and the browser drew a scrollbar for it. */
	.live {
		top: 0;
		left: 0;
	}
</style>
