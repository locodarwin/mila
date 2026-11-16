<script lang="ts">
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import Button from '../Button.svelte';
	import { setTaskDeleted } from '../../sync/data';
	import { toasts } from '../../toasts.svelte';

	interface Props {
		ws: string;
		/** The tasks the import made. */
		ids: string[];
		/** The project they went to, or null for the inbox. */
		project: string | null;
		/** Once they are going to the trash, so the page can stop showing this. */
		ontrash: () => void;
		/** Puts them in the trash; the style guide's does nothing. */
		trash?: (ids: string[]) => Promise<void>;
		/** Whether its heading takes focus once shown, as the button that ran the import goes. */
		focus?: boolean;
	}

	let {
		ws,
		ids,
		project,
		ontrash,
		focus = true,
		trash = async (all) => {
			for (const id of all) await setTaskDeleted(ws, id, true);
		}
	}: Props = $props();
	let heading = $state<HTMLElement>();

	onMount(() => {
		if (focus) requestAnimationFrame(() => heading?.focus());
	});

	async function trashThem() {
		const all = ids;
		ontrash();
		await trash(all);
		toasts.show(`Moved ${all.length} imported ${all.length === 1 ? 'task' : 'tasks'} to the trash`);
	}
</script>

<section>
	<h2 tabindex="-1" bind:this={heading}>
		Imported {ids.length}
		{ids.length === 1 ? 'task' : 'tasks'}
	</h2>
	<div class="actions">
		{#if project}
			<a class="link" href={resolve('/(app)/projects/[id]', { id: project })}>Open the project</a>
		{:else}
			<a class="link" href={resolve('/(app)')}>Open the inbox</a>
		{/if}
		<Button
			title="Move the tasks this import made to the trash"
			variant="danger"
			onclick={trashThem}>Move them to the trash</Button
		>
	</div>
</section>

<style>
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
	}
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
</style>
