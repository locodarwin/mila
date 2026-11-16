<script lang="ts">
	import { api, call, messageOf, type Schemas } from '#lib/api/client.js';
	import AdminStatus from '#lib/components/admin/AdminStatus.svelte';
	import InviteList from '#lib/components/admin/InviteList.svelte';
	import UserList from '#lib/components/admin/UserList.svelte';
	import { session } from '#lib/session.svelte.js';
	import { toasts } from '#lib/toasts.svelte.js';

	let status = $state<Schemas['AdminStatus'] | null>(null);
	let users = $state<Schemas['AdminUser'][]>([]);
	let invites = $state<Schemas['AdminInvite'][]>([]);
	let backingUp = $state(false);
	const admin = $derived(!!session.user?.isInstanceAdmin);

	async function load() {
		[status, users, invites] = await Promise.all([
			call(api.GET('/admin/status')),
			call(api.GET('/admin/users')),
			call(api.GET('/admin/invites'))
		]);
	}

	$effect(() => {
		if (admin) load().catch((err) => toasts.error(messageOf(err)));
	});

	// A backup is made in the background; the status is read again until
	// its outcome changes.
	async function backUp() {
		backingUp = true;
		const outcome = (b?: Schemas['BackupStatus']) =>
			`${b?.lastOkAt}|${b?.lastErrorAt}|${b?.recent[0]?.name}`;
		const before = outcome(status?.backup);
		try {
			await call(api.POST('/admin/backups'));
			toasts.show('Backing up the database');
			for (let i = 0; i < 120; i++) {
				await new Promise((r) => setTimeout(r, 1000));
				status = await call(api.GET('/admin/status'));
				const b = status.backup;
				if (outcome(b) === before) continue;
				if (b?.lastErrorAt && (!b.lastOkAt || b.lastOkAt < b.lastErrorAt))
					toasts.error(`The backup failed: ${b.lastError}`);
				else toasts.show('Backed up');
				return;
			}
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			backingUp = false;
		}
	}

	async function setAdmin(u: Schemas['AdminUser'], on: boolean) {
		try {
			await call(
				api.PATCH('/admin/users/{userId}', {
					params: { path: { userId: u.id } },
					body: { isInstanceAdmin: on }
				})
			);
			toasts.show(on ? `${u.name} is now an admin` : `${u.name} is no longer an admin`);
			if (u.id === session.user?.id && !on) {
				await session.load();
				return;
			}
		} catch (err) {
			toasts.error(messageOf(err));
		}
		await load().catch((err) => toasts.error(messageOf(err)));
	}

	async function signOut(u: Schemas['AdminUser']) {
		try {
			const res = await call(
				api.POST('/admin/users/{userId}/sign-out', { params: { path: { userId: u.id } } })
			);
			toasts.show(
				`Signed ${u.name} out of ${res.sessions} ${res.sessions === 1 ? 'device' : 'devices'}`
			);
			if (u.id === session.user?.id) {
				location.reload();
				return;
			}
			await load();
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}

	async function removePicture(u: Schemas['AdminUser']) {
		try {
			const changed = await call(
				api.DELETE('/admin/users/{userId}/avatar', { params: { path: { userId: u.id } } })
			);
			users = users.map((x) => (x.id === changed.id ? changed : x));
			toasts.show(`Removed ${u.name}'s picture`);
			if (u.id === session.user?.id) void session.refreshPeople();
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}

	async function cancelInvite(inv: Schemas['AdminInvite']) {
		try {
			await call(
				api.DELETE('/admin/invites/{inviteId}', { params: { path: { inviteId: inv.id } } })
			);
			toasts.show(`Canceled the invitation to ${inv.email}`);
			await load();
		} catch (err) {
			toasts.error(messageOf(err));
		}
	}
</script>

<svelte:head><title>Admin · Mila</title></svelte:head>

{#if !admin}
	<section>
		<h2>Admin</h2>
		<p class="hint">Only an admin of this Mila can see this page.</p>
	</section>
{:else}
	<section>
		<h2>This Mila</h2>
		{#if status}
			<AdminStatus {status} busy={backingUp} onbackup={backUp} />
			{#if status.emailIn?.microsoft}
				<!-- Only for a Microsoft mailbox, which Mila signs in to from here. -->
				{#await import('#lib/components/admin/MicrosoftMailbox.svelte') then { default: MicrosoftMailbox }}
					<MicrosoftMailbox
						mailbox={status.emailIn.microsoft}
						refresh={async () => {
							status = await call(api.GET('/admin/status'));
						}}
					/>
				{/await}
			{/if}
		{:else}
			<p class="hint">Loading…</p>
		{/if}
	</section>

	<section>
		<h2>Accounts</h2>
		<p class="hint">
			Admins run this Mila: this page, its accounts and its backups. Signing someone out everywhere
			ends every session of theirs; each device clears its copy when it next connects. Remove a
			picture that does not belong here; its owner's initials show instead. Someone locked out of
			every admin account can run <code>mila user make-admin</code> at the server.
		</p>
		<UserList
			{users}
			me={session.user?.id}
			onadmin={setAdmin}
			onsignout={signOut}
			onremovepicture={removePicture}
		/>
	</section>

	<section>
		<h2>Open invitations</h2>
		<p class="hint">Every invitation not yet accepted, in every workspace.</p>
		<InviteList {invites} oncancel={cancelInvite} />
	</section>
{/if}
