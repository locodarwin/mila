<script lang="ts">
	// One webhook: what it hears about, its controls, and its log, which
	// loads as it is opened.
	import { onMount } from 'svelte';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import type { Schemas } from '../../api/client';
	import { formatClock, formatDate } from '../../dates';
	import { relative } from '../../time';
	import Button from '../Button.svelte';

	type Webhook = Schemas['Webhook'];
	type Delivery = Schemas['WebhookDelivery'];

	interface Props {
		hook: Webhook;
		/** A project's name by its ID. */
		projectName: (id: string) => string;
		/** The log, newest first; null until it has loaded. */
		deliveries: Delivery[] | null;
		onopen: () => void;
		ontest: () => void;
		onpause: (paused: boolean) => void;
		onrotate: () => void;
		ondelete: () => void;
		onresend: (d: Delivery) => void;
		/** Shown open from the start (the style guide). */
		open?: boolean;
	}

	let {
		hook,
		projectName,
		deliveries,
		onopen,
		ontest,
		onpause,
		onrotate,
		ondelete,
		onresend,
		open = false
	}: Props = $props();
	let confirming = $state(false);
	// Only to begin with: from then on it is the person's to open and close.
	let details = $state<HTMLDetailsElement>();
	onMount(() => {
		if (open && details) details.open = true;
	});

	const FAMILIES: Record<Schemas['WebhookFamily'], string> = {
		task: 'Tasks',
		comment: 'Comments',
		project: 'Projects',
		page: 'Wiki pages',
		time: 'Time'
	};
	const STATES: Record<Webhook['state'], string> = {
		working: 'Working',
		failing: 'Failing',
		paused: 'Paused',
		stopped: 'Stopped: it failed for 3 days'
	};
	const heard = $derived(hook.events.map((e) => FAMILIES[e]).join(', '));

	function result(d: Delivery): string {
		if (d.status === 'pending' && d.nextAt && d.attempts > 0) {
			return `Trying again ${relative(d.nextAt)}`;
		}
		if (d.status === 'pending') return 'On its way';
		return d.responseCode ? String(d.responseCode) : 'No answer';
	}
</script>

<details class="hook" bind:this={details} ontoggle={(e) => e.currentTarget.open && onopen()}>
	<summary>
		<ChevronRight size={15} />
		<span class="url">{hook.url}</span>
		<span class="what"
			>{heard}{#if hook.projectId}&nbsp;· {projectName(hook.projectId)}{/if}</span
		>
		<span class="state {hook.state}">{STATES[hook.state]}</span>
	</summary>
	<div class="inner">
		<div class="tools">
			<Button size="sm" title="Send a ping to this address" onclick={ontest}>Send a test</Button>
			{#if hook.state === 'paused' || hook.state === 'stopped'}
				<Button
					size="sm"
					title="Start telling this address of changes again"
					onclick={() => onpause(false)}>Turn back on</Button
				>
			{:else}
				<Button
					size="sm"
					title="Stop telling this address of changes for now"
					onclick={() => onpause(true)}>Pause</Button
				>
			{/if}
			<Button
				size="sm"
				title="Make a new signing secret; the old one stops at once"
				onclick={onrotate}>Make a new secret</Button
			>
			<Button
				size="sm"
				variant="danger"
				title="Remove this webhook and its log"
				aria-label={`${confirming ? 'Confirm delete' : 'Delete'} the webhook for ${hook.url}`}
				onclick={() => {
					if (confirming) ondelete();
					confirming = !confirming;
				}}>{confirming ? 'Confirm delete' : 'Delete'}</Button
			>
		</div>
		{#if deliveries === null}
			<p class="hint">Loading…</p>
		{:else if deliveries.length === 0}
			<p class="hint">Nothing sent yet. The log keeps 30 days.</p>
		{:else}
			<div class="wide">
				<table>
					<thead>
						<tr>
							<th scope="col">What happened</th>
							<th scope="col">When</th>
							<th scope="col">Result</th>
							<th scope="col">Tries</th>
							<th scope="col"><span class="visually-hidden">Resend</span></th>
						</tr>
					</thead>
					<tbody>
						{#each deliveries as d (d.id)}
							<tr>
								<td><code>{d.event}</code> {d.summary}</td>
								<td
									title={`${formatDate(d.createdAt.slice(0, 10))} ${formatClock(new Date(d.createdAt))}`}
									>{relative(d.createdAt)}</td
								>
								<td title={d.responseBody || undefined}>
									<span class="result {d.status}">{result(d)}</span>
								</td>
								<td>{d.attempts}</td>
								<td>
									{#if d.status === 'failed'}
										<Button
											size="sm"
											title="Send it again: the same body, with a new signature"
											aria-label={`Resend ${d.event} ${d.summary}`}
											onclick={() => onresend(d)}>Resend</Button
										>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</details>

<style>
	.hook {
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
	}
	summary {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1) var(--space-3);
		padding: var(--space-2) var(--space-3);
		cursor: pointer;
		list-style: none;
	}
	summary::-webkit-details-marker {
		display: none;
	}
	summary :global(svg) {
		flex: none;
		transition: transform var(--dur-micro) var(--ease);
	}
	details[open] summary :global(svg) {
		transform: rotate(90deg);
	}
	.url {
		flex: 1 1 14rem;
		min-width: 0;
		font-family: var(--font-mono);
		font-size: var(--text-sm);
		overflow-wrap: anywhere;
	}
	.what {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.state,
	.result {
		font-size: var(--text-xs);
		font-weight: 600;
		white-space: nowrap;
	}
	.state.working,
	.result.delivered {
		color: var(--text-primary);
	}
	.state.failing,
	.state.stopped,
	.result.failed {
		color: var(--danger);
	}
	.state.paused,
	.result.pending {
		color: var(--text-secondary);
	}
	.inner {
		display: grid;
		gap: var(--space-3);
		padding: 0 var(--space-3) var(--space-3);
	}
	.tools {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.wide {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
	}
	th,
	td {
		padding: var(--space-1) var(--space-3) var(--space-1) 0;
		border-bottom: var(--border-width) solid var(--border);
		text-align: left;
		vertical-align: middle;
	}
	th {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
		white-space: nowrap;
	}
	code {
		font-family: var(--font-mono);
		font-size: var(--text-xs);
	}
</style>
