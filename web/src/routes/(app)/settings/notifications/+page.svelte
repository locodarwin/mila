<script lang="ts">
	import { formatTime } from '#lib/dates.js';
	import { api, call, messageOf, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import Checkbox from '#lib/components/Checkbox.svelte';
	import NotificationMatrix from '#lib/components/NotificationMatrix.svelte';
	import HelpLink from '#lib/components/help/HelpLink.svelte';
	import PushDevice from '#lib/components/PushDevice.svelte';
	import {
		currentSupport,
		deviceSubscription,
		disablePush,
		enablePush,
		type PushSupport
	} from '#lib/push.js';
	import { session } from '#lib/session.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';

	type Settings = Schemas['NotificationSettings'];

	let settings = $state<Settings | null>(null);
	let status = $state('');
	let sending = $state(false);
	const detected = (() => {
		try {
			return Intl.DateTimeFormat().resolvedOptions().timeZone;
		} catch {
			return '';
		}
	})();

	// The app sends this device's zone at start, which may land after the
	// settings load; the signed-in user has it as soon as it does.
	const zone = $derived(session.user?.timezone ?? settings?.timezone ?? '');

	$effect(() => {
		call(api.GET('/me/settings/notifications'))
			.then((s) => (settings = s))
			.catch((err) => toasts.error(messageOf(err)));
	});

	// Saves run one after another, each sending the preferences as they
	// stand then, so quick clicks never overwrite each other.
	let queue: Promise<unknown> = Promise.resolve();
	// Saves not yet done: the status waits for the last of them, so it never
	// speaks for a change still on its way. Each sends everything, so the
	// last one's outcome is the one that counts.
	let unsaved = 0;
	function save() {
		status = 'Saving…';
		unsaved++;
		queue = queue.then(async () => {
			let ok = false;
			try {
				if (!settings) return;
				const saved = await call(
					api.PUT('/me/settings/notifications', {
						body: { kinds: settings.kinds, digest: settings.digest, weekly: settings.weekly }
					})
				);
				settings.timezone = saved.timezone;
				ok = true;
			} catch (err) {
				toasts.error(messageOf(err));
			} finally {
				if (--unsaved === 0) status = ok ? 'Saved' : '';
			}
		});
	}

	// A server from before the weekly report sends none.
	const weekly = $derived(settings?.weekly ?? { enabled: false });

	const hours = $derived(
		Array.from({ length: 24 }, (_, h) => ({ value: h, label: formatTime(`${h}:00`) }))
	);

	// Push on this device: what the browser allows, whether it is on here,
	// and on how many of the person's devices.
	let support = $state<PushSupport | null>(null);
	let deviceOn = $state(false);
	let devices = $state(0);
	let pushBusy = $state(false);
	let pushTesting = $state(false);

	$effect(() => {
		void loadPush();
	});

	async function loadPush() {
		const s = currentSupport();
		deviceOn = s === 'available' && (await deviceSubscription().catch(() => null)) !== null;
		support = s;
		try {
			devices = (await call(api.GET('/me/push'))).devices;
		} catch {
			// Offline: the count stays as it was.
		}
	}

	async function togglePush(on: boolean) {
		pushBusy = true;
		try {
			if (on) {
				if (!session.user) return;
				await enablePush(session.user.id);
				toasts.show('Push is on for this device');
			} else {
				await disablePush();
				toasts.show('Push is off for this device');
			}
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			await loadPush();
			pushBusy = false;
		}
	}

	const deviceCount = (n: number) => (n === 1 ? '1 device' : `${n} devices`);

	async function testPush() {
		pushTesting = true;
		try {
			const r = await call(api.POST('/me/push/test'));
			if (r.sent) toasts.show(`Test push sent to ${deviceCount(r.sent)}`);
			if (r.failed)
				toasts.error(
					`The push service for ${deviceCount(r.failed)} could not be reached. Try again later.`
				);
			if (r.removed)
				toasts.error(
					`${deviceCount(r.removed)} had stopped taking pushes and ${r.removed === 1 ? 'was' : 'were'} removed. Turn push on there again.`
				);
			if (!r.sent && !r.failed && !r.removed) toasts.error('No device has push turned on.');
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			await loadPush();
			pushTesting = false;
		}
	}

	async function testEmail() {
		sending = true;
		try {
			await call(api.POST('/me/settings/notifications/test-email'));
			toasts.show('Test email sent');
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			sending = false;
		}
	}
</script>

<svelte:head><title>Notifications · Mila</title></svelte:head>

{#if settings}
	<section>
		<h2>What reaches you</h2>
		<p class="hint">
			Choose where each kind of notification goes. You are never told about your own changes.
		</p>
		<p>
			<HelpLink topic="reminders-and-notifications">How reminders and notifications work</HelpLink>
		</p>
		<NotificationMatrix
			kinds={settings.kinds}
			push={settings.push}
			onchange={(kind, channel, on) => {
				settings!.kinds[kind][channel] = on;
				save();
			}}
		/>
		<p class="hint">
			Mentions and reminders always show in the app, where the list is their record.
		</p>
		{#if !settings.push}
			<p class="hint">This server cannot send push notifications.</p>
		{/if}
		<p class="status" role="status">{status}</p>
	</section>

	{#if !session.local}
		<section class="half">
			<h2>Summaries by email</h2>
			<div class="summary">
				<Checkbox
					label="Email me a daily digest"
					checked={settings.digest.enabled}
					onchange={(on) => {
						settings!.digest.enabled = on;
						save();
					}}
				/>
				<p class="hint">
					One email a day with your overdue tasks, the ones due that day, and what you have not
					read. Nothing to say means no email.
				</p>
			</div>
			<div class="summary">
				<Checkbox
					label="Email me a weekly report"
					checked={weekly.enabled}
					onchange={(on) => {
						settings!.weekly = { enabled: on };
						save();
					}}
				/>
				<p class="hint">
					On the first day of your week, an email for each workspace about the week before: what was
					done and made, what is overdue, each project, the time logged, and what needs you. A week
					with nothing done, made or logged means no email.
				</p>
			</div>
			<label class="hour">
				<span>Send them at</span>
				<select
					value={settings.digest.hour}
					disabled={!settings.digest.enabled && !weekly.enabled}
					onchange={(e) => {
						settings!.digest.hour = Number(e.currentTarget.value);
						save();
					}}
				>
					{#each hours as h (h.value)}<option value={h.value}>{h.label}</option>{/each}
				</select>
			</label>
		</section>
	{/if}

	<section class="half">
		<h2>Time zone</h2>
		<p>
			Due times and the hour of your summaries are in <strong>{zone}</strong
			>{#if detected && detected === zone}, detected on this device{/if}.
		</p>
		{#if detected && detected !== zone}
			<p class="hint">
				This device is set to {detected}. Mila switches to it the next time the app starts with a
				connection.
			</p>
		{/if}
	</section>

	{#if settings.push && !session.local}
		<section class="half" aria-labelledby="push-heading">
			<h2 id="push-heading">Push</h2>
			<p class="hint">
				Push notifications reach a phone or computer even when Mila is not open. Turn them on in
				each browser you want them in.
			</p>
			<PushDevice
				{support}
				bind:on={deviceOn}
				{devices}
				busy={pushBusy}
				testing={pushTesting}
				ontoggle={togglePush}
				ontest={testPush}
			/>
		</section>
	{/if}

	{#if !session.local}
		<section class="half">
			<h2>Email</h2>
			{#if !settings.email}
				<p class="hint">
					Email is not set up on this server, so emails go to the server's log instead of an inbox.
				</p>
			{/if}
			<div>
				<Button onclick={testEmail} disabled={sending}>Send a test email</Button>
			</div>
		</section>
	{/if}
{:else}
	<p class="hint">Loading…</p>
{/if}

<style>
	.status {
		min-height: 1.5em;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.summary {
		display: grid;
		gap: var(--space-1);
	}
	.summary .hint {
		margin: 0;
	}
	.hour {
		display: flex;
		gap: var(--space-2);
		align-items: center;
		font-size: var(--text-sm);
	}
	select {
		height: 32px;
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	select:disabled {
		opacity: 0.6;
	}
</style>
