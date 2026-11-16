<script lang="ts">
	import { session } from '../session.svelte';
	import type { Schemas } from '../api/client';
	import Checkbox from './Checkbox.svelte';

	type Kinds = Schemas['NotificationPreferences']['kinds'];
	type Kind = keyof Kinds;
	type Channel = keyof Schemas['NotificationChannels'];

	interface Props {
		kinds: Kinds;
		/** Whether this server can send web pushes. */
		push: boolean;
		onchange: (kind: Kind, channel: Channel, on: boolean) => void;
	}

	let { kinds, push, onchange }: Props = $props();

	const LOCAL_LEAVES: Kind[] = [
		'mention',
		'assigned',
		'comment',
		'page_comment',
		'invite_accepted',
		'email_task',
		'automation'
	];
	const allRows: { kind: Kind; name: string; about: string }[] = [
		{ kind: 'mention', name: 'Mentions', about: 'Someone mentions you in a comment' },
		{ kind: 'assigned', name: 'Assigned to you', about: 'Someone gives you a task' },
		{ kind: 'comment', name: 'Comments', about: 'On tasks you made or are assigned to' },
		{
			kind: 'page_comment',
			name: 'Comments on pages',
			about: 'In threads on wiki pages you wrote in'
		},
		{
			kind: 'unblocked',
			name: 'Ready to start',
			about: 'A task of yours was waiting for one that is now done'
		},
		{ kind: 'due', name: 'Due tasks', about: 'Your tasks reach their due time' },
		{ kind: 'reminder', name: 'Reminders', about: 'Reminders you set on tasks' },
		{
			kind: 'email_task',
			name: 'Tasks from email',
			about: 'Mail you sent to one of your addresses became a task'
		},
		{
			kind: 'automation',
			name: 'Automations',
			about: 'An automation tells you something, or one of yours stops'
		},
		{
			kind: 'invite_accepted',
			name: 'Accepted invites',
			about: 'Someone joins through your invite'
		}
	];
	const rows = session.local ? allRows.filter((r) => !LOCAL_LEAVES.includes(r.kind)) : allRows;
	// On this computer nothing leaves it, and there is nobody to be told about.
	const channels: { channel: Channel; name: string }[] = session.local
		? [{ channel: 'app', name: 'In the app' }]
		: [
				{ channel: 'app', name: 'In the app' },
				{ channel: 'push', name: 'Push' },
				{ channel: 'email', name: 'Email' }
			];
	// The in-app list is the record of these, so they always show there.
	const alwaysInApp: Kind[] = ['mention', 'reminder'];

	function why(kind: Kind, channel: Channel, name: string): string | undefined {
		if (channel === 'app' && alwaysInApp.includes(kind)) return `${name} always show in the app.`;
		if (channel === 'push' && !push) return 'This server cannot send push notifications.';
		return undefined;
	}
</script>

<table>
	<thead>
		<tr>
			<th scope="col">Notification</th>
			{#each channels as c (c.channel)}<th scope="col">{c.name}</th>{/each}
		</tr>
	</thead>
	<tbody>
		{#each rows as r (r.kind)}
			<tr>
				<th scope="row">{r.name}<span class="about">{r.about}</span></th>
				{#each channels as c (c.channel)}
					{@const reason = why(r.kind, c.channel, r.name)}
					<td>
						<Checkbox
							label="{r.name}: {c.name.toLowerCase()}"
							hideLabel
							checked={kinds[r.kind][c.channel] && !(c.channel === 'push' && !push)}
							disabled={reason !== undefined}
							title={reason}
							onchange={(on) => onchange(r.kind, c.channel, on)}
						/>
					</td>
				{/each}
			</tr>
		{/each}
	</tbody>
</table>

<style>
	table {
		width: 100%;
		border-collapse: collapse;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		font-size: var(--text-sm);
	}
	th,
	td {
		padding: var(--space-2) var(--space-3);
		text-align: center;
		vertical-align: middle;
	}
	thead th {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	thead th:first-child,
	tbody th {
		text-align: left;
	}
	tbody th {
		font-weight: 550;
	}
	tbody tr {
		border-top: var(--border-width) solid var(--border);
	}
	.about {
		display: block;
		font-size: var(--text-xs);
		font-weight: 400;
		color: var(--text-secondary);
	}
	td :global(.checkbox) {
		justify-content: center;
	}
	@media (max-width: 720px) {
		th,
		td {
			padding: var(--space-2);
		}
	}
</style>
