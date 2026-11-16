<script lang="ts">
	import { resolve } from '$app/paths';
	// The menu beside New task (docs/PLAN.md section 3): a task from one of
	// the workspace's task templates, made in the project on screen or the
	// inbox.
	import { page } from '$app/state';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import { anchored } from '../../popover';
	import { engine } from '../../sync/engine.svelte';
	import { contents } from '../../templateInfo';
	import { uuid } from '../../uuid';

	let { ws }: { ws: string } = $props();
	const id = `task-templates-${uuid()}`;
	const templates = $derived(
		(engine.replica(ws)?.templates() ?? []).filter((t) => t.kind === 'task')
	);
	// The project on screen, if any: a task made here lands in it.
	const projectId = $derived(
		page.route.id?.startsWith('/(app)/projects/[id]') ? (page.params.id ?? null) : null
	);
	let menu = $state<HTMLElement>();

	async function make(templateId: string) {
		menu?.hidePopover();
		const { newTaskFromTemplate } = await import('../../templateActions');
		await newTaskFromTemplate(ws, templateId, projectId);
	}
</script>

<button
	class="more"
	type="button"
	popovertarget={id}
	aria-label="Make a task from a template"
	title="Make a task from a template"><ChevronDown size={14} /></button
>
<div {id} popover class="menu" use:anchored bind:this={menu}>
	<p class="heading">From a template</p>
	{#each templates as t (t.id)}
		<button type="button" class="option" onclick={() => void make(t.id)}>
			<span class="name">{t.name}</span><span class="meta">{contents(t)}</span>
		</button>
	{:else}
		<p class="none">No task templates yet. Save one from a task's panel.</p>
	{/each}
	<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved -->
	<a
		class="option"
		href={resolve('/(app)/settings/w/[ws]/templates', { ws })}
		onclick={() => menu?.hidePopover()}>Manage templates</a
	>
</div>

<style>
	.more {
		display: grid;
		flex: none;
		place-items: center;
		width: max(var(--tap-min), 28px);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.more:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	/* base.css lays out .menu .option; these are buttons and a link. */
	.option {
		width: 100%;
		border: none;
		background: none;
		color: var(--text-primary);
		text-align: left;
		text-decoration: none;
		cursor: pointer;
	}
	.name {
		flex: 1;
	}
	.meta,
	.none {
		color: var(--text-muted);
		font-size: var(--text-xs);
	}
	.none {
		margin: 0;
		padding: var(--space-1) var(--space-2);
	}
</style>
