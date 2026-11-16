<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { announce } from '#lib/announce.svelte.js';
	import { api, call, messageOf, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import TextInput from '#lib/components/TextInput.svelte';
	import { session } from '#lib/session.svelte.js';

	const token = page.url.searchParams.get('token') ?? '';
	let preview = $state<Schemas['InvitePreview'] | null>(null);
	let name = $state('');
	let error = $state('');
	let busy = $state(false);
	let joinedNeedsSignIn = $state(false);
	// Joining replaces the form; focus moves to what replaced it.
	const focusNow = (el: HTMLElement) => el.focus();

	// Whoever is signed in, against whom the invitation is for. A member's or
	// guest's invitation joins whoever opens it signed in; anything more
	// (an admin's) only the address it went to.
	const me = $derived(session.user);
	const sameAddress = $derived(
		!!me && !!preview && me.email.toLowerCase() === preview.email.toLowerCase()
	);
	const alreadyIn = $derived(
		!!me && !!preview && session.workspaces.some((w) => w.id === preview?.workspaceId)
	);
	const onlyThem = $derived(!!preview && preview.role !== 'member' && preview.role !== 'guest');

	$effect(() => {
		// The workspaces too, to know whether whoever is signed in is in this one.
		const signedIn = session
			.load()
			.then(() => (session.user ? session.loadWorkspaces().catch(() => {}) : undefined));
		Promise.all([signedIn, call(api.POST('/invites/preview', { body: { token } }))])
			.then(([, p]) => (preview = p))
			.catch((err) => (error = messageOf(err)));
	});

	async function accept(e?: SubmitEvent) {
		e?.preventDefault();
		error = '';
		busy = true;
		try {
			const result = await call(
				api.POST('/invites/accept', { body: { token, name: name || undefined } })
			);
			session.select(result.workspaceId);
			if (result.signedIn) await goto(resolve('/(app)'), { replaceState: true });
			else joinedNeedsSignIn = true;
		} catch (err) {
			error = messageOf(err);
		} finally {
			busy = false;
		}
	}

	async function open() {
		if (!preview) return;
		session.select(preview.workspaceId);
		await goto(resolve('/(app)'), { replaceState: true });
	}

	const changes = (n: number) => (n === 1 ? '1 change' : `${n} changes`);

	// Signing out here deletes what this device holds, as signing out does
	// anywhere. Edits not sent yet exist nowhere else, and Mila isn't running
	// on this page to send them, so with any waiting it sends the person to
	// Mila first rather than losing them.
	async function signOutToJoin() {
		if (!preview) return;
		error = '';
		busy = true;
		try {
			const [{ engine }, { unfinished }] = await Promise.all([
				import('#lib/sync/engine.svelte.js'),
				import('#lib/sync/journal.js')
			]);
			const n = (await engine.unsent()).length + unfinished().length;
			if (n > 0) {
				error = `This device has ${changes(n)} not sent yet, which signing out would delete. Open Mila so ${n === 1 ? 'it goes' : 'they go'}, then come back to this link.`;
				return;
			}
			const { signOutHere } = await import('#lib/signout.js');
			await signOutHere();
			announce(`Signed out. Join as ${preview.email}.`);
		} catch (err) {
			error = messageOf(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Invitation · Mila</title></svelte:head>

{#if joinedNeedsSignIn}
	<div class="stack">
		<h1 tabindex="-1" {@attach focusNow}>You're in</h1>
		<p class="lead">
			You joined {preview?.workspaceName}. Sign in as {preview?.email} to get started.
		</p>
		<a class="link" href={resolve('/(auth)/login')}>Sign in</a>
	</div>
{:else if preview}
	<h1>Join {preview.workspaceName}</h1>
	<p class="lead">
		{preview.invitedByName} invited {preview.email} to the workspace
		<strong>{preview.workspaceName}</strong> as {preview.role === 'admin' ? 'an' : 'a'}
		{preview.role}.
	</p>
	{#if me && alreadyIn}
		<div class="stack">
			{#if sameAddress}
				<p class="lead">You're in {preview.workspaceName} already.</p>
				<Button variant="primary" onclick={open}>Open {preview.workspaceName}</Button>
			{:else}
				<p class="lead">
					You're signed in as {me.name} ({me.email}), who is in {preview.workspaceName} already. To accept
					it as {preview.email}, sign out first.
				</p>
				<div class="row">
					<Button variant="primary" disabled={busy} onclick={signOutToJoin}>Sign out first</Button>
					<Button onclick={open}>Open {preview.workspaceName}</Button>
				</div>
			{/if}
		</div>
	{:else if me && !sameAddress}
		<div class="stack">
			<p class="lead">
				This invitation was sent to {preview.email}{onlyThem
					? `, to help run ${preview.workspaceName}, so only ${preview.email} can accept it`
					: ''}. You're signed in as {me.name} ({me.email}). To accept it as {preview.email}, sign
				out first{onlyThem ? '' : `, or join as ${me.name}`}.
			</p>
			<div class="row">
				<Button variant="primary" disabled={busy} onclick={signOutToJoin}>Sign out first</Button>
				{#if !onlyThem}
					<Button disabled={busy} onclick={() => accept()}>Join as {me.name}</Button>
				{/if}
			</div>
		</div>
	{:else}
		<form onsubmit={accept}>
			{#if me}
				<p class="lead">You will join as {me.name} ({me.email}).</p>
			{:else}
				<TextInput
					label="Your name"
					autocomplete="name"
					bind:value={name}
					placeholder="Only needed if you are new to Mila"
				/>
			{/if}
			<Button type="submit" variant="primary" disabled={busy}>Join {preview.workspaceName}</Button>
		</form>
	{/if}
	{#if error}<p class="error" role="alert">{error}</p>{/if}
{:else if error}
	<div class="stack">
		<h1>This invitation can't be used</h1>
		<p class="error" role="alert">{error}</p>
	</div>
{:else}
	<h1>Loading invitation…</h1>
{/if}

<style>
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
	/* Addresses are long words; they wrap rather than widen the card. */
	.lead {
		overflow-wrap: anywhere;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
</style>
