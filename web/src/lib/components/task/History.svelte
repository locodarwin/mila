<script lang="ts">
	import { api, call, messageOf, type Schemas } from '../../api/client';
	import { describeAfter, tryParseRule, type RepeatMode } from '../../recurrence';
	import { engine } from '../../sync/engine.svelte';
	import { relative } from '../../time';
	import { formatDuration } from '../../duration';

	let { ws, taskId }: { ws: string; taskId: string } = $props();
	let events = $state<Schemas['ActivityEvent'][] | null>(null);
	let error = $state('');

	// History lives on the server; it needs a connection.
	$effect(() => {
		void taskId;
		events = null;
		call(
			api.GET('/workspaces/{workspaceId}/tasks/{taskId}/activity', {
				params: { path: { workspaceId: ws, taskId } }
			})
		)
			.then((e) => (events = e))
			.catch((err) => (error = messageOf(err)));
	});

	const fieldNames: Record<string, string> = {
		title: 'the title',
		notes: 'the notes',
		priority: 'the priority',
		start_date: 'the start date',
		due_date: 'the due date',
		due_time: 'the due time',
		status_id: 'the status',
		project_id: 'the project',
		parent_id: 'the parent task',
		recurrence: 'the repeat',
		recurrence_mode: 'how it repeats',
		estimate_minutes: 'the estimate'
	};

	function describe(e: Schemas['ActivityEvent']): string {
		const f = e.fields as Record<string, unknown>;
		const replica = engine.replica(ws);
		switch (e.entity) {
			case 'task':
				if (e.kind === 'created')
					return e.entityId === taskId ? 'created this task' : 'added a subtask';
				if (e.kind === 'deleted') return 'deleted this task';
				if (e.kind === 'restored') return 'restored this task';
				if ('completed_at' in f) return f.completed_at ? 'completed this' : 'reopened this';
				if ('status_id' in f)
					return `moved this to ${replica?.status(f.status_id as string)?.name ?? 'another status'}`;
				if ('title' in f) return `renamed this to “${f.title}”`;
				if ('priority' in f) return `set the priority to ${f.priority}`;
				if ('recurrence' in f) {
					const rule = tryParseRule(f.recurrence as string | null);
					const mode = (f.recurrence_mode as RepeatMode | undefined) ?? 'schedule';
					return rule ? `set it to repeat: ${describeAfter(rule, mode)}` : 'stopped it repeating';
				}
				if ('due_date' in f)
					return f.due_date ? `set the due date to ${f.due_date}` : 'removed the due date';
				if ('estimate_minutes' in f)
					return typeof f.estimate_minutes === 'number'
						? `set the estimate to ${formatDuration(f.estimate_minutes)}`
						: 'removed the estimate';
				return `changed ${Object.keys(f)
					.map((k) => fieldNames[k] ?? k)
					.join(', ')}`;
			case 'comment':
				return e.kind === 'created'
					? 'commented'
					: e.kind === 'deleted'
						? 'deleted a comment'
						: 'edited a comment';
			case 'checklist_item':
				if (e.kind === 'created') return `added “${f.text}” to the checklist`;
				if ('done' in f)
					return f.done ? 'checked off a checklist item' : 'unchecked a checklist item';
				return 'changed the checklist';
			case 'task_label': {
				const name = replica?.labels().find((l) => l.id === f.label_id)?.name ?? 'a label';
				return f.deleted_at ? `removed the label ${name}` : `added the label ${name}`;
			}
			case 'task_assignee':
				return f.deleted_at ? 'removed an assignee' : 'assigned someone';
			case 'task_dependency': {
				const id = (f.blocker_id ??
					replica?.get('task_dependency', e.entityId)?.values.blocker_id) as string | undefined;
				const name = id ? replica?.task(id)?.title : undefined;
				const it = name ? `“${name}”` : 'another task';
				if (id && id !== taskId && e.entityId)
					return f.deleted_at ? `stopped waiting for ${it}` : `made this wait for ${it}`;
				return f.deleted_at ? 'removed a dependency' : 'added a dependency';
			}
			case 'attachment':
				return e.kind === 'deleted' ? 'removed a file' : `added the file ${f.filename ?? ''}`;
			case 'time_entry': {
				const spent = typeof f.minutes === 'number' ? formatDuration(f.minutes) : '';
				if (e.kind === 'deleted') return 'deleted time logged';
				if (e.kind === 'created') return f.running ? 'started a timer' : `logged ${spent}`;
				if (f.running === false) return spent ? `stopped a timer at ${spent}` : 'stopped a timer';
				return spent ? `changed time logged to ${spent}` : 'changed time logged';
			}
		}
		return 'made a change';
	}
</script>

{#if error}
	<p class="note">History needs a connection.</p>
{:else if events === null}
	<p class="note">Loading…</p>
{:else}
	<ol aria-label="History">
		{#each events as e (e.id)}
			<li>
				<strong>{e.actorName || 'Someone'}</strong>
				{describe(e)}{#if e.via}&nbsp;through {e.via}{/if} <span>{relative(e.at)}</span>
			</li>
		{/each}
	</ol>
{/if}

<style>
	ol {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	strong {
		color: var(--text-primary);
		font-weight: 600;
	}
	span {
		font-size: var(--text-xs);
	}
	.note {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
