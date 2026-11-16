<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	// The workspace's themes load apart, so Settings stays small for everyone else.
	const ws = $derived(settingsWorkspace());
</script>

<svelte:head><title>Workspace themes · Mila</title></svelte:head>

{#if ws?.isPersonal}
	<section>
		<h2>Workspace themes</h2>
		<p class="hint">
			A shared workspace can offer themes to everyone in it. Your personal workspace has none; your
			own themes are in Appearance.
		</p>
	</section>
{:else if ws}
	{#await import('#lib/components/themes/WorkspaceThemes.svelte') then { default: WorkspaceThemes }}
		<WorkspaceThemes ws={ws.id} />
	{/await}
{/if}
