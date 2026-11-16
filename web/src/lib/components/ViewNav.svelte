<script lang="ts">
	import { menu, viewActions } from '../contextMenu.svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import Layers from '@lucide/svelte/icons/layers';
	import Plus from '@lucide/svelte/icons/plus';
	import Users from '@lucide/svelte/icons/users';
	import { canEditViews } from '../roles';
	import { session } from '../session.svelte';
	import { createView } from '../sync/data';
	import { engine } from '../sync/engine.svelte';
	import { layout } from '../layout.svelte';
	import { collapse } from '../motion';
	import { toasts } from '../toasts.svelte';

	// The style guide gives it a sample workspace and person.
	let { ws: sampleWs, me: sampleMe }: { ws?: string; me?: string } = $props();
	const ws = $derived(sampleWs ?? session.workspace?.id ?? '');
	const me = $derived(sampleMe ?? session.user?.id ?? '');
	const views = $derived(engine.replica(ws)?.views(me) ?? []);

	const open = $derived(layout.sidebarListOpen('views'));

	async function add() {
		layout.setSidebarList('views', true);
		try {
			const id = await createView(ws, {
				name: 'New view',
				type: 'list',
				projectId: null,
				ownerId: me,
				filter: { assignees: ['me'] }
			});
			await goto(resolve('/(app)/views/[id]', { id }));
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'Could not create the view.');
		}
	}
</script>

<section class="section">
	<div class="heading">
		<h2>
			<!-- A collapsed sidebar's lists always show, so there is nothing to fold. -->
			{#if layout.collapsed}
				Views
			{:else}
				<button
					class="fold"
					type="button"
					title={open ? 'Hide views' : 'Show views'}
					aria-expanded={open}
					aria-controls={open && views.length ? 'sidebar-views' : undefined}
					onclick={() => layout.setSidebarList('views', !open)}
				>
					<ChevronRight size={12} class={open ? 'turned' : undefined} />Views
				</button>
			{/if}
		</h2>
		<!-- A guest cannot make views; the server would refuse it. -->
		{#if !session.workspace || canEditViews(session.workspace.role)}
			<button
				class="icon"
				type="button"
				aria-label="New view"
				title={layout.collapsed ? 'New view' : undefined}
				onclick={add}
			>
				<Plus size={16} />
			</button>
		{/if}
	</div>
	<!-- It slides when folded and unfolded, and not when the first view arrives. -->
	{#if open}
		<div transition:collapse>
			{#if views.length}
				<ul id="sidebar-views" aria-label="Saved views">
					{#each views as v (v.id)}
						{@const href = resolve('/(app)/views/[id]', { id: v.id })}
						<li>
							<a
								{href}
								use:menu={viewActions(ws, v.id)}
								aria-current={page.url.pathname === href ? 'page' : undefined}
								title={layout.collapsed ? v.name : undefined}
							>
								{#if v.ownerId}<Layers size={16} />{:else}<Users size={16} />{/if}
								<span class="name">{v.name}</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}
</section>

<style>
	.fold {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 24px);
		padding: 0;
		border: none;
		background: none;
		color: inherit;
		font: inherit;
		letter-spacing: inherit;
		text-transform: inherit;
		cursor: pointer;
	}
	.fold:hover {
		color: var(--text-secondary);
	}
	.fold :global(.turned) {
		transform: rotate(90deg);
	}
	.heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 var(--space-1) 0 var(--space-2);
	}
	h2 {
		font-family: var(--font-ui);
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.icon {
		display: grid;
		place-items: center;
		width: max(var(--tap-min), 24px);
		height: max(var(--tap-min), 24px);
		padding: 0;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-muted);
		cursor: pointer;
	}
	.icon:hover {
		background: var(--sidebar-item-hover);
		color: var(--sidebar-text-active);
	}
	ul {
		display: grid;
		/* No wider than the sidebar, so a long name ends in an ellipsis. */
		grid-template-columns: minmax(0, 1fr);
		gap: 2px;
		margin: var(--space-1) 0 0;
		padding: 0;
		list-style: none;
	}
	a {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		height: calc(30px * var(--control-scale));
		padding: 0 var(--space-2);
		border-radius: var(--radius-md);
		color: var(--sidebar-text);
	}
	a:hover {
		background: var(--sidebar-item-hover);
		color: var(--sidebar-text-active);
	}
	a[aria-current='page'] {
		background: var(--sidebar-item-active);
		color: var(--sidebar-text-active);
		font-weight: 550;
	}
	.name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
