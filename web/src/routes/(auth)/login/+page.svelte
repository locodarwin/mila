<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { api, call, messageOf } from '#lib/api/client.js';
	import { browserSupportsWebAuthn, signInWithPasskey } from '#lib/api/passkeys.js';
	import Button from '#lib/components/Button.svelte';
	import TextInput from '#lib/components/TextInput.svelte';
	import { rememberNext, safeNext } from '#lib/next.js';
	import { askedForLink } from '#lib/owner.js';
	import { session } from '#lib/session.svelte.js';

	const next = safeNext(page.url.searchParams.get('next'));
	// Sent here to sign in again (src/lib/signInAgain.ts): a signed-in
	// visitor stays to do it, rather than going straight on.
	const again = page.url.searchParams.get('again') === '1';
	let email = $state('');
	let sentTo = $state('');
	let error = $state('');
	let busy = $state(false);
	let canPasskey = $state(false);
	// The form is replaced once the link is sent; focus moves to what
	// replaced it, which a screen reader then reads.
	const focusNow = (el: HTMLElement) => el.focus();

	$effect(() => {
		session.load().then(() => {
			canPasskey = session.passkeys && browserSupportsWebAuthn();
			if (session.user && again) email ||= session.user.email;
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- next is a checked same-site path
			else if (session.user) goto(next, { replaceState: true });
		});
	});

	async function withPasskey() {
		error = '';
		busy = true;
		try {
			session.signedIn(await signInWithPasskey());
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- next is a checked same-site path
			await goto(next, { replaceState: true });
		} catch (err) {
			error = messageOf(err);
		} finally {
			busy = false;
		}
	}

	async function sendLink(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			// First: the link can arrive, and be opened, before the answer.
			if (next !== '/') rememberNext(next);
			askedForLink();
			await call(api.POST('/auth/magic-link', { body: { email } }));
			sentTo = email;
		} catch (err) {
			error = messageOf(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Sign in · Mila</title></svelte:head>

{#if sentTo}
	<div class="stack">
		<h1 tabindex="-1" {@attach focusNow}>Check your email</h1>
		<p class="lead">
			If <strong>{sentTo}</strong> has an account, a sign-in link is on its way. It works once and expires
			in 15 minutes.
		</p>
		<Button variant="ghost" onclick={() => (sentTo = '')}>Use a different address</Button>
	</div>
{:else}
	{#if again}
		<h1>Sign in again</h1>
		<p class="lead">For your account's safety, sign in once more to go on.</p>
	{:else}
		<h1>Sign in</h1>
	{/if}
	{#if canPasskey}
		<Button variant="primary" onclick={withPasskey} disabled={busy}>
			<KeyRound size={16} />Sign in with a passkey
		</Button>
		<div class="divider">or</div>
	{/if}
	<form onsubmit={sendLink}>
		<TextInput label="Email" type="email" autocomplete="email" required bind:value={email} />
		<Button type="submit" variant={canPasskey ? 'secondary' : 'primary'} disabled={busy}>
			Email me a sign-in link
		</Button>
	</form>
	{#if error}<p class="error" role="alert">{error}</p>{/if}
{/if}
