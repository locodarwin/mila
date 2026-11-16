<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { api, call, messageOf } from '#lib/api/client.js';
	import { session } from '#lib/session.svelte.js';

	let error = $state('');
	let email = $state('');

	// The token is spent by this request, not by opening the link, so mail
	// scanners that prefetch links cannot use it up.
	$effect(() => {
		const token = page.url.searchParams.get('token') ?? '';
		call(api.POST('/auth/email/verify', { body: { token } }))
			.then((user) => {
				email = user.email;
				// Opened where they are signed in: the app shows it at once.
				if (session.user?.id === user.id) session.setUser(user);
			})
			.catch((err) => (error = messageOf(err)));
	});
</script>

<svelte:head><title>Email address · Mila</title></svelte:head>

<div class="stack">
	{#if error}
		<h1>That link did not work</h1>
		<p class="error" role="alert">{error}</p>
		<a class="link" href={resolve('/(app)/settings')}>Back to Settings</a>
	{:else if email}
		<h1>Email address changed</h1>
		<p class="lead">Your Mila account's email address is now {email}.</p>
		<a class="link" href={resolve('/(app)')}>Open Mila</a>
	{:else}
		<h1>Changing your email address…</h1>
	{/if}
</div>

<style>
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
</style>
