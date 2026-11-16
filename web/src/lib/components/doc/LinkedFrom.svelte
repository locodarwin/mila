<script lang="ts">
	// What links to a page or task (docs/PLAN.md section 3): the pages and
	// task notes holding a link to it, worked out on the device from what
	// they say, so it shows offline too.
	import FileText from '@lucide/svelte/icons/file-text';
	import SquareCheck from '@lucide/svelte/icons/square-check';
	import { engine } from '../../sync/engine.svelte';
	import { openTask } from '../../taskLink';
	import { pageHref, pageTitle } from '../../wiki';

	interface Props {
		ws: string;
		/** The page or task linked to. */
		id: string;
		/** Just the list, under a heading from elsewhere (the task panel's). */
		bare?: boolean;
	}

	let { ws, id, bare = false }: Props = $props();
	const replica = $derived(engine.replica(ws));
	const from = $derived(replica?.linkedFrom(id) ?? { pages: [], tasks: [] });
	const count = $derived(from.pages.length + from.tasks.length);
</script>

{#snippet list()}
	<ul>
		{#each from.pages as p (p.id)}
			<li>
				<FileText size={14} aria-hidden="true" />
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- pageHref resolves it -->
				<a href={pageHref(p)}
					>{pageTitle(ws, p, p.projectId ? replica?.project(p.projectId) : null)}</a
				>
			</li>
		{/each}
		{#each from.tasks as t (t.id)}
			<li>
				<SquareCheck size={14} aria-hidden="true" />
				<button type="button" class="link" onclick={() => void openTask(t.id)}>{t.title}</button>
			</li>
		{/each}
	</ul>
{/snippet}

{#if count}
	{#if bare}
		{@render list()}
	{:else}
		<section class="linked" aria-labelledby={`linked-${id}`}>
			<h3 id={`linked-${id}`}>Linked from</h3>
			{@render list()}
		</section>
	{/if}
{/if}

<style>
	.linked {
		display: grid;
		gap: var(--space-2);
	}
	h3 {
		font-size: var(--text-sm);
		font-weight: 600;
		color: var(--text-secondary);
	}
	ul {
		display: grid;
		gap: var(--space-1);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 24px);
		color: var(--text-secondary);
	}
	a,
	.link {
		color: var(--accent-text);
		text-decoration: underline;
	}
	.link {
		padding: 0;
		border: none;
		background: none;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
</style>
