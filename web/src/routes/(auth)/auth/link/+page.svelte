<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { api, call, messageOf } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import { takeNext } from '#lib/next.js';
	import { session } from '#lib/session.svelte.js';
	import { deviceOwner, setOwner, tookAskedLink } from '#lib/owner.js';
	import { local } from '#lib/sync/db.js';

	const token = page.url.searchParams.get('token') ?? '';
	let error = $state('');
	// Set when the link is for another account than the one signed in here,
	// with how many of that account's changes this device has yet to send.
	let switching = $state<{ from: string; to: string; unsent: number } | null>(null);
	// Set when this browser did not ask for a link: someone may have sent
	// it to sign the person in as themselves, or a mail scanner opened it.
	let confirming = $state<string | null>(null);

	async function signIn() {
		const was = session.user?.id;
		switching = null;
		confirming = null;
		try {
			// A copy here from before the device knew whose it was is the
			// account signed in now's.
			if (was && !deviceOwner()) setOwner(was);
			const user = await call(api.POST('/auth/magic-link/verify', { body: { token } }));
			if (was && was !== user.id) {
				// Someone else's: the app starts again from nothing, and deletes
				// their copy of everything on this device before it does.
				location.replace(takeNext());
				return;
			}
			session.signedIn(user);
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- takeNext gives a checked same-site path
			await goto(takeNext(), { replaceState: true });
		} catch (err) {
			error = messageOf(err);
		}
	}

	// The token is spent by a request, not by opening the link, so mail
	// scanners that prefetch links cannot use it up. A browser signed in to
	// another account asks first: a link someone sends must not quietly
	// sign a person in as them.
	$effect(() => {
		session
			.load()
			.then(async () => {
				const signedIn = session.user?.email;
				const asked = tookAskedLink();
				if (!signedIn && !asked) {
					const { email } = await call(api.POST('/auth/magic-link/peek', { body: { token } }));
					confirming = email;
					return;
				}
				if (signedIn) {
					const { email } = await call(api.POST('/auth/magic-link/peek', { body: { token } }));
					if (email !== signedIn) {
						const unsent = await local.outbox.count().catch(() => 0);
						switching = { from: signedIn, to: email, unsent };
						return;
					}
				}
				await signIn();
			})
			.catch((err) => (error = messageOf(err)));
	});
</script>

<svelte:head><title>Signing in · Mila</title></svelte:head>

<div class="stack">
	{#if error}
		<h1>That link did not work</h1>
		<p class="error" role="alert">{error}</p>
		<a class="link" href={resolve('/(auth)/login')}>Back to sign in</a>
	{:else if switching}
		<h1>Switch accounts?</h1>
		<p class="lead">
			This link signs in to <strong>{switching.to}</strong>. This browser is signed in to
			<strong>{switching.from}</strong>.
		</p>
		<p>
			Switching deletes what this device holds for {switching.from}{switching.unsent
				? `, ${switching.unsent === 1 ? '1 change' : `${switching.unsent} changes`} not sent yet included`
				: ''}, as signing out does.
		</p>
		<Button variant="primary" onclick={signIn}>Sign in to {switching.to}</Button>
		<a class="link" href={resolve('/(app)')}>Stay signed in to {switching.from}</a>
	{:else if confirming}
		<h1>Sign in to Mila?</h1>
		<p class="lead">This link signs in to <strong>{confirming}</strong>.</p>
		<Button variant="primary" onclick={signIn}>Sign in to {confirming}</Button>
	{:else}
		<h1>Signing you in…</h1>
	{/if}
</div>

<style>
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
</style>
