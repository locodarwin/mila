<script lang="ts">
	// Notifications' parts, loaded apart from the style guide so it stays
	// inside its script budget: the bell, the panel, the settings matrix
	// and push on this device. Nothing here is sent.
	import NotificationBell from '#lib/components/NotificationBell.svelte';
	import NotificationMatrix from '#lib/components/NotificationMatrix.svelte';
	import NotificationPanel from '#lib/components/NotificationPanel.svelte';
	import PushDevice from '#lib/components/PushDevice.svelte';
	import type { Notification } from '#lib/notifications.svelte.js';
	import type { PushSupport } from '#lib/push.js';

	const ago = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString();
	let demoNotes = $state<Notification[]>([
		{
			id: 'n1',
			workspaceId: 'w',
			kind: 'mention',
			taskId: 't1',
			actorId: 'u-dana',
			actorName: 'Dana Whitfield',
			title: 'Dana Whitfield mentioned you in Repaint the porch',
			body: 'Can you pick up the primer on Saturday, @Shawn? The hardware store closes at noon.',
			createdAt: ago(4),
			url: '/?task=t1&ws=w'
		},
		{
			id: 'n2',
			workspaceId: 'w',
			kind: 'assigned',
			taskId: 't2',
			actorId: 'u-lee',
			actorName: 'Lee Park',
			title: 'Lee Park assigned you to Book the venue',
			body: 'In Spring fundraiser',
			createdAt: ago(95),
			url: '/?task=t2&ws=w'
		},
		{
			id: 'n3',
			workspaceId: 'w',
			kind: 'invite_accepted',
			actorId: 'u-sam',
			actorName: 'Sam Ortega',
			title: 'Sam Ortega joined Household',
			body: 'Sam Ortega (sam@example.com) accepted your invitation.',
			createdAt: ago(60 * 26),
			readAt: ago(60 * 20),
			url: '/?ws=w'
		}
	]);
	const demoUnread = $derived(demoNotes.filter((n) => !n.readAt).length);
	let demoPush = $state<PushSupport | 'checking'>('available');
	let demoPushOn = $state(false);
	let demoDevices = $state(1);
	let demoKinds = $state({
		mention: { app: true, push: true, email: true },
		assigned: { app: true, push: true, email: false },
		page_comment: { app: true, push: false, email: false },
		automation: { app: true, push: true, email: false },
		email_task: { app: true, push: false, email: false },
		unblocked: { app: true, push: true, email: false },
		comment: { app: true, push: false, email: false },
		due: { app: true, push: true, email: true },
		reminder: { app: true, push: true, email: true },
		invite_accepted: { app: true, push: false, email: true }
	});
</script>

<div class="wrap">
	<div class="bell-row">
		<NotificationBell unread={demoUnread} target="demo-notifications" />
	</div>
	<NotificationBell unread={demoUnread} target="demo-notifications" variant="icon" />
	<NotificationBell unread={0} target="demo-notifications" variant="icon" />
</div>
<div class="panel-demo">
	<NotificationPanel
		id="demo-notifications"
		inline
		items={demoNotes}
		unread={demoUnread}
		onopen={(n) => {
			const hit = demoNotes.find((x) => x.id === n.id);
			if (hit) hit.readAt = new Date().toISOString();
		}}
		onreadall={() => demoNotes.forEach((n) => (n.readAt ??= new Date().toISOString()))}
		onclear={(n) =>
			demoNotes.splice(
				demoNotes.findIndex((x) => x.id === n.id),
				1
			)}
		onclearall={() => demoNotes.splice(0)}
	/>
	<NotificationPanel id="demo-notifications-empty" inline items={[]} unread={0} />
</div>
<h3>Settings matrix</h3>
<NotificationMatrix
	kinds={demoKinds}
	push={true}
	onchange={(kind, channel, on) => (demoKinds[kind][channel] = on)}
/>
<h3>Push on this device</h3>
<p class="note">
	Explains plainly why push cannot be turned on in a browser. Pick what the browser allows:
</p>
<label class="pair">
	Browser
	<select bind:value={demoPush}>
		<option value="checking">Still checking</option>
		<option value="available">Can push</option>
		<option value="denied">Notifications blocked</option>
		<option value="insecure">Not a secure connection</option>
		<option value="unsupported">No push support</option>
	</select>
</label>
<PushDevice
	support={demoPush === 'checking' ? null : demoPush}
	bind:on={demoPushOn}
	devices={demoDevices}
	ontoggle={(on) => (demoDevices += on ? 1 : -1)}
	ontest={() => {}}
/>

<style>
	h3 {
		font-size: var(--text-md);
	}
	.bell-row {
		width: 240px;
		padding: var(--space-2);
		border-radius: var(--radius-md);
		background: var(--surface-1);
	}
	.panel-demo {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(280px, 380px));
		gap: var(--space-4);
		align-items: start;
	}
	.wrap {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3);
		align-items: center;
	}
	.pair {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.pair select {
		height: 32px;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		color: var(--text-primary);
	}
	.note {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
