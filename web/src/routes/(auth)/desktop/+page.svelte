<script lang="ts">
	// Signing in to the desktop app: it opens this page in
	// the person's own browser, where passkeys work. Once they are signed in here and
	// say yes, the browser hands a one-time code to a port on this computer, where the
	// app is listening. It asks first, so that a link someone sends can't sign an app
	// in unnoticed, and the code is no use without a secret only the app holds.
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { api, ApiError, call, messageOf } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import { session } from '#lib/session.svelte.js';

	const params = page.url.searchParams;
	const port = Number(params.get('port'));
	const appState = params.get('state') ?? '';
	const challenge = params.get('challenge') ?? '';
	const valid =
		Number.isInteger(port) &&
		port >= 1024 &&
		port <= 65535 &&
		/^[A-Za-z0-9_-]{16,128}$/.test(appState) &&
		/^[A-Za-z0-9_-]{43}$/.test(challenge);

	let loaded = $state(false);
	let error = $state('');
	let done = $state(false);
	let busy = $state(false);
	let canceled = $state(false);
	const back = `${resolve('/(auth)/desktop')}${page.url.search}`;

	$effect(() => {
		session.load().then(() => {
			loaded = true;
			if (valid && !session.user) {
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() plus a query
				void goto(`${resolve('/(auth)/login')}?next=${encodeURIComponent(back)}`, {
					replaceState: true
				});
			}
		});
	});

	async function allow() {
		busy = true;
		error = '';
		try {
			const { code } = await call(api.POST('/auth/desktop/code', { body: { challenge } }));
			done = true;
			// Only ever this computer: the port is all the page takes from its address.
			// It says which server this is, so that the app can tell if it was sent here
			// by another (RFC 9207).
			location.href = `http://127.0.0.1:${port}/?code=${encodeURIComponent(code)}&state=${encodeURIComponent(appState)}&iss=${encodeURIComponent(location.origin)}`;
		} catch (err) {
			if (err instanceof ApiError && err.code === 'sign_in_again') {
				// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() plus a query
				await goto(`${resolve('/(auth)/login')}?again=1&next=${encodeURIComponent(back)}`);
				return;
			}
			error = messageOf(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Sign in to the Mila app</title></svelte:head>

{#if !valid}
	<h1>This link isn't complete</h1>
	<p>Start again from the Mila app on your computer.</p>
{:else if canceled}
	<h1>Canceled</h1>
	<p>The app wasn't signed in. You can close this tab.</p>
{:else if done}
	<h1>Signed in</h1>
	<p>You can close this tab and go back to the Mila app.</p>
{:else if loaded && session.user}
	<h1>Sign in to the Mila app?</h1>
	<p>
		The Mila app on this computer asked to sign in to {page.url.host} as {session.user.name} ({session
			.user.email}). If you didn't just ask for this in the app, or gave it another address, choose
		Cancel.
	</p>
	{#if error}<p class="error" role="alert">{error}</p>{/if}
	<div class="actions">
		<Button variant="primary" disabled={busy} onclick={allow}>Sign in to the app</Button>
		<Button variant="ghost" onclick={() => (canceled = true)}>Cancel</Button>
	</div>
{/if}

<style>
	h1 {
		font-size: var(--text-xl);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.error {
		color: var(--danger);
	}
</style>
