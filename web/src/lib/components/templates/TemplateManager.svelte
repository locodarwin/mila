<script lang="ts">
	import { settingsWorkspace } from '../../settingsWorkspace.svelte';
	// Settings, Templates: the workspace's
	// templates, a preview of the one chosen, and making, renaming and
	// deleting them. Loaded only on that page.
	import FileText from '@lucide/svelte/icons/file-text';
	import FolderKanban from '@lucide/svelte/icons/folder-kanban';
	import ListTodo from '@lucide/svelte/icons/list-todo';
	import Button from '../Button.svelte';
	import { announce } from '../../announce.svelte';
	import { formatDate } from '../../dates';
	import { session } from '../../session.svelte';
	import { engine } from '../../sync/engine.svelte';
	import { contents } from '../../templateInfo';
	import type { Template } from '../../templates';
	import {
		newPageForTemplate,
		newTaskFromTemplate,
		renameTemplate,
		setTemplateDeleted
	} from '../../templateActions';
	import { goto } from '$app/navigation';
	import { wikiMain } from '../../sync/pages';
	import { toasts } from '../../toasts.svelte';
	import NewProject from './NewProject.svelte';
	import TemplatePreview from './TemplatePreview.svelte';

	let { ws }: { ws: string } = $props();
	const templates = $derived(engine.replica(ws)?.templates() ?? []);
	let chosen = $state<string | null>(null);
	const current = $derived(templates.find((t) => t.id === chosen) ?? templates[0]);
	let renaming = $state(false);
	let name = $state('');
	let making = $state(false);

	const groups = $derived([
		{ kind: 'project', title: 'Projects', items: templates.filter((t) => t.kind === 'project') },
		{ kind: 'task', title: 'Tasks', items: templates.filter((t) => t.kind === 'task') },
		{ kind: 'page', title: 'Pages', items: templates.filter((t) => t.kind === 'page') }
	]);
	const KIND_WORD = { project: 'a project', task: 'a task', page: 'a page' };
	const KIND_NAME = { project: 'Project', task: 'Task', page: 'Page' };

	// A page from a template goes in the Workspace Wiki, under its main page.
	async function newPage(t: Template) {
		const address = await newPageForTemplate(ws, t.id, {
			projectId: null,
			parentId: wikiMain(ws, null)
		});
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- a page's address, made by pageHref
		await goto(address);
	}

	const savedSaid = (t: Template) =>
		[
			t.savedBy ? `saved by ${session.nameOf(ws, t.savedBy)}` : '',
			t.savedAt ? formatDate(t.savedAt.slice(0, 10)) : ''
		]
			.filter(Boolean)
			.join(', ');

	function choose(t: Template) {
		chosen = t.id;
		renaming = false;
	}

	async function rename(e: SubmitEvent) {
		e.preventDefault();
		if (!current || !name.trim()) return;
		await renameTemplate(ws, current.id, name);
		renaming = false;
		announce(`Renamed to ${name.trim()}`);
	}

	function remove(t: Template) {
		const { undo } = engine.track(() => setTemplateDeleted(ws, t.id, true));
		toasts.show(`Deleted the template ${t.name}. It stays in the trash for 30 days.`, {
			onundo: () => void undo()
		});
		chosen = null;
	}
</script>

<section class="templates">
	<h2>Templates{settingsWorkspace() ? ` in ${settingsWorkspace()?.name}` : ''}</h2>
	<p class="hint">
		Start a project, a task or a wiki page from one you made before. Save one from its menu.
		Everyone in the workspace can use them; guests don't see them.
	</p>
	{#if !templates.length}
		<p class="empty">
			No templates yet. Open a project, a task or a wiki page and choose Save as a template to make
			the first.
		</p>
	{:else}
		<div class="split">
			<div class="list">
				{#each groups as g (g.kind)}
					{#if g.items.length}
						<h3>{g.title}</h3>
						<ul aria-label={`${g.title} templates`}>
							{#each g.items as t (t.id)}
								<li>
									<button
										type="button"
										class="item"
										aria-pressed={current?.id === t.id}
										onclick={() => choose(t)}
									>
										<span class="ic" aria-hidden="true"
											>{#if t.kind === 'project'}<FolderKanban
													size={16}
												/>{:else if t.kind === 'page'}<FileText size={16} />{:else}<ListTodo
													size={16}
												/>{/if}</span
										>
										<span class="what">
											<span class="name">{t.name}</span>
											<span class="meta"
												>{contents(t)}{savedSaid(t) ? ` · ${savedSaid(t)}` : ''}</span
											>
										</span>
									</button>
								</li>
							{/each}
						</ul>
					{/if}
				{/each}
			</div>
			{#if current}
				<div
					class="preview"
					role="region"
					aria-label={`${current.name}, a ${current.kind} template`}
				>
					<div class="head">
						{#if renaming}
							<form class="rename" onsubmit={rename}>
								<input aria-label="Template name" bind:value={name} maxlength="120" required />
								<Button type="submit" variant="primary" size="sm">Save</Button>
								<Button size="sm" onclick={() => (renaming = false)}>Cancel</Button>
							</form>
						{:else}
							<h3>{current.name}</h3>
							<span class="meta">{KIND_NAME[current.kind]} template · {contents(current)}</span>
						{/if}
						<div class="actions">
							{#if current.kind === 'project'}
								<Button variant="primary" size="sm" onclick={() => (making = true)}
									>New project from it</Button
								>
							{:else if current.kind === 'page'}
								<Button
									variant="primary"
									size="sm"
									title="Make a page from this template in the Workspace Wiki"
									onclick={() => void newPage(current)}>New page from it</Button
								>
							{:else}
								<Button
									variant="primary"
									size="sm"
									onclick={() => void newTaskFromTemplate(ws, current.id, null)}
									>New task from it</Button
								>
							{/if}
							<Button
								size="sm"
								onclick={() => {
									name = current.name;
									renaming = true;
								}}>Rename</Button
							>
							<Button size="sm" variant="danger" onclick={() => remove(current)}>Delete</Button>
						</div>
					</div>
					<TemplatePreview template={current} />
					<p class="meta">
						To change it, make {KIND_WORD[current.kind]} from it, edit that, and save it as a template
						again, replacing this one.
					</p>
				</div>
			{/if}
		</div>
	{/if}
</section>

<NewProject bind:open={making} {ws} templateId={current?.kind === 'project' ? current.id : null} />

<style>
	.templates {
		display: grid;
		gap: var(--space-3);
	}
	.hint,
	.empty,
	.meta {
		margin: 0;
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.split {
		display: grid;
		grid-template-columns: minmax(200px, 280px) minmax(0, 1fr);
		gap: var(--space-4);
		align-items: start;
	}
	@media (max-width: 860px) {
		.split {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	.list h3 {
		margin: var(--space-3) 0 var(--space-1);
		color: var(--text-muted);
		font-size: var(--text-xs);
		font-weight: 650;
	}
	ul {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.item {
		display: grid;
		grid-template-columns: 28px minmax(0, 1fr);
		gap: var(--space-3);
		align-items: center;
		width: 100%;
		min-height: max(var(--tap-min), 48px);
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		text-align: left;
		cursor: pointer;
	}
	.item[aria-pressed='true'] {
		border-color: var(--accent);
		background: var(--accent-soft);
	}
	.ic {
		display: grid;
		place-items: center;
		width: 28px;
		height: 28px;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		color: var(--text-secondary);
	}
	.what {
		display: grid;
		min-width: 0;
	}
	.name {
		overflow: hidden;
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.preview > :global(*) {
		min-width: 0;
	}
	.preview {
		display: grid;
		gap: var(--space-3);
		min-width: 0;
		padding: var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-0);
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2) var(--space-3);
	}
	.head h3 {
		margin: 0;
		font-size: var(--text-lg);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		margin-left: auto;
	}
	.rename {
		display: flex;
		gap: var(--space-2);
	}
	.rename input {
		min-height: max(var(--tap-min), 30px);
		padding: var(--space-1) var(--space-2);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
	}
</style>
