<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { api, call, messageOf, type Role, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import TextInput from '#lib/components/TextInput.svelte';
	import { canDelete, canRename, canViewMembers } from '#lib/roles.js';
	import { session } from '#lib/session.svelte.js';
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';

	const ws = $derived(settingsWorkspace());
	const me = $derived(session.user);
	const role = $derived<Role>(ws?.role ?? 'guest');

	let members = $state<Schemas['Member'][]>([]);
	// The server keeps one owner in every workspace; don't offer what it refuses.
	const soleOwner = $derived(
		role === 'owner' && members.filter((m) => m.role === 'owner').length === 1
	);
	let name = $state('');
	let confirmDelete = $state(false);

	async function load() {
		members =
			ws && !ws.isPersonal && canViewMembers(role)
				? await call(
						api.GET('/workspaces/{workspaceId}/members', {
							params: { path: { workspaceId: ws.id } }
						})
					)
				: [];
	}

	$effect(() => {
		name = ws?.name ?? '';
		confirmDelete = false;
		load().catch((err) => toasts.error(messageOf(err)));
	});

	const wsPath = () => ({ params: { path: { workspaceId: ws!.id } } });

	async function rename(e: SubmitEvent) {
		e.preventDefault();
		try {
			await call(api.PATCH('/workspaces/{workspaceId}', { ...wsPath(), body: { name } }));
			toasts.show('Renamed');
			await session.loadWorkspaces();
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}

	/** Leaving or deleting: the workspace is gone from this person's list, so Settings starts over. */
	async function end(work: () => Promise<unknown>, done: string) {
		try {
			await work();
			toasts.show(done);
			await session.loadWorkspaces();
			await goto(resolve('/(app)/settings'), { replaceState: true });
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}

	function leave() {
		const gone = ws!.name;
		void end(
			() =>
				call(
					api.DELETE('/workspaces/{workspaceId}/members/{userId}', {
						params: { path: { workspaceId: ws!.id, userId: me!.id } }
					})
				),
			`You left ${gone}`
		);
	}

	function deleteWorkspace() {
		if (!confirmDelete) {
			confirmDelete = true;
			return;
		}
		const gone = ws!.name;
		void end(() => call(api.DELETE('/workspaces/{workspaceId}', wsPath())), `${gone} deleted`);
	}
</script>

<svelte:head><title>General · Mila</title></svelte:head>

{#if ws}
	<section class="half">
		<h2>{ws.name}</h2>
		{#if ws.isPersonal}
			<p class="hint">
				Your personal workspace is just for you. To work with other people, create a shared
				workspace from the workspace menu.
			</p>
		{/if}
		{#if canRename(role)}
			<form class="inline-form" onsubmit={rename}>
				<div class="grow"><TextInput label="Workspace name" required bind:value={name} /></div>
				<Button title="Rename this workspace" type="submit" disabled={name === ws.name}
					>Rename</Button
				>
			</form>
		{/if}
	</section>

	{#if !ws.isPersonal}
		<section class="half">
			<h2>Your membership</h2>
			<p class="hint">
				{role === 'guest'
					? 'You are a guest in this workspace.'
					: `You are ${role === 'owner' || role === 'admin' ? 'an' : 'a'} ${role} of this workspace.`}
				{#if soleOwner}
					You are its only owner, so make someone else an owner in People before you leave.
				{/if}
			</p>
			{#if !soleOwner}
				<div>
					<Button variant="ghost" onclick={leave}>Leave {ws.name}</Button>
				</div>
			{/if}
		</section>

		{#if canDelete(role)}
			<section>
				<h2>Delete workspace</h2>
				<p class="hint">Deleting removes the workspace for everyone in it.</p>
				<div>
					<Button variant="danger" onclick={deleteWorkspace}>
						{confirmDelete ? `Yes, delete ${ws.name}` : `Delete ${ws.name}`}
					</Button>
				</div>
			</section>
		{/if}
	{/if}
{/if}
