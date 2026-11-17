<script lang="ts">
	// A row's or card's time: the time logged
	// against the estimate, red past it, as the view shows them; and the
	// person's own timer, ticking, when it runs on this task.
	import Clock from '@lucide/svelte/icons/clock';
	import { timeBadge } from '../duration';
	import { session } from '../session.svelte';
	import { engine } from '../sync/engine.svelte';
	import type { Task } from '../sync/entities';
	import Stopwatch from './Stopwatch.svelte';

	interface Props {
		ws: string;
		task: Task;
		show: (part: string) => boolean;
		/** What is left of the estimate too: only when a view asks for it. */
		left?: boolean;
		/** Words before what is said, as a card's run-on name needs. */
		lead?: string;
	}

	let { ws, task, show, left = false, lead = '' }: Props = $props();
	const replica = $derived(engine.replica(ws));
	const timer = $derived(session.user ? replica?.runningTimer(session.user.id) : undefined);
	const running = $derived(timer?.taskId === task.id ? timer : undefined);
	const badge = $derived(
		timeBadge(task.estimateMinutes, replica?.logged(task.id) ?? 0, {
			estimate: show('estimate'),
			logged: show('logged'),
			remaining: left
		})
	);
</script>

{#if running}<span class="badge running" title="Your timer is running"
		><span class="dot" aria-hidden="true"></span><Stopwatch since={running.startedAt} short /><span
			class="visually-hidden">{lead}your timer is running</span
		></span
	>{/if}
{#if badge}<span class="badge tabular" class:over={badge.over} title="Time logged and estimate"
		><Clock size={12} /><span aria-hidden="true">{badge.text}</span><span class="visually-hidden"
			>{lead}{badge.said}</span
		></span
	>{/if}

<style>
	.badge {
		/* Holds its hidden words, which would otherwise stretch the page. */
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 3px;
		font-size: var(--text-xs);
		color: var(--row-meta-text);
		white-space: nowrap;
	}
	.over {
		color: var(--danger);
		font-weight: 550;
	}
	.running {
		color: var(--accent-text);
		font-weight: 600;
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: var(--radius-full);
		background: var(--accent);
	}
</style>
