<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { api, call, messageOf } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import DeleteAccount from '#lib/components/data/DeleteAccount.svelte';
	import ProfilePicture from '#lib/components/ProfilePicture.svelte';
	import TextInput from '#lib/components/TextInput.svelte';
	import { session } from '#lib/session.svelte.js';
	import { forgetDevice } from '#lib/signout.js';
	import { toasts } from '#lib/toasts.svelte.js';
	import { showFailure } from '#lib/signInAgain.js';

	let name = $state(session.user?.name ?? '');
	let busy = $state(false);
	let email = $state('');
	let sending = $state(false);
	let sentTo = $state('');
	let deleting = $state(false);

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			session.setUser(await call(api.PATCH('/me', { body: { name } })));
			toasts.show('Name saved');
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			busy = false;
		}
	}

	async function changeEmail(e: SubmitEvent) {
		e.preventDefault();
		sending = true;
		try {
			const to = email.trim();
			await call(api.POST('/me/email', { body: { email: to } }));
			sentTo = to;
			email = '';
		} catch (err) {
			showFailure(err);
		} finally {
			sending = false;
		}
	}

	async function deleteAccount(typed: string) {
		deleting = true;
		try {
			await call(api.POST('/me/delete', { body: { email: typed } }));
		} catch (err) {
			showFailure(err);
			deleting = false;
			return;
		}
		await forgetDevice();
		await goto(resolve('/(auth)/login'), { replaceState: true });
	}
</script>

<svelte:head><title>Profile · Mila</title></svelte:head>

<section class="half">
	<h2>Profile</h2>
	<ProfilePicture />
	<form class="inline-form" onsubmit={save}>
		<div class="grow">
			<TextInput label="Name" autocomplete="name" required bind:value={name} />
		</div>
		<Button type="submit" disabled={busy || name === session.user?.name}>Save</Button>
	</form>
</section>

{#if !session.local}
	<section class="half">
		<h2>Email address</h2>
		<p class="hint">
			Signed in as {session.user?.email}. Sign-in links and email from Mila go here.
		</p>
		<form class="inline-form" onsubmit={changeEmail}>
			<div class="grow">
				<TextInput
					label="New address"
					type="email"
					autocomplete="email"
					required
					bind:value={email}
				/>
			</div>
			<Button type="submit" disabled={sending || !email.trim()}>Send link</Button>
		</form>
		<p class="hint" role="status">
			{#if sentTo}
				We sent a link to {sentTo}. Open it within 24 hours to make it your address; until then,
				nothing changes.
			{/if}
		</p>
	</section>
{/if}

{#if !session.local}
	<section>
		<h2>Delete your account</h2>
		{#if session.user}
			<DeleteAccount email={session.user.email} busy={deleting} ondelete={deleteAccount} />
		{/if}
	</section>
{/if}
