<script lang="ts">
	// What a template holds, read-only: a project's statuses, fields, wiki
	// pages, views and tasks, or a task's subtasks and checklist. Dates
	// read as days from the template's first, since it has no calendar.
	import StatusIcon from '../StatusIcon.svelte';
	import type { Template, TemplateTask } from '../../templates';

	let { template }: { template: Template } = $props();
	const p = $derived(template.data.project);
	const depthOf = $derived.by(() => {
		const byId = new Map(template.data.tasks.map((t) => [t.id, t]));
		return (t: TemplateTask) => {
			let d = 0;
			for (let at = t.parent; at && d < 20; at = byId.get(at)?.parent ?? null) d++;
			return d;
		};
	});
	const titleOf = $derived.by(() => {
		const byId = new Map(template.data.tasks.map((t) => [t.id, t.title]));
		return (id: string) => byId.get(id) ?? '';
	});

	function days(t: TemplateTask): string {
		const a = t.start ?? t.due;
		const b = t.due ?? t.start;
		if (a === null || b === null) return 'No dates';
		if (template.kind === 'task') {
			if (a === b)
				return b === 0 ? 'Due the day it is made' : `Due ${b} ${b === 1 ? 'day' : 'days'} after`;
			return `Days ${a} to ${b} after`;
		}
		return a === b ? `Day ${a + 1}` : `Day ${a + 1} to ${b + 1}`;
	}
</script>

{#if template.kind === 'project' && p}
	<div class="parts">
		<div class="part">
			<h4>Statuses</h4>
			<ul class="chips" aria-label="Statuses">
				{#each p.statuses as s (s.id)}<li class="chip">
						<StatusIcon category={s.category} size={12} />{s.name}
					</li>{:else}<li class="none">None</li>{/each}
			</ul>
		</div>
		<div class="part">
			<h4>Fields</h4>
			<ul class="chips" aria-label="Fields">
				{#each p.fields as f (f.id)}<li class="chip">{f.name}</li>{:else}<li class="none">
						None of its own
					</li>{/each}
			</ul>
		</div>
		<div class="part">
			<h4>Wiki pages</h4>
			<ul class="chips" aria-label="Wiki pages">
				{#each p.pages as pg (pg.id)}<li class="chip">
						{pg.parent ? pg.title || 'Untitled' : p.name}
					</li>{:else}<li class="none">None</li>{/each}
			</ul>
		</div>
		<div class="part">
			<h4>Saved views</h4>
			<ul class="chips" aria-label="Saved views">
				{#each p.views as v, i (i)}<li class="chip">{v.name}</li>{:else}<li class="none">
						None
					</li>{/each}
			</ul>
		</div>
	</div>
{/if}

{#if template.kind === 'page'}
	<section>
		<h4>{(template.data.pages?.length ?? 0) > 1 ? 'The page and its subpages' : 'The page'}</h4>
		<ul>
			{#each template.data.pages ?? [] as p (p.id)}<li>
					<span class="t">{p.title || 'Untitled'}</span>
				</li>{/each}
		</ul>
	</section>
{/if}
{#if template.kind === 'task' || template.data.tasks.length}
	<div class="part">
		<h4>{template.kind === 'project' ? 'Tasks' : 'The task and its subtasks'}</h4>
		<ul class="tree" aria-label="Tasks">
			{#each template.data.tasks as t (t.id)}
				<li style:padding-left={`${8 + depthOf(t) * 18}px`}>
					<span class="t"
						>{t.title}{#if t.waits.length}<span class="waits">
								waits for {t.waits.map(titleOf).join(', ')}</span
							>{/if}</span
					>
					<span class="d tabular">{days(t)}</span>
				</li>
			{/each}
		</ul>
	</div>
{/if}

{#if template.kind === 'task' && template.data.tasks[0]?.checklist.length}
	<div class="part">
		<h4>Checklist</h4>
		<ul class="tree" aria-label="Checklist">
			{#each template.data.tasks[0].checklist as c, i (i)}<li><span class="t">{c}</span></li>{/each}
		</ul>
	</div>
{/if}

<style>
	.parts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
		gap: var(--space-3);
	}
	h4 {
		margin: 0 0 var(--space-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 650;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 1px var(--space-2);
		border-radius: var(--radius-full);
		background: var(--surface-2);
		font-size: var(--text-xs);
	}
	.none {
		color: var(--text-muted);
		font-size: var(--text-xs);
	}
	.tree {
		display: grid;
		gap: 1px;
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: var(--text-sm);
	}
	.tree li {
		display: flex;
		gap: var(--space-3);
		justify-content: space-between;
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-sm);
	}
	.tree li:nth-child(odd) {
		background: var(--surface-1);
	}
	.part {
		min-width: 0;
	}
	.t {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.waits {
		margin-left: var(--space-2);
		color: var(--text-muted);
		font-size: var(--text-xs);
	}
	.d {
		flex: none;
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
</style>
