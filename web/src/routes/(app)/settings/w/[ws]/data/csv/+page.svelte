<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import CsvImport from '#lib/components/data/CsvImport.svelte';

	const ws = $derived(settingsWorkspace());
	const guest = $derived(ws?.role === 'guest');
	let importer = $state<ReturnType<typeof CsvImport>>();

	async function pick(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		if (!(await importer?.load(input.files?.[0]))) input.value = '';
	}
</script>

<svelte:head><title>Import a CSV file · Mila</title></svelte:head>

<section>
	<h2>Import a CSV file</h2>
	<p class="hint">
		Tasks from a spreadsheet or another app, into {ws?.name ?? 'this workspace'}. Choose what each
		column means, check what would happen, then import. Dates are read as 2026-11-03.
		<a class="link" href={resolve('/(app)/settings/w/[ws]/data', { ws: page.params.ws ?? '' })}
			>Back to Import and export</a
		>
	</p>
	{#if guest}
		<p class="hint">Guests cannot import tasks here.</p>
	{:else}
		<label class="file">
			<span>CSV file</span>
			<input type="file" accept=".csv,.tsv,.txt,text/csv" onchange={pick} />
		</label>
	{/if}
</section>

{#if !guest}
	<CsvImport bind:this={importer} />
{/if}

<style>
	.file {
		display: grid;
		gap: var(--space-1);
		font-size: var(--text-sm);
	}
	.file span {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.file input {
		max-width: 100%;
		min-height: var(--tap-min);
	}
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
</style>
