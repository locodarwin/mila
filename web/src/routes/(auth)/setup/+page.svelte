<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { api, call, messageOf } from '#lib/api/client.js';
	import { browserSupportsWebAuthn, registerPasskey } from '#lib/api/passkeys.js';
	import Button from '#lib/components/Button.svelte';
	import TextInput from '#lib/components/TextInput.svelte';
	import { deviceName } from '#lib/device.js';
	import { session } from '#lib/session.svelte.js';

	const token = page.url.searchParams.get('token') ?? '';
	let name = $state('');
	let email = $state('');
	let error = $state('');
	let busy = $state(false);
	let step = $state<'account' | 'passkey'>('account');
	// The next step replaces the form; focus moves to its heading.
	const focusNow = (el: HTMLElement) => el.focus();

	async function create(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			session.signedIn(await call(api.POST('/auth/setup', { body: { token, name, email } })));
			await session.load();
			if (session.passkeys && browserSupportsWebAuthn()) step = 'passkey';
			else await goto(resolve('/(app)'), { replaceState: true });
		} catch (err) {
			error = messageOf(err);
		} finally {
			busy = false;
		}
	}

	async function addPasskey() {
		error = '';
		busy = true;
		try {
			await registerPasskey(deviceName(navigator.userAgent));
			await goto(resolve('/(app)'), { replaceState: true });
		} catch (err) {
			error = messageOf(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Set up · Mila</title></svelte:head>

{#if !token}
	<div class="stack">
		<h1>Set up Mila</h1>
		<p class="lead">
			Open the one-time setup link printed in the server's log when it started. Restarting Mila
			prints a fresh one.
		</p>
	</div>
{:else if step === 'account'}
	<h1>Create your account</h1>
	<p class="lead">You will be the owner of this Mila.</p>
	<form onsubmit={create}>
		<TextInput label="Name" autocomplete="name" required bind:value={name} />
		<TextInput label="Email" type="email" autocomplete="email" required bind:value={email} />
		<Button type="submit" variant="primary" disabled={busy}>Create account</Button>
	</form>
	{#if error}<p class="error" role="alert">{error}</p>{/if}
{:else}
	<div class="stack">
		<h1 tabindex="-1" {@attach focusNow}>Add a passkey</h1>
		<p class="lead">
			A passkey lets this device sign you in with your fingerprint, face or screen lock. No password
			to remember.
		</p>
		<Button variant="primary" onclick={addPasskey} disabled={busy}>
			<KeyRound size={16} />Add a passkey
		</Button>
		<Button
			title="Add a passkey later, in Settings"
			variant="ghost"
			onclick={() => goto(resolve('/(app)'), { replaceState: true })}
		>
			Skip for now
		</Button>
		{#if error}<p class="error" role="alert">{error}</p>{/if}
	</div>
{/if}
