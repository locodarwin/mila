<script lang="ts">
	// The second half of signing in to the desktop app: the app's own window opens
	// this and hands over the code the browser gave it and the secret behind it. They
	// come from the app and never from the address, or a link holding someone else's
	// code would sign whoever follows it in to that person's account.
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { api, call, messageOf, type User } from '#lib/api/client.js';
	import { takeDesktopSignIn } from '#lib/desktop.js';
	import { session } from '#lib/session.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';

	let error = $state('');

	async function finish() {
		const handoff = await takeDesktopSignIn();
		if (!handoff) {
			error = 'This page is only for the Mila app, as it signs in. Start again from the app.';
			return;
		}
		const user = await call(api.POST('/auth/desktop/finish', { body: handoff }));
		session.signedIn(user);
		await goto(resolve('/(app)'), { replaceState: true });
		said(user);
	}

	/**
	 * Says whom the window signed in as, and where, until the person closes it.
	 * Someone else using this computer could hand the app their own sign-in while
	 * the person is still in their browser, and this is where they would notice.
	 */
	function said(user: User) {
		toasts.show(`Signed in to ${location.host} as ${user.name} (${user.email}).`, {
			sticky: true,
			closable: true,
			action: {
				label: 'Not you? Sign out',
				run: async () => {
					const { signOutHere } = await import('#lib/signout.js');
					await signOutHere();
					await goto(resolve('/(auth)/login'), { replaceState: true });
				}
			}
		});
	}

	$effect(() => {
		finish().catch((err) => (error = messageOf(err)));
	});
</script>

<svelte:head><title>Signing in · Mila</title></svelte:head>

{#if error}
	<h1>That didn't work</h1>
	<p role="alert">{error}</p>
{:else}
	<h1>Signing in…</h1>
{/if}

<style>
	h1 {
		font-size: var(--text-xl);
	}
</style>
