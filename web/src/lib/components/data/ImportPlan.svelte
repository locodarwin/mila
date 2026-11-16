<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { Plan } from '../../importplan';

	interface Props {
		plan: Plan;
		/** Where tasks with no project go, for the summary. */
		destination: string;
		/** What one of the file's entries is called, from its line: "Row 4" by default. */
		item?: (line: number) => string;
		/** The entries' name in the summary, one and many. */
		unit?: [string, string];
		/** What the file's own reader adds to the summary. */
		children?: Snippet;
	}

	let {
		plan,
		destination,
		item = (line) => `Row ${line}`,
		unit = ['row', 'rows'],
		children
	}: Props = $props();
	const SHOWN = 20;
	const projects = $derived(new Set(plan.tasks.map((t) => JSON.stringify(t.project))).size);
	const made = $derived(
		[
			plan.newProjects.length && `projects ${plan.newProjects.join(', ')}`,
			plan.newStatuses.length && `statuses ${plan.newStatuses.map((s) => s.name).join(', ')}`,
			plan.newLabels.length && `labels ${plan.newLabels.join(', ')}`,
			plan.newOptions.length && `field choices ${plan.newOptions.map((o) => o.name).join(', ')}`
		].filter(Boolean) as string[]
	);
	const where = (lines: number[]) =>
		lines.slice(0, 5).map(item).join(', ') +
		(lines.length > 5 ? ` and ${lines.length - 5} more` : '');
</script>

<div class="plan" role="region" aria-label="What the import would do">
	<p class="headline">
		{plan.tasks.length}
		{plan.tasks.length === 1 ? 'task' : 'tasks'} would come in{projects > 1
			? `, into ${projects} places`
			: `, into ${destination}`}.
		{#if plan.skipped.length}
			{plan.skipped.length}
			{plan.skipped.length === 1 ? `${unit[0]} is` : `${unit[1]} are`} left out.
		{/if}
	</p>
	{@render children?.()}
	{#if made.length}
		<p>New: {made.join('; ')}.</p>
	{/if}
	{#if plan.skipped.length}
		<h3>Left out</h3>
		<ul>
			{#each plan.skipped.slice(0, SHOWN) as s, i (i)}<li>{item(s.line)}: {s.reason}</li>{/each}
			{#if plan.skipped.length > SHOWN}<li>and {plan.skipped.length - SHOWN} more</li>{/if}
		</ul>
	{/if}
	{#if plan.warnings.length}
		<h3>Coming in without a value</h3>
		<ul>
			{#each plan.warnings.slice(0, SHOWN) as w, i (i)}<li>{item(w.line)}: {w.message}</li>{/each}
			{#if plan.warnings.length > SHOWN}<li>and {plan.warnings.length - SHOWN} more</li>{/if}
		</ul>
	{/if}
	{#if plan.strangers.length}
		<h3>Not people in this workspace</h3>
		<p class="note">They are left off the tasks that name them.</p>
		<ul>
			{#each plan.strangers.slice(0, SHOWN) as s, i (i)}<li>{s.name}: {where(s.lines)}</li>{/each}
			{#if plan.strangers.length > SHOWN}<li>and {plan.strangers.length - SHOWN} more</li>{/if}
		</ul>
	{/if}
	{#if plan.unassigned.length}
		<h3>Coming in unassigned</h3>
		<p class="note">
			{plan.unassigned.slice(0, SHOWN).map(item).join(', ')}{plan.unassigned.length > SHOWN
				? `, and ${plan.unassigned.length - SHOWN} more`
				: ''}.
		</p>
	{/if}
</div>

<style>
	.plan {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-1);
		font-size: var(--text-sm);
	}
	p,
	.plan :global(ul) {
		margin: 0;
	}
	.plan :global(ul) {
		padding-left: var(--space-5);
		color: var(--text-secondary);
	}
	.note {
		color: var(--text-secondary);
	}
	.headline {
		font-weight: 600;
	}
	h3 {
		font-size: var(--text-sm);
	}
</style>
