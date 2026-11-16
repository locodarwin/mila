<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	// The manager loads apart, so Settings stays small for everyone else.
</script>

<svelte:head><title>Templates · Mila</title></svelte:head>

{#await import('#lib/components/templates/TemplateManager.svelte') then { default: TemplateManager }}
	<TemplateManager ws={settingsWorkspace()?.id ?? ''} />
{/await}
