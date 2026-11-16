<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import { api, call, messageOf, type Role, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import Avatar from '#lib/components/task/Avatar.svelte';
	import RoleSelect from '#lib/components/RoleSelect.svelte';
	import TextInput from '#lib/components/TextInput.svelte';
	import {
		assignableRoles,
		canInvite,
		canRemove,
		canViewMembers,
		invitableRoles
	} from '#lib/roles.js';
	import { session } from '#lib/session.svelte.js';
	import { relative } from '#lib/time.js';
	import { toasts } from '#lib/toasts.svelte.js';

	const ws = $derived(settingsWorkspace());
	const me = $derived(session.user);
	const role = $derived<Role>(ws?.role ?? 'guest');

	let members = $state<Schemas['Member'][]>([]);
	let invites = $state<Schemas['Invite'][]>([]);

	let inviteEmail = $state('');
	let inviteRole = $state<Role>('member');

	async function load() {
		if (!ws || ws.isPersonal) {
			members = [];
			invites = [];
			return;
		}
		const path = { params: { path: { workspaceId: ws.id } } };
		members = canViewMembers(role)
			? await call(api.GET('/workspaces/{workspaceId}/members', path))
			: [];
		invites =
			canInvite(role) && !session.local
				? await call(api.GET('/workspaces/{workspaceId}/invites', path))
				: [];
	}

	$effect(() => {
		load().catch((err) => toasts.error(messageOf(err)));
	});

	async function act(work: () => Promise<unknown>, done?: string) {
		try {
			await work();
			if (done) toasts.show(done);
			await session.loadWorkspaces();
			await load();
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}

	const wsPath = () => ({ params: { path: { workspaceId: ws!.id } } });

	function invite(e: SubmitEvent) {
		e.preventDefault();
		act(async () => {
			await call(
				api.POST('/workspaces/{workspaceId}/invites', {
					...wsPath(),
					body: { email: inviteEmail, role: inviteRole }
				})
			);
			inviteEmail = '';
		}, 'Invitation sent');
	}

	function cancelInvite(id: string) {
		act(
			() =>
				call(
					api.DELETE('/workspaces/{workspaceId}/invites/{inviteId}', {
						params: { path: { workspaceId: ws!.id, inviteId: id } }
					})
				),
			'Invitation canceled'
		);
	}

	function changeRole(userId: string, to: Role) {
		act(() =>
			call(
				api.PATCH('/workspaces/{workspaceId}/members/{userId}', {
					params: { path: { workspaceId: ws!.id, userId } },
					body: { role: to }
				})
			)
		);
	}

	function removeMember(userId: string) {
		act(
			() =>
				call(
					api.DELETE('/workspaces/{workspaceId}/members/{userId}', {
						params: { path: { workspaceId: ws!.id, userId } }
					})
				),
			'Member removed'
		);
	}
</script>

<svelte:head><title>People · Mila</title></svelte:head>

{#if ws}
	{#if ws.isPersonal}
		<section>
			<h2>People in {ws.name}</h2>
			<p class="hint">
				Your personal workspace is just for you. To work with other people, create a shared
				workspace from the workspace menu.
			</p>
		</section>
	{:else if !canViewMembers(role)}
		<section>
			<h2>People in {ws.name}</h2>
			<p class="hint">
				As a guest you see the people you share projects with, not the member list.
			</p>
		</section>
	{:else}
		<section>
			<h2>Members of {ws.name}</h2>
			<ul class="rows" aria-label="Members">
				{#each members as m (m.userId)}
					{@const options = assignableRoles(role, m.role)}
					<li>
						<Avatar name={m.name} src={m.avatar} size={28} />
						<span class="grow">
							{m.name}
							{#if m.userId === me?.id}<span class="badge">You</span>{/if}
							<span class="sub">{m.email} · joined {relative(m.joinedAt)}</span>
						</span>
						{#if options.length}
							<RoleSelect
								label={`Role for ${m.name}`}
								value={m.role}
								roles={options}
								onchange={(to) => changeRole(m.userId, to)}
							/>
						{:else}
							<span class="sub">{m.role}</span>
						{/if}
						{#if m.userId !== me?.id && canRemove(role, m.role)}
							<Button size="sm" variant="danger" onclick={() => removeMember(m.userId)}
								>Remove</Button
							>
						{/if}
					</li>
				{/each}
			</ul>
		</section>

		{#if canInvite(role) && !session.local}
			<section>
				<h2>Invite people</h2>
				<form class="inline-form" onsubmit={invite}>
					<div class="grow">
						<TextInput label="Email" type="email" required bind:value={inviteEmail} />
					</div>
					<RoleSelect
						label="Role for the invitation"
						bind:value={inviteRole}
						roles={invitableRoles(role)}
					/>
					<Button type="submit" variant="primary">Send invitation</Button>
				</form>
				{#if invites.length}
					<ul class="rows" aria-label="Open invitations">
						{#each invites as inv (inv.id)}
							<li>
								<span class="grow">
									{inv.email}
									<span class="sub">{inv.role} · expires {relative(inv.expiresAt)}</span>
								</span>
								<Button size="sm" variant="ghost" onclick={() => cancelInvite(inv.id)}
									>Cancel</Button
								>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		{/if}
	{/if}
{/if}
