<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import AddTask from '#lib/components/AddTask.svelte';
	import TaskList from '#lib/components/TaskList.svelte';
	import ListViews from '#lib/components/views/ListViews.svelte';
	import { canOrganize } from '#lib/roles.js';
	import { session } from '#lib/session.svelte.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import type { Task } from '#lib/sync/entities.js';

	const guest = $derived(!!session.workspace && !canOrganize(session.workspace.role));
	const ws = $derived(session.workspace?.id ?? '');
	// Top-level tasks in no project.
	const scope = $derived(engine.replica(ws)?.tasks(null) ?? []);
	const sectionsOf = (keep: (t: Task) => boolean) => [
		{ id: 'inbox', title: 'Inbox', tasks: scope.filter(keep) }
	];
	const emptyTitle = $derived(
		session.workspace?.isPersonal ? 'Your inbox is clear' : 'The inbox is clear'
	);
	const emptyHint = $derived(
		session.workspace?.isPersonal
			? 'Anything you add here waits until you give it a project.'
			: 'Everyone in this workspace shares it. Anything added here waits until someone gives it a project.'
	);

	// A guest has no inbox: their work is in the projects shared with them.
	// However they came here, on to Projects, with any task they were opening.
	$effect(() => {
		if (!guest) return;
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() plus a query
		void goto(`${resolve('/(app)/projects')}${page.url.search}`, { replaceState: true });
	});
</script>

<svelte:head><title>Inbox · Mila</title></svelte:head>

{#if guest}
	<!-- On the way to Projects. -->
{:else if session.workspace}
	<ListViews
		{ws}
		list="inbox"
		title="Inbox"
		{scope}
		{sectionsOf}
		label="Inbox"
		{emptyTitle}
		{emptyHint}
	>
		{#snippet own()}
			<TaskList {ws} projectId={null} {emptyTitle} {emptyHint} />
		{/snippet}
		{#snippet add()}
			<AddTask {ws} projectId={null} />
		{/snippet}
	</ListViews>
{/if}
