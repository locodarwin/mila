<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import { api, call, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import EmailAddressCard from '#lib/components/email/EmailAddressCard.svelte';
	import RecentMail from '#lib/components/email/RecentMail.svelte';
	import { showFailure } from '#lib/signInAgain.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { openTask } from '#lib/taskLink.js';
	import { toasts } from '#lib/toasts.svelte.js';

	let info = $state<Schemas['EmailToTask'] | null>(null);
	let busy = $state(false);
	const ws = $derived(settingsWorkspace()?.id ?? '');
	const projects = $derived(
		(engine.replica(ws)?.projects() ?? [])
			.filter((p) => !p.archivedAt)
			.map((p) => ({ id: p.id, name: p.name }))
	);

	async function load() {
		if (!ws) return;
		info = await call(
			api.GET('/workspaces/{workspaceId}/email-to-task', { params: { path: { workspaceId: ws } } })
		);
	}

	$effect(() => {
		void ws;
		load().catch((err) => showFailure(err));
	});

	async function act(what: () => Promise<unknown>, said?: string) {
		busy = true;
		try {
			await what();
			if (said) toasts.show(said);
		} catch (err) {
			showFailure(err);
		} finally {
			busy = false;
			await load().catch(() => {});
		}
	}

	const make = () =>
		act(() =>
			call(
				api.POST('/workspaces/{workspaceId}/email-to-task/addresses', {
					params: { path: { workspaceId: ws } },
					body: { assignToMe: true }
				})
			)
		);

	const path = (addressId: string) => ({ path: { workspaceId: ws, addressId } });
</script>

<svelte:head><title>Email to task · Mila</title></svelte:head>

<section>
	<h2>Email to task</h2>
	{#if info && !info.enabled}
		<p class="hint">
			This server has no mailbox set up to receive email. Whoever runs it can give it one, with any
			mail provider that offers IMAP.
		</p>
	{:else if info}
		<p class="hint">
			Mail sent to one of these addresses becomes a task in {settingsWorkspace()?.name}, made by
			you. The subject is the title, the message is the notes, and files come along. Treat an
			address like a password: anyone who has it and sends from an address listed here can add tasks
			as you.
		</p>
		<div class="cards">
			{#each info.addresses as a (a.id)}
				<EmailAddressCard
					address={a}
					{projects}
					own={info.signInAddress}
					onsave={(changes) =>
						act(() =>
							call(
								api.PATCH('/workspaces/{workspaceId}/email-to-task/addresses/{addressId}', {
									params: path(a.id),
									body: changes
								})
							)
						)}
					onreplace={() =>
						act(
							() =>
								call(
									api.POST(
										'/workspaces/{workspaceId}/email-to-task/addresses/{addressId}/replace',
										{
											params: path(a.id)
										}
									)
								),
							'Made a new address. Mail to the old one is now refused.'
						)}
					ondelete={() =>
						act(
							() =>
								call(
									api.DELETE('/workspaces/{workspaceId}/email-to-task/addresses/{addressId}', {
										params: path(a.id)
									})
								),
							'Deleted the address'
						)}
				/>
			{:else}
				<p class="hint">You have no addresses yet.</p>
			{/each}
		</div>
		<div>
			<Button
				variant="primary"
				title="Make a new address that turns email into tasks"
				disabled={busy}
				onclick={make}>New address</Button
			>
		</div>
	{/if}
</section>

{#if info?.enabled}
	<section>
		<h2>Recent mail</h2>
		<RecentMail items={info.recent} onopen={(id) => void openTask(id)} />
		<p class="hint">
			The last 30 days. Mila never replies to email, so this is where to look when a task doesn't
			appear. For mail it refuses, it keeps who it was from and the subject, not the message.
		</p>
	</section>
{/if}

<style>
	.cards {
		display: grid;
		gap: var(--space-3);
	}
</style>
