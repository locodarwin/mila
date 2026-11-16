<script lang="ts">
	import { tick } from 'svelte';
	import { resolve } from '$app/paths';
	import { api, call, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import ShownOnce from '#lib/components/api/ShownOnce.svelte';
	import TokenList from '#lib/components/api/TokenList.svelte';
	import { session } from '#lib/session.svelte.js';
	import { showFailure } from '#lib/signInAgain.js';
	import { toasts } from '#lib/toasts.svelte.js';

	let tokens = $state<Schemas['ApiToken'][]>([]);
	let secret = $state<{ name: string; value: string } | null>(null);
	let busy = $state(false);
	let shown = $state<ReturnType<typeof ShownOnce>>();
	let nameField = $state<HTMLInputElement>();

	let name = $state('');
	let access = $state<'read' | 'write'>('read');
	let where = $derived(session.workspace?.id ?? '');
	let days = $state('90');

	const workspaceName = (id: string) =>
		session.workspaces.find((w) => w.id === id)?.name ?? 'A workspace you left';

	async function load() {
		tokens = await call(api.GET('/me/tokens'));
	}

	$effect(() => {
		load().catch((err) => showFailure(err));
	});

	async function create(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			const made = await call(
				api.POST('/me/tokens', {
					body: {
						name,
						canWrite: access === 'write',
						...(where === 'all' ? {} : { workspaceId: where }),
						...(days === 'never' ? {} : { expiresInDays: Number(days) })
					}
				})
			);
			secret = { name: made.token.name, value: made.secret };
			name = '';
			await tick();
			shown?.focus();
			await load();
		} catch (err) {
			showFailure(err);
		} finally {
			busy = false;
		}
	}

	async function revoke(t: Schemas['ApiToken']) {
		try {
			await call(api.DELETE('/me/tokens/{tokenId}', { params: { path: { tokenId: t.id } } }));
			toasts.show(`Revoked ${t.name}`);
			await load();
		} catch (err) {
			showFailure(err);
		}
	}
</script>

<svelte:head><title>API tokens · Mila</title></svelte:head>

<section>
	<h2>API tokens</h2>
	<p class="hint">
		A token lets a script or another app use Mila as you. It can read and change your work, and can
		never do more than you can in the workspaces it reaches. It cannot sign in, manage your account
		or other tokens, or use admin powers. The
		<a class="link" href={resolve('/api/docs')}>API reference</a> lists what it can do.
	</p>
	<form class="new" onsubmit={create}>
		<label class="field">
			<span>Name</span>
			<input
				bind:this={nameField}
				bind:value={name}
				maxlength="80"
				required
				placeholder="What will use it"
			/>
		</label>
		<fieldset class="field">
			<legend>What it can do</legend>
			<label><input type="radio" bind:group={access} value="read" /> Read only</label>
			<label><input type="radio" bind:group={access} value="write" /> Read and write</label>
		</fieldset>
		<label class="field">
			<span>Where</span>
			<select bind:value={where}>
				{#each session.workspaces as w (w.id)}<option value={w.id}>{w.name}</option>{/each}
				<option value="all">All my workspaces, now and later</option>
			</select>
		</label>
		<label class="field">
			<span>Expires</span>
			<select bind:value={days}>
				<option value="30">In 30 days</option>
				<option value="90">In 90 days</option>
				<option value="365">In a year</option>
				<option value="never">Never</option>
			</select>
		</label>
		<div>
			<Button type="submit" variant="primary" disabled={busy || !name.trim()}>Make a token</Button>
		</div>
	</form>
	{#if secret}
		<ShownOnce
			bind:this={shown}
			title={`Your new token, ${secret.name}`}
			note="This is the only time Mila shows it, so copy it now. Mila keeps only a fingerprint of it, so nobody can read it back. Send it as Authorization: Bearer, followed by the token."
			value={secret.value}
			ondone={() => {
				secret = null;
				nameField?.focus();
			}}
		/>
	{/if}
</section>

<section>
	<h2>Your tokens</h2>
	<TokenList {tokens} {workspaceName} onrevoke={revoke} />
	<p class="hint">
		Each change a token makes is yours in a task's history, marked with the token's name. Making a
		token asks for a recent sign-in, and it goes in your security log, with an email to you. An app
		you connected by OAuth has a token here too, named after it, which reaches only the agents'
		tools.
	</p>
</section>

<style>
	.new {
		display: grid;
		gap: var(--space-3);
		max-width: 32rem;
	}
	.field {
		display: grid;
		gap: var(--space-1);
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
	.field > input,
	select {
		width: 100%;
		height: calc(34px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
</style>
