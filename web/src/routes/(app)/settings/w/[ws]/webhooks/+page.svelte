<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import { tick } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { resolve } from '$app/paths';
	import { api, call, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import ShownOnce from '#lib/components/api/ShownOnce.svelte';
	import WebhookItem from '#lib/components/api/WebhookItem.svelte';
	import { canManageWebhooks } from '#lib/roles.js';
	import { showFailure } from '#lib/signInAgain.js';
	import { engine } from '#lib/sync/engine.svelte.js';

	type Webhook = Schemas['Webhook'];
	type Family = Schemas['WebhookFamily'];

	const ws = $derived(settingsWorkspace()?.id ?? '');
	const w = $derived(settingsWorkspace());
	const allowed = $derived(w ? canManageWebhooks(w.role) : false);
	const projects = $derived(engine.replica(ws)?.projects() ?? []);
	const projectName = (id: string) =>
		projects.find((p) => p.id === id)?.name ?? 'A project no longer here';

	const FAMILIES: { id: Family; label: string; what: string }[] = [
		{ id: 'task', label: 'Tasks', what: 'made, changed, completed, reopened, deleted' },
		{ id: 'comment', label: 'Comments', what: 'added, edited, deleted' },
		{ id: 'project', label: 'Projects', what: 'made, changed, archived, deleted' },
		{ id: 'page', label: 'Wiki pages', what: 'made, renamed, deleted' },
		{ id: 'time', label: 'Time', what: 'logged, changed, deleted' }
	];

	let hooks = $state<Webhook[]>([]);
	const logs = new SvelteMap<string, Schemas['WebhookDelivery'][]>();
	let secret = $state<{ title: string; value: string } | null>(null);
	let busy = $state(false);
	let shown = $state<ReturnType<typeof ShownOnce>>();

	let url = $state('');
	let events = $state<Family[]>(['task']);
	let project = $state('');

	const base = () => ({ path: { workspaceId: ws } });
	const one = (webhookId: string) => ({ path: { workspaceId: ws, webhookId } });

	async function load() {
		if (!ws || !allowed) return;
		hooks = await call(api.GET('/workspaces/{workspaceId}/webhooks', { params: base() }));
	}

	$effect(() => {
		logs.clear();
		load().catch((err) => showFailure(err));
	});

	async function loadLog(id: string) {
		logs.set(
			id,
			await call(
				api.GET('/workspaces/{workspaceId}/webhooks/{webhookId}/deliveries', { params: one(id) })
			)
		);
	}

	/** Runs an action on a webhook, then shows how things stand. */
	async function act(id: string, action: () => Promise<unknown>) {
		try {
			await action();
			await load();
			if (logs.has(id)) await loadLog(id);
		} catch (err) {
			showFailure(err);
		}
	}

	async function showSecret(title: string, value: string) {
		secret = { title, value };
		await tick();
		shown?.focus();
	}

	async function create(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			const made = await call(
				api.POST('/workspaces/{workspaceId}/webhooks', {
					params: base(),
					body: { url, events, ...(project ? { projectId: project } : {}) }
				})
			);
			url = '';
			await showSecret('Its signing secret', made.secret);
			await load();
		} catch (err) {
			showFailure(err);
		} finally {
			busy = false;
		}
	}

	function toggle(id: Family, on: boolean) {
		events = on ? [...events, id] : events.filter((f) => f !== id);
	}
</script>

<svelte:head><title>Webhooks · Mila</title></svelte:head>

{#if !allowed}
	<section>
		<h2>Webhooks</h2>
		<p class="hint">Only owners and admins of a workspace can manage its webhooks.</p>
	</section>
{:else}
	<section>
		<h2>Webhooks</h2>
		<p class="hint">
			{settingsWorkspace()?.name} tells these addresses when something changes, so another app can act
			on it. Help has
			<a class="link" href={resolve('/(app)/help/[topic]', { topic: 'api' })}
				>what each delivery holds and how to check its signature</a
			>.
		</p>
		<form class="new" onsubmit={create}>
			<label class="field">
				<span>Address</span>
				<input
					bind:value={url}
					type="url"
					required
					placeholder="https://"
					spellcheck="false"
					autocomplete="off"
				/>
			</label>
			<fieldset class="field">
				<legend>What it hears about</legend>
				{#each FAMILIES as f (f.id)}
					<label>
						<input
							type="checkbox"
							checked={events.includes(f.id)}
							onchange={(e) => toggle(f.id, e.currentTarget.checked)}
						/>
						<span>{f.label}: <span class="what">{f.what}</span></span>
					</label>
				{/each}
			</fieldset>
			<label class="field">
				<span>From</span>
				<select bind:value={project}>
					<option value="">Every project</option>
					{#each projects as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
				</select>
			</label>
			<div>
				<Button type="submit" variant="primary" disabled={busy || !url.trim() || !events.length}
					>Add webhook</Button
				>
			</div>
		</form>
		{#if secret}
			<ShownOnce
				bind:this={shown}
				title={secret.title}
				note="The app receiving these uses it to check they really came from Mila. This is the only time Mila shows it, so copy it now."
				value={secret.value}
				ondone={() => (secret = null)}
			/>
		{/if}
	</section>

	<section>
		<h2>This workspace's webhooks</h2>
		{#if hooks.length}
			<div class="list">
				{#each hooks as hook (hook.id)}
					<WebhookItem
						{hook}
						{projectName}
						deliveries={logs.get(hook.id) ?? null}
						onopen={() => void loadLog(hook.id).catch((err) => showFailure(err))}
						ontest={() =>
							act(hook.id, () =>
								call(
									api.POST('/workspaces/{workspaceId}/webhooks/{webhookId}/test', {
										params: one(hook.id)
									})
								)
							)}
						onpause={(paused) =>
							act(hook.id, () =>
								call(
									api.PATCH('/workspaces/{workspaceId}/webhooks/{webhookId}', {
										params: one(hook.id),
										body: { paused }
									})
								)
							)}
						onrotate={() =>
							act(hook.id, async () => {
								const made = await call(
									api.POST('/workspaces/{workspaceId}/webhooks/{webhookId}/secret', {
										params: one(hook.id)
									})
								);
								await showSecret(`The new signing secret for ${hook.url}`, made.secret);
							})}
						ondelete={() =>
							act(hook.id, async () => {
								await call(
									api.DELETE('/workspaces/{workspaceId}/webhooks/{webhookId}', {
										params: one(hook.id)
									})
								);
								logs.delete(hook.id);
							})}
						onresend={(d) =>
							act(hook.id, () =>
								call(
									api.POST(
										'/workspaces/{workspaceId}/webhooks/{webhookId}/deliveries/{deliveryId}/resend',
										{ params: { path: { workspaceId: ws, webhookId: hook.id, deliveryId: d.id } } }
									)
								)
							)}
					/>
				{/each}
			</div>
		{:else}
			<p class="hint">This workspace has no webhooks.</p>
		{/if}
		<p class="hint">
			Each delivery is tried up to six times over about a day. A webhook stops after 3 days of
			nothing but failures, and whoever made it is told.
		</p>
	</section>
{/if}

<style>
	.new {
		display: grid;
		gap: var(--space-3);
		max-width: 36rem;
	}
	.list {
		display: grid;
		gap: var(--space-2);
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
	.what {
		color: var(--text-secondary);
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
