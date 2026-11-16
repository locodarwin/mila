<script lang="ts">
	// The report loads apart, so no other page pays for it.
	import { session } from '#lib/session.svelte.js';
</script>

<svelte:head><title>Time · Mila</title></svelte:head>

<header class="page"><h1>Time</h1></header>
{#await import('#lib/components/time/TimeReport.svelte') then { default: TimeReport }}
	<TimeReport ws={session.workspace?.id ?? ''} />
{/await}
