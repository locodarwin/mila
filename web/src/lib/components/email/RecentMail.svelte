<script lang="ts">
	// The mail that reached a person's addresses lately, and what became of it.
	// Refused mail is listed by who it was from and its subject only.
	import type { Schemas } from '../../api/client';
	import { relative } from '../../time';
	import Button from '../Button.svelte';

	type Received = Schemas['EmailReceived'];

	interface Props {
		items: Received[];
		onopen: (taskId: string) => void;
	}

	let { items, onopen }: Props = $props();

	const WORDS: Record<Received['outcome'], string> = {
		made: 'Made a task',
		refused_sender: 'Refused: sender not listed',
		refused_automatic: 'Refused: sent automatically',
		refused_spam: 'Refused: marked as spam',
		refused_forged: 'Refused: failed its sender checks',
		refused_limit: 'Refused: 50 tasks in a day already',
		refused_place: 'Refused: its project is gone or archived',
		refused_too_large: 'Refused: too large',
		failed: 'Could not make a task of this'
	};
</script>

{#if items.length}
	<div class="wrap">
		<table>
			<thead>
				<tr
					><th scope="col">When</th><th scope="col">From</th><th scope="col">Subject</th><th
						scope="col">What happened</th
					></tr
				>
			</thead>
			<tbody>
				{#each items as m (m.id)}
					<tr>
						<td class="when">{relative(m.receivedAt)}</td>
						<td>{m.from || 'Nobody named'}</td>
						<td>{m.subject || 'No subject'}</td>
						<td>
							<span class="said" class:made={m.outcome === 'made'}>{WORDS[m.outcome]}</span>
							{#if m.taskId}
								{@const task = m.taskId}
								<Button
									size="sm"
									variant="ghost"
									title="Open the task this made"
									onclick={() => onopen(task)}>Open</Button
								>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<p class="empty">Nothing has arrived yet.</p>
{/if}

<style>
	.wrap {
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
		vertical-align: baseline;
		overflow-wrap: anywhere;
	}
	th {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.when {
		white-space: nowrap;
		color: var(--text-secondary);
	}
	.said {
		color: var(--text-secondary);
	}
	.said.made {
		color: var(--text-primary);
	}
	.empty {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
