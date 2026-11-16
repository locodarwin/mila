<script lang="ts">
	import DatabaseBackup from '@lucide/svelte/icons/database-backup';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import type { Schemas } from '../../api/client';
	import { formatClock, formatDate, formatTime } from '../../dates';
	import { relative } from '../../time';
	import Button from '../Button.svelte';
	import { fileSize } from '../data/format';

	interface Props {
		status: Schemas['AdminStatus'];
		/** A backup was just asked for. */
		busy?: boolean;
		onbackup?: () => void;
	}

	let { status, busy = false, onbackup }: Props = $props();

	const EMAIL: Record<Schemas['AdminStatus']['emailMode'], string> = {
		smtp: 'Sent by SMTP',
		file: 'Written to a folder (for tests)',
		log: 'Printed in the server log: set MILA_SMTP_URL to send it'
	};
	const KINDS: Record<Schemas['BackupKind'], string> = {
		nightly: 'Nightly',
		manual: 'Asked for',
		premigrate: 'Before an upgrade',
		prerestore: 'Before a restore'
	};

	function when(at: string): string {
		const d = new Date(at);
		return `${formatDate(d, { weekday: 'short' })}, ${formatClock(d)}`;
	}

	const backup = $derived(status.backup);
	// The last thing that happened was a failure: the newest backup is
	// older than the newest error.
	const failing = $derived(
		!!backup?.lastErrorAt && (!backup.lastOkAt || backup.lastOkAt < backup.lastErrorAt)
	);
	const behind = $derived(status.schema !== status.latestSchema);

	// What this server still lacks, and the setting that fixes it. On the
	// server, mila config check tests the ones that are set.
	const loopback = (host: string) =>
		host === 'localhost' || host === '[::1]' || /^127\./.test(host);
	const host = $derived.by(() => {
		try {
			return new URL(status.baseUrl).hostname;
		} catch {
			return '';
		}
	});
	const missing = $derived(
		[
			status.emailMode === 'log' && {
				what: 'Email',
				why: 'Sign-in links, invitations and reminders are only printed in the server log. MILA_SMTP_URL sends them.'
			},
			status.baseUrl.startsWith('http://') &&
				!loopback(host) && {
					what: 'HTTPS',
					why: 'Mila is reached over plain HTTP, so sign-in links and sessions cross the network unencrypted. Put it behind a proxy with HTTPS, or set MILA_TLS_DOMAIN for its own certificate.'
				},
			!status.passkeys && {
				what: 'A domain',
				why: 'Mila is reached by an IP address, so passkeys cannot work. Give it a domain or subdomain in MILA_BASE_URL.'
			},
			status.emailIn?.microsoft &&
				status.emailIn.microsoft.state !== 'connected' &&
				status.emailIn.microsoft.state !== 'signing-in' && {
					what: 'Email to task',
					why:
						status.emailIn.microsoft.state === 'lost'
							? "Microsoft stopped accepting Mila's sign-in to the mailbox. Connect it again below."
							: 'The mailbox is with Microsoft, and Mila is not signed in to it yet. Connect it below.'
				},
			status.backup &&
				!status.backup.nightly && {
					what: 'Backups',
					why: 'Nightly backups are off. MILA_BACKUP=on turns them on.'
				}
		].filter((x): x is { what: string; why: string } => !!x)
	);
</script>

{#if missing.length}
	<section class="missing" aria-labelledby="missing-heading">
		<h3 id="missing-heading">Not set up yet</h3>
		<ul>
			{#each missing as m (m.what)}
				<li><strong>{m.what}.</strong> {m.why}</li>
			{/each}
		</ul>
		<p class="sub">
			After a change, restart Mila. On the server, <code>mila config check</code> tries the email server,
			the mailbox and the address without sending anything.
		</p>
	</section>
{/if}

<dl class="facts">
	<dt>Version</dt>
	<dd>
		{status.version}
		<!-- A build from a commit with no release tag is named by the commit. -->
		{#if status.commit && status.commit !== status.version}<span class="sub">({status.commit})</span
			>{/if}
		{#if status.update}
			<span class="update">
				Mila {status.update.version.replace(/^v/, '')} is out, from {formatDate(
					new Date(status.update.publishedAt)
				)}.
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a link out of the app -->
				<a href={status.update.url} target="_blank" rel="noopener noreferrer">What changed</a>
			</span>
		{/if}
	</dd>
	<dt>Database</dt>
	<dd>
		{status.dialect === 'postgres' ? 'PostgreSQL' : 'SQLite'}, schema {status.schema}
		{#if behind}of {status.latestSchema}{/if}
		{#if status.migratedAt}<span class="sub">Last upgraded {relative(status.migratedAt)}</span>{/if}
	</dd>
	<dt>Email</dt>
	<dd>{EMAIL[status.emailMode]}</dd>
	{#if status.emailIn}
		<dt>Email to task</dt>
		<dd>
			{#if status.emailIn.lastError}
				Not working: {status.emailIn.lastError}
			{:else if status.emailIn.lastCheck}
				Mailbox last checked {relative(status.emailIn.lastCheck)}
			{:else}
				Not checked yet
			{/if}
		</dd>
	{/if}
	<dt>Passkeys</dt>
	<dd>
		{status.passkeys ? 'On' : 'Off: Mila is reached by an IP address, so only email links work'}
	</dd>
	<dt>Address</dt>
	<dd class="url">{status.baseUrl}</dd>
	<dt>Accounts</dt>
	<dd>{status.users}</dd>
</dl>

<section class="backups" aria-labelledby="backups-heading">
	<h3 id="backups-heading">Backups</h3>
	{#if !backup}
		<p class="sub">Backups are not available on this server.</p>
	{:else}
		{#if failing}
			<p class="failed" role="status">
				<TriangleAlert size={16} aria-hidden="true" />
				<span>
					The last backup failed {relative(backup.lastErrorAt ?? '')}: {backup.lastError}
				</span>
			</p>
		{/if}
		<p>
			{#if backup.lastOkAt}
				Last backed up {relative(backup.lastOkAt)} <span class="sub">{when(backup.lastOkAt)}</span>
			{:else}
				No backup has been made yet.
			{/if}
		</p>
		<p class="sub">
			{#if backup.nightly}
				Nightly from {formatTime(`${backup.hour}:00`)} UTC, kept 7 days, 4 weeks and 6 months.
			{:else}
				Nightly backups are off (MILA_BACKUP).
			{/if}
			They go to <span class="url">{backup.dir}</span> on the server; attachments need their own copy.
		</p>
		<div>
			<Button onclick={() => onbackup?.()} disabled={busy}>
				<DatabaseBackup size={16} />{busy ? 'Backing up…' : 'Back up now'}
			</Button>
		</div>
		{#if backup.recent.length}
			<ul class="list" aria-label="Backups kept">
				{#each backup.recent.slice(0, 10) as b (b.name)}
					<li>
						<span class="grow">
							{when(b.at)}
							<span class="sub">{KINDS[b.kind]} · {fileSize(b.size)} · schema {b.schema}</span>
						</span>
					</li>
				{/each}
			</ul>
			{#if backup.recent.length > 10}
				<p class="sub">
					And {backup.recent.length - 10} older; <code>mila backup list</code> shows them all.
				</p>
			{/if}
		{/if}
	{/if}
</section>

<style>
	.update {
		display: block;
		font-size: var(--text-sm);
	}
	.update a {
		color: var(--accent-text);
		font-weight: 550;
	}
	.missing {
		display: grid;
		gap: var(--space-2);
		margin-bottom: var(--space-4);
		padding: var(--space-3) var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-1);
	}
	.missing h3 {
		margin: 0;
		font-size: var(--text-sm);
	}
	.missing ul {
		display: grid;
		gap: var(--space-1);
		margin: 0;
		padding-left: var(--space-5);
		font-size: var(--text-sm);
	}
	.missing p {
		margin: 0;
	}
	.facts {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: var(--space-2) var(--space-4);
		margin: 0;
		font-size: var(--text-sm);
	}
	dt {
		color: var(--text-secondary);
		font-weight: 600;
	}
	dd {
		margin: 0;
		min-width: 0;
	}
	.url {
		overflow-wrap: anywhere;
	}
	.sub {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	dd .sub {
		display: block;
	}
	.backups {
		display: grid;
		gap: var(--space-3);
		font-size: var(--text-sm);
	}
	h3 {
		font-size: var(--text-md);
	}
	.failed {
		display: flex;
		gap: var(--space-2);
		align-items: flex-start;
		padding: var(--space-2) var(--space-3);
		border: var(--border-width) solid var(--danger);
		border-radius: var(--radius-md);
	}
	.failed :global(svg) {
		flex: none;
		margin-top: 2px;
		color: var(--danger);
	}
	.list {
		margin: 0;
		padding: 0;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	.list li {
		display: flex;
		align-items: center;
		min-height: var(--row-height);
		padding: var(--pad-y) var(--space-4);
	}
	.list li + li {
		border-top: 1px solid var(--border);
	}
	.grow {
		flex: 1;
	}
	.list li .sub {
		display: block;
	}
	@media (max-width: 480px) {
		.facts {
			grid-template-columns: 1fr;
			gap: 0 var(--space-4);
		}
		dd + dt {
			margin-top: var(--space-2);
		}
	}
</style>
