<script lang="ts">
	import { resolve } from '$app/paths';
	import { session } from '#lib/session.svelte.js';
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	// An address can name a workspace this person has left, or never had.
	const ws = $derived(settingsWorkspace());
</script>

{#if ws}
	{@render children()}
{:else if session.workspaces.length}
	<section>
		<h2>Not one of your workspaces</h2>
		<p class="hint">
			This address is for a workspace you are not in, or one that was deleted. Pick one of yours
			under Workspace in Settings.
		</p>
		<div><a class="link" href={resolve('/(app)/settings')}>Go to Settings</a></div>
	</section>
{/if}

<style>
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
</style>
