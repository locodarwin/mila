<script lang="ts">
	// Connecting an app by OAuth: an app such
	// as a hosted assistant sends the person here to say whether it may use Mila's
	// tools as them. The server checks the app and where it will send them before
	// anything shows, and never sends anyone to an address the app didn't register.
	// The app's name is its own words, so the page says where the answer goes too.
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { api, ApiError, call, messageOf, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import { session } from '#lib/session.svelte.js';

	const params = page.url.searchParams;
	const clientId = params.get('client_id') ?? '';
	const redirectUri = params.get('redirect_uri') ?? '';
	const valid =
		params.get('response_type') === 'code' &&
		clientId !== '' &&
		redirectUri !== '' &&
		(params.get('code_challenge') ?? '') !== '';

	let asked = $state<Schemas['OAuthRequest'] | null>(null);
	let refused = $state('');
	let error = $state('');
	let busy = $state(false);
	let sent = $state<'yes' | 'no' | null>(null);

	let workspace = $state('');
	let access = $state<'read' | 'write'>('read');
	let days = $state<30 | 90 | 365>(90);
	const back = `${resolve('/(auth)/oauth/authorize')}${page.url.search}`;

	$effect(() => {
		if (!valid) return;
		void start();
	});

	async function start() {
		await session.load();
		if (!session.user) {
			// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() plus a query
			await goto(`${resolve('/(auth)/login')}?next=${encodeURIComponent(back)}`, {
				replaceState: true
			});
			return;
		}
		try {
			const query = { client_id: clientId, redirect_uri: redirectUri };
			const resource = params.get('resource');
			const [got] = await Promise.all([
				call(
					api.GET('/oauth/consent', {
						params: { query: resource ? { ...query, resource } : query }
					})
				),
				session.loadWorkspaces()
			]);
			// The workspace the app's address named, by ID or name, else the one in use.
			const named = got.workspace?.toLowerCase();
			workspace =
				session.workspaces.find((w) => w.id === got.workspace || w.name.toLowerCase() === named)
					?.id ??
				session.workspace?.id ??
				'';
			asked = got;
		} catch (err) {
			refused = messageOf(err);
		}
	}

	async function answer(allow: boolean) {
		busy = true;
		error = '';
		try {
			const { redirect } = await call(
				api.POST('/oauth/consent', {
					body: {
						clientId,
						redirectUri,
						state: params.get('state') ?? undefined,
						codeChallenge: params.get('code_challenge') ?? '',
						codeChallengeMethod: params.get('code_challenge_method') ?? 'plain',
						resource: params.get('resource') ?? undefined,
						allow,
						workspaceId: workspace,
						canWrite: access === 'write',
						expiresInDays: days
					}
				})
			);
			sent = allow ? 'yes' : 'no';
			// The server built this from the address the app registered, and only that.
			location.href = redirect;
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

<svelte:head><title>Connect an app to Mila</title></svelte:head>

{#if !valid}
	<h1>This link isn't complete</h1>
	<p>Start connecting again from the app.</p>
{:else if refused}
	<h1>This app can't connect</h1>
	<p class="error" role="alert">{refused}</p>
{:else if sent === 'yes'}
	<h1>Connected</h1>
	<p>Sending you back to {asked?.returnsTo}. You can close this tab if it stays open.</p>
{:else if sent === 'no'}
	<h1>Not connected</h1>
	<p>The app was told no. You can close this tab if it stays open.</p>
{:else if asked && session.user}
	<h1>Connect {asked.app} to Mila?</h1>
	<p>
		An app calling itself {asked.app} asked to use Mila's tools as {session.user.name} ({session
			.user.email}), in one workspace. It chose that name itself, so connect it only if you just
		started connecting it.
	</p>
	<p>Either way, you go back to <strong>{asked.returnsTo}</strong>.</p>
	<form
		onsubmit={(e) => {
			e.preventDefault();
			void answer(true);
		}}
	>
		<label class="field">
			<span>Workspace</span>
			<select bind:value={workspace} required>
				{#each session.workspaces as w (w.id)}<option value={w.id}>{w.name}</option>{/each}
			</select>
		</label>
		<fieldset class="field">
			<legend>What it can do</legend>
			<label><input type="radio" bind:group={access} value="read" /> Read only</label>
			<label><input type="radio" bind:group={access} value="write" /> Read and write</label>
		</fieldset>
		<label class="field">
			<span>For</span>
			<select bind:value={days}>
				<option value={30}>30 days</option>
				<option value={90}>90 days</option>
				<option value={365}>A year</option>
			</select>
		</label>
		<p class="lead">
			It acts as you, never with more than you can do, and only through the agents' tools. It shows
			in Settings, API tokens, where you can revoke it.
		</p>
		{#if error}<p class="error" role="alert">{error}</p>{/if}
		<div class="actions">
			<Button type="submit" variant="primary" disabled={busy || !workspace}>Connect</Button>
			<Button variant="ghost" disabled={busy} onclick={() => answer(false)}>Don't connect</Button>
		</div>
	</form>
{/if}

<style>
	.field {
		display: grid;
		gap: var(--space-1);
		margin: 0;
		padding: 0;
		border: none;
		font-size: var(--text-sm);
	}
	.field > span,
	legend {
		padding: 0;
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	.field > label {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: max(var(--tap-min), 28px);
	}
	select {
		width: 100%;
		height: max(var(--tap-min), calc(34px * var(--control-scale)));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
</style>
