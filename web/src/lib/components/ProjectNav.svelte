<script lang="ts">
	import { menu, projectActions } from '../contextMenu.svelte';
	import { focusOnShow } from '../focus';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import Hash from '@lucide/svelte/icons/hash';
	import Plus from '@lucide/svelte/icons/plus';
	import { session } from '../session.svelte';
	import { tick } from 'svelte';
	import { createProject, moveProject } from '../sync/data';
	import ProjectIcon from './ProjectIcon.svelte';
	import { engine } from '../sync/engine.svelte';
	import { toasts } from '../toasts.svelte';
	import { announce } from '../announce.svelte';
	import { layout } from '../layout.svelte';
	import { collapse } from '../motion';
	import { canOrganize } from '../roles';

	const ws = $derived(session.workspace?.id ?? '');
	const projects = $derived(engine.replica(ws)?.projects() ?? []);
	const organize = $derived(!session.workspace || canOrganize(session.workspace.role));
	const open = $derived(layout.sidebarListOpen('projects'));
	let adding = $state(false);
	let name = $state('');

	// Alt+Up and Alt+Down move the focused project, as they move tasks.
	async function reorder(e: KeyboardEvent, id: string) {
		if (!organize || !e.altKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return;
		e.preventDefault();
		const by = e.key === 'ArrowUp' ? -1 : 1;
		const i = projects.findIndex((p) => p.id === id);
		const to = i + by;
		if (i < 0 || to < 0 || to >= projects.length) return;
		announce(`Moved ${projects[i].name} to ${to + 1} of ${projects.length}`);
		await moveProject(ws, id, by);
		await tick();
		document.querySelector<HTMLElement>(`[data-project-nav="${id}"]`)?.focus();
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		const n = name.trim();
		adding = false;
		name = '';
		if (!n) return;
		try {
			const id = await createProject(ws, n);
			await goto(resolve('/(app)/projects/[id]', { id }));
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'Could not create the project.');
		}
	}
</script>

<section class="section">
	<div class="heading">
		<h2>
			<!-- A collapsed sidebar's lists always show, so there is nothing to fold. -->
			{#if layout.collapsed}
				Projects
			{:else}
				<button
					class="fold"
					type="button"
					title={open ? 'Hide projects' : 'Show projects'}
					aria-expanded={open}
					aria-controls={open ? 'sidebar-projects' : undefined}
					onclick={() => layout.setSidebarList('projects', !open)}
				>
					<ChevronRight size={12} class={open ? 'turned' : undefined} />Projects
				</button>
			{/if}
		</h2>
		{#if organize}
			<button
				class="icon"
				type="button"
				aria-label="New project"
				title={layout.collapsed ? 'New project' : undefined}
				onclick={() => {
					layout.setSidebarList('projects', true);
					adding = true;
				}}
			>
				<Plus size={16} />
			</button>
		{/if}
	</div>
	{#if open}
		<ul id="sidebar-projects" transition:collapse>
			{#each projects as p (p.id)}
				{@const href = resolve('/(app)/projects/[id]', { id: p.id })}
				<li>
					<a
						{href}
						use:menu={projectActions(ws, p.id)}
						aria-current={page.url.pathname === href ? 'page' : undefined}
						data-project-nav={p.id}
						title={layout.collapsed ? p.name : undefined}
						aria-keyshortcuts="Alt+ArrowUp Alt+ArrowDown"
						onkeydown={(e) => reorder(e, p.id)}
					>
						<ProjectIcon icon={p.icon} color={p.color} />
						<span class="name">{p.name}</span>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
	{#if adding}
		<form onsubmit={add}>
			<Hash size={14} />
			<input
				aria-label="New project name"
				placeholder="Project name"
				bind:value={name}
				use:focusOnShow
				onblur={() => !name && (adding = false)}
				onkeydown={(e) => {
					// Handled here, so the open task stays open (TaskDetail).
					if (e.key === 'Escape') {
						e.preventDefault();
						adding = false;
					}
				}}
			/>
		</form>
	{:else if open && projects.length === 0}
		<p>Nothing here yet</p>
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
	a,
	form {
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
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	input {
		flex: 1;
		min-width: 0;
		height: 26px;
		padding: 0 var(--space-1);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-sm);
		background: var(--input-bg);
		color: var(--input-text);
	}
	input::placeholder {
		color: var(--input-placeholder);
	}
	p {
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-muted);
	}
</style>
