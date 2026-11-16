<script lang="ts">
	// The style guide's Time section: badges, a task's Time, the Time page
	// with its Log time form, and a project's Time tab on the sample workspace. Loaded apart, as the
	// app loads them.
	import TimeBadge from '#lib/components/TimeBadge.svelte';
	import TimeSection from '#lib/components/task/TimeSection.svelte';
	import LogProjectTime from '#lib/components/time/LogProjectTime.svelte';
	import ProjectTime from '#lib/components/time/ProjectTime.svelte';
	import TimeReport from '#lib/components/time/TimeReport.svelte';
	import type { Replica } from '#lib/sync/replica.svelte.js';
	import { SAMPLE_WS } from './sample';

	let { replica }: { replica: Replica } = $props();
	const beds = $derived(replica.task('t-beds'));
	const seeds = $derived(replica.task('t-seeds'));
	const fence = $derived(replica.task('t-fence'));
	const all = () => true;
	const garden = $derived(beds?.projectId ?? null);
</script>

<div class="badges">
	{#each [beds, seeds, fence] as t (t?.id)}
		{#if t}<span class="badge-demo">{t.title} <TimeBadge ws={SAMPLE_WS} task={t} show={all} /></span
			>{/if}
	{/each}
</div>
<div class="demo">
	{#if beds}<div class="section"><TimeSection ws={SAMPLE_WS} {replica} task={beds} /></div>{/if}
	<div class="report-demo"><TimeReport ws={SAMPLE_WS} /></div>
</div>
<div class="log-demo">
	<LogProjectTime
		ws={SAMPLE_WS}
		projects={replica.projects()}
		start={garden ?? ''}
		focus={false}
		onclose={() => {}}
	/>
</div>
{#if garden}
	<div class="project-demo"><ProjectTime ws={SAMPLE_WS} projectId={garden} /></div>
{/if}

<style>
	.badges {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
		font-size: var(--text-sm);
	}
	.badge-demo {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
	}
	.demo {
		display: grid;
		grid-template-columns: minmax(0, 380px) minmax(0, 1fr);
		gap: var(--space-4);
		align-items: start;
		margin-top: var(--space-3);
	}
	.section,
	.report-demo,
	.project-demo {
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	.section {
		padding: var(--space-3);
	}
	.project-demo,
	.log-demo {
		margin-top: var(--space-4);
	}
	@media (max-width: 900px) {
		.demo {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>
