<script lang="ts">
	import { menu, projectActions } from '#lib/contextMenu.svelte.js';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import LayoutTemplate from '@lucide/svelte/icons/layout-template';
	import Plus from '@lucide/svelte/icons/plus';
	import Button from '#lib/components/Button.svelte';
	import { canOrganize, canUseTemplates } from '#lib/roles.js';
	import EmptyState from '#lib/components/EmptyState.svelte';
	import ProjectIcon from '#lib/components/ProjectIcon.svelte';
	import { session } from '#lib/session.svelte.js';
	import { createProject } from '#lib/sync/data.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';

	const ws = $derived(session.workspace?.id ?? '');
	const replica = $derived(engine.replica(ws));
	const projects = $derived(replica?.projects() ?? []);
	const archived = $derived(replica?.archivedProjects() ?? []);
	let name = $state('');
	// New project from a template; ?template=<id> opens it on that one.
	let fromTemplate = $state(false);
	let opened = $state(false);
	const preselected = $derived(page.url.searchParams.get('template'));
	$effect(() => {
		if (preselected !== null) fromTemplate = opened = true;
	});

	function openCount(projectId: string) {
		return replica?.tasks(projectId).filter((t) => t.completedAt === null).length ?? 0;
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		const n = name.trim();
		if (!n) return;
		name = '';
		try {
			const id = await createProject(ws, n);
			await goto(resolve('/(app)/projects/[id]', { id }));
		} catch (err) {
			toasts.error(err instanceof Error ? err.message : 'Could not create the project.');
		}
	}
</script>

<svelte:head><title>Projects · Mila</title></svelte:head>

<header class="page"><h1>Projects</h1></header>
{#if !session.workspace || canOrganize(session.workspace.role)}
	<form class="add" onsubmit={add}>
		<Plus size={16} />
		<input aria-label="Name a new project" placeholder="New project" bind:value={name} />
		{#if session.workspace && canUseTemplates(session.workspace.role)}
			<Button size="sm" variant="ghost" onclick={() => (fromTemplate = opened = true)}
				><LayoutTemplate size={14} />From a template</Button
			>
		{/if}
	</form>
{/if}
{#if opened}
	<!-- Loaded the first time it opens. -->
	{#await import('#lib/components/templates/NewProject.svelte') then { default: NewProject }}
		<NewProject bind:open={fromTemplate} {ws} templateId={preselected} />
	{/await}
{/if}
{#if projects.length === 0}
	<EmptyState title="No projects yet" hint="Name one above to get started." />
{:else}
	<ul aria-label="Projects">
		{#each projects as p (p.id)}
			{@const open = openCount(p.id)}
			<li>
				<a href={resolve('/(app)/projects/[id]', { id: p.id })} use:menu={projectActions(ws, p.id)}>
					<ProjectIcon icon={p.icon} color={p.color} />
					<span class="name">{p.name}</span>
					<span class="count tabular">{open} open</span>
				</a>
			</li>
		{/each}
	</ul>
{/if}
{#if archived.length}
	<section class="archived" aria-labelledby="archived-heading">
		<h2 id="archived-heading">Archived</h2>
		<ul aria-label="Archived projects">
			{#each archived as p (p.id)}
				<li>
					<a
						href={resolve('/(app)/projects/[id]', { id: p.id })}
						use:menu={projectActions(ws, p.id)}
					>
						<ProjectIcon icon={p.icon} color={p.color} />
						<span class="name">{p.name}</span>
					</a>
				</li>
			{/each}
		</ul>
	</section>
{/if}

<style>
	.add {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: 0 var(--pad-x);
		border-bottom: 1px solid var(--border);
		color: var(--text-muted);
	}
	.add input {
		flex: 1;
		height: var(--row-height);
		border: none;
		background: none;
		color: var(--text-primary);
	}
	.add input:focus-visible {
		outline: none;
	}
	.add:focus-within {
		box-shadow: inset 2px 0 0 var(--accent);
	}
	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	a {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: 0 var(--pad-x);
		border-bottom: 1px solid var(--border);
	}
	a:hover {
		background: var(--surface-1);
	}
	.archived h2 {
		padding: var(--space-5) var(--pad-x) var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.archived a {
		color: var(--text-secondary);
	}
	.name {
		flex: 1;
	}
	.count {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
</style>
