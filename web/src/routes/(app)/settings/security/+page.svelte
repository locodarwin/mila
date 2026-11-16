<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { api, call, messageOf, type Schemas } from '#lib/api/client.js';
	import { browserSupportsWebAuthn, registerPasskey } from '#lib/api/passkeys.js';
	import Button from '#lib/components/Button.svelte';
	import SecurityLog from '#lib/components/SecurityLog.svelte';
	import { deviceName } from '#lib/device.js';
	import { session } from '#lib/session.svelte.js';
	import { forgetDevice, warnOfUnsent } from '#lib/signout.js';
	import { relative } from '#lib/time.js';
	import { toasts } from '#lib/toasts.svelte.js';
	import { showFailure } from '#lib/signInAgain.js';

	let passkeys = $state<Schemas['Passkey'][]>([]);
	let sessions = $state<Schemas['Session'][]>([]);
	let events = $state<Schemas['SecurityEvent'][]>([]);
	let more = $state(false);
	let older = $state(false);
	let confirming = $state<string | null>(null);
	let busy = $state(false);
	const canAdd = $derived(session.passkeys && browserSupportsWebAuthn());

	async function load() {
		let page: Schemas['SecurityEventPage'];
		[passkeys, sessions, page] = await Promise.all([
			call(api.GET('/me/passkeys')),
			call(api.GET('/me/sessions')),
			call(api.GET('/me/security-events', { params: { query: { limit: 20 } } }))
		]);
		events = page.events;
		more = page.more;
	}

	async function showOlder() {
		older = true;
		try {
			const page = await call(
				api.GET('/me/security-events', {
					params: { query: { limit: 20, before: events.at(-1)?.id } }
				})
			);
			events = [...events, ...page.events];
			more = page.more;
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			older = false;
		}
	}

	$effect(() => {
		load().catch((err) => toasts.error(messageOf(err)));
	});

	async function add() {
		busy = true;
		try {
			await registerPasskey(deviceName(navigator.userAgent));
			toasts.show('Passkey added');
			await load();
		} catch (err) {
			showFailure(err);
		} finally {
			busy = false;
		}
	}

	async function rename(id: string, current: string) {
		const name = prompt('Name this passkey', current)?.trim();
		if (!name || name === current) return;
		try {
			await call(
				api.PATCH('/me/passkeys/{passkeyId}', {
					params: { path: { passkeyId: id } },
					body: { name }
				})
			);
			await load();
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}

	// Removing a passkey cannot be undone, so it takes a second click.
	async function remove(id: string) {
		if (confirming !== id) {
			confirming = id;
			return;
		}
		confirming = null;
		try {
			await call(api.DELETE('/me/passkeys/{passkeyId}', { params: { path: { passkeyId: id } } }));
			toasts.show('Passkey removed');
			await load();
		} catch (err) {
			showFailure(err);
		}
	}

	async function revoke(s: Schemas['Session'], anyway = false) {
		try {
			if (s.current && !anyway && (await warnOfUnsent(() => void revoke(s, true)))) return;
			await call(api.DELETE('/me/sessions/{sessionId}', { params: { path: { sessionId: s.id } } }));
			if (s.current) {
				await forgetDevice();
				await goto(resolve('/(auth)/login'), { replaceState: true });
				return;
			}
			toasts.show(`Signed out of ${deviceName(s.userAgent)}`);
			await load();
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}
</script>

<svelte:head><title>Security · Mila</title></svelte:head>

<section class="half">
	<h2>Passkeys</h2>
	<p class="hint">
		Passkeys sign you in with your fingerprint, face or screen lock. Email links always work too, so
		removing a passkey never locks you out.
	</p>
	{#if passkeys.length}
		<ul class="rows" aria-label="Passkeys">
			{#each passkeys as key (key.id)}
				<li>
					<KeyRound size={16} />
					<span class="grow">
						{key.name}
						<span class="sub">
							Added {relative(key.createdAt)}{key.lastUsedAt
								? ` · last used ${relative(key.lastUsedAt)}`
								: ''}
						</span>
					</span>
					<Button
						title="Rename this passkey"
						size="sm"
						variant="ghost"
						onclick={() => rename(key.id, key.name)}>Rename</Button
					>
					<Button size="sm" variant="danger" onclick={() => remove(key.id)}>
						{confirming === key.id ? 'Confirm remove' : 'Remove'}
					</Button>
				</li>
			{/each}
		</ul>
	{/if}
	{#if canAdd}
		<div><Button onclick={add} disabled={busy}><KeyRound size={16} />Add a passkey</Button></div>
	{:else if !session.passkeys}
		<p class="hint">
			Passkeys are off because this Mila is reached by an IP address. Serve it from a domain name to
			use them.
		</p>
	{/if}
</section>

<section class="half">
	<h2>Signed-in devices</h2>
	<ul class="rows" aria-label="Signed-in devices">
		{#each sessions as s (s.id)}
			<li>
				<span class="grow">
					{deviceName(s.userAgent)}
					{#if s.current}<span class="badge">This device</span>{/if}
					<span class="sub"
						>Signed in {relative(s.createdAt)} · active {relative(s.lastSeenAt)}{s.ip
							? ` · ${s.ip}`
							: ''}</span
					>
				</span>
				<Button size="sm" variant={s.current ? 'secondary' : 'danger'} onclick={() => revoke(s)}>
					Sign out
				</Button>
			</li>
		{/each}
	</ul>
</section>

<section>
	<h2>Recent activity</h2>
	<p class="hint">
		Sign-ins and changes to how you sign in, for the past year. A sign-in from a browser you have
		not used before is also emailed to you. If something here was not you, sign that device out
		above.
	</p>
	<SecurityLog {events} {more} busy={older} onmore={showOlder} />
</section>
