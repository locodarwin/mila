<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import Automations from '#lib/components/automations/Automations.svelte';
	import { canAutomate } from '#lib/roles.js';
	import { session } from '#lib/session.svelte.js';

	const ws = $derived(settingsWorkspace()?.id ?? '');
	const w = $derived(settingsWorkspace());
	const allowed = $derived(w ? canAutomate(w.role) : false);
</script>

<svelte:head><title>Automations · Mila</title></svelte:head>

{#if allowed && ws}
	<Automations {ws} role={settingsWorkspace()?.role ?? 'member'} me={session.user?.id ?? ''} />
{:else}
	<section>
		<h2>Automations</h2>
		<p class="hint">Automations belong to a workspace's members, and guests don't have them.</p>
	</section>
{/if}
