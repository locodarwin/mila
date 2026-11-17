<script lang="ts">
	// Settings, Admin: Mila's sign-in to a Microsoft mailbox for email to task,
	// which takes no password. Connect asks the server for a code, which an
	// admin enters at Microsoft; the page reads the status again until the
	// sign-in is made, refused or runs out.
	import { untrack } from 'svelte';
	import { announce } from '../../announce.svelte';
	import { api, call, messageOf, type Schemas } from '../../api/client';
	import { copyText } from '../../clipboard';
	import { formatClock, formatDate } from '../../dates';
	import { relative } from '../../time';
	import { toasts } from '../../toasts.svelte';
	import Button from '../Button.svelte';

	interface Props {
		mailbox: Schemas['MicrosoftMailbox'];
		/** Reads the admin status again. */
		refresh: () => Promise<void>;
	}

	let { mailbox, refresh }: Props = $props();

	let busy = $state(false);
	let confirming = $state(false);

	const STATE: Record<Schemas['MicrosoftMailbox']['state'], { label: string; tone: string }> = {
		'not-connected': { label: 'Not connected', tone: 'quiet' },
		'signing-in': { label: 'Waiting for you at Microsoft', tone: 'quiet' },
		connected: { label: 'Connected', tone: 'ok' },
		lost: { label: 'Needs signing in again', tone: 'bad' }
	};
	const badge = $derived(STATE[mailbox.state]);
	const day = (at: string) => formatDate(new Date(at));
	/** What a sign-in reaches, said before one is made. */
	const reach =
		"Mila can read and delete mail in every mailbox the account you sign in as can open, so for a shared mailbox use an account that opens nothing else. The sign-in is sealed with a key in Mila's data directory: a backup of the database alone can't use it, but a copy of the whole directory can.";
	/** The code as Microsoft's page shows it: in two halves. */
	const code = $derived(
		mailbox.userCode && mailbox.userCode.length === 8
			? `${mailbox.userCode.slice(0, 4)}-${mailbox.userCode.slice(4)}`
			: (mailbox.userCode ?? '')
	);

	// While a code waits to be entered, the status is read every two seconds.
	$effect(() => {
		if (mailbox.state !== 'signing-in') return;
		const timer = setInterval(() => void untrack(refresh).catch(() => {}), 2000);
		return () => clearInterval(timer);
	});
	// Said aloud when it changes, since nothing moves focus.
	let said = '';
	$effect(() => {
		const now = mailbox.state;
		if (said && said !== now) {
			if (now === 'connected') announce(`Connected to ${mailbox.mailbox}`);
			else if (now === 'not-connected' && mailbox.problem) announce(mailbox.problem);
		}
		said = now;
	});

	async function act(fn: () => Promise<unknown>) {
		busy = true;
		try {
			await fn();
			await refresh();
		} catch (err) {
			toasts.error(messageOf(err));
		} finally {
			busy = false;
		}
	}

	const connect = () =>
		act(async () => {
			await call(api.POST('/admin/mailbox/microsoft'));
			announce('A code to enter at Microsoft is showing');
		});
	const cancel = () => act(() => call(api.POST('/admin/mailbox/microsoft/cancel')));
	const disconnect = () =>
		act(async () => {
			await call(api.DELETE('/admin/mailbox/microsoft'));
			confirming = false;
			toasts.show(`Disconnected from ${mailbox.mailbox}. Email to task is paused.`);
		});
	async function copy() {
		if (await copyText(mailbox.userCode ?? '')) toasts.show('Copied the code');
	}
</script>

<section class="mailbox" aria-labelledby="mailbox-heading">
	<div class="head">
		<h3 id="mailbox-heading">Mailbox for email to task</h3>
		<span class={`pill ${badge.tone}`}>{badge.label}</span>
	</div>

	{#if mailbox.state === 'signing-in'}
		<p>
			On any device, open <strong>{mailbox.verificationUri ?? 'microsoft.com/devicelogin'}</strong>,
			enter this code, and sign in as {mailbox.mailbox} (or someone with full access to it).
		</p>
		<p class="sub">{reach}</p>
		<code class="code tabular" aria-label={`The code: ${[...(mailbox.userCode ?? '')].join(' ')}`}>
			{code}
		</code>
		{#if mailbox.expiresAt}
			<p class="sub">
				The code works until {formatClock(new Date(mailbox.expiresAt))}. This page carries on by
				itself once you've signed in.
			</p>
		{/if}
		<div class="row">
			<Button onclick={copy}>Copy the code</Button>
			<Button disabled={busy} onclick={cancel}>Cancel</Button>
		</div>
	{:else}
		{#if mailbox.state === 'connected'}
			<dl class="facts">
				<dt>Mailbox</dt>
				<dd>
					{mailbox.mailbox}{#if mailbox.account && mailbox.account.toLowerCase() !== mailbox.mailbox.toLowerCase()},
						signed in as {mailbox.account}{/if}
				</dd>
				{#if mailbox.connectedAt}
					<dt>Connected</dt>
					<dd>
						{day(mailbox.connectedAt)}{#if mailbox.connectedBy}, by {mailbox.connectedBy}{/if}
					</dd>
				{/if}
			</dl>
			{#if mailbox.account && mailbox.account.toLowerCase() !== mailbox.mailbox.toLowerCase()}
				<p>
					Mila can read and delete mail in every mailbox {mailbox.account} can open, not only {mailbox.mailbox}.
					An account with access to this mailbox alone is safer: connect again as that one.
				</p>
			{/if}
		{:else if mailbox.state === 'lost' && mailbox.keyMissing}
			<p>
				Mila can't open its sign-in to {mailbox.mailbox}: the key it was sealed with, secret.key in
				its data directory, is missing, another, or not readable by Mila, as when the database is
				restored on another server or the file's owner changes. Email to task is paused. Put back
				the secret.key it was made with, readable by Mila, and email to task goes on at its next
				check; or connect again. Mail waits in the mailbox and is read once you do.
			</p>
			{#if mailbox.lostWhy}<p class="sub">{mailbox.lostWhy}</p>{/if}
		{:else if mailbox.state === 'lost'}
			<p>
				Microsoft stopped accepting Mila's sign-in to {mailbox.mailbox}{mailbox.lostAt
					? ` ${relative(mailbox.lostAt)}`
					: ''}, so email to task is paused. This happens when the account's password changes or an
				admin revokes it. Mail waits in the mailbox and is read once you connect again.
			</p>
		{:else}
			<p>
				This server's mailbox, {mailbox.mailbox}, is with Microsoft, which needs you to sign Mila in
				to it. Sign in as the mailbox's own account, or as someone with full access to a shared
				mailbox.
			</p>
			<p class="sub">{reach}</p>
		{/if}
		{#if mailbox.problem}<p class="problem" role="alert">{mailbox.problem}</p>{/if}
		{#if confirming}
			<div class="confirm" role="group" aria-label="Disconnect the mailbox">
				<p>Disconnect {mailbox.mailbox}? Email to task stops until someone connects it again.</p>
				<div class="row">
					<Button variant="danger" disabled={busy} onclick={disconnect}>Disconnect</Button>
					<Button onclick={() => (confirming = false)}>Keep it</Button>
				</div>
			</div>
		{:else}
			<div class="row">
				{#if mailbox.state !== 'connected'}
					<Button variant="primary" disabled={busy} onclick={connect}
						>{mailbox.state === 'lost' ? 'Connect again' : 'Connect a Microsoft mailbox'}</Button
					>
				{/if}
				{#if mailbox.state !== 'not-connected'}
					<Button disabled={busy} onclick={() => (confirming = true)}>Disconnect</Button>
				{/if}
			</div>
		{/if}
	{/if}
</section>

<style>
	.mailbox {
		display: grid;
		gap: var(--space-3);
		padding: var(--space-4);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		max-width: 640px;
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2) var(--space-3);
	}
	h3 {
		margin: 0;
		font-size: var(--text-md);
	}
	p {
		margin: 0;
		color: var(--text-secondary);
		overflow-wrap: anywhere;
	}
	.pill {
		font-size: var(--text-xs);
		font-weight: 600;
		padding: 1px var(--space-2);
		border-radius: 999px;
		border: var(--border-width) solid currentColor;
	}
	.pill.quiet {
		color: var(--text-secondary);
	}
	.pill.ok {
		color: var(--accent-text);
	}
	.pill.bad,
	.problem {
		color: var(--danger);
	}
	.code {
		justify-self: start;
		padding: var(--space-2) var(--space-4);
		border: var(--border-width) dashed var(--border-strong);
		border-radius: var(--radius-md);
		font-family: var(--font-mono);
		font-size: var(--text-xl);
		font-weight: 600;
		letter-spacing: 0.12em;
		color: var(--text-primary);
	}
	.sub {
		font-size: var(--text-sm);
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.facts {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: var(--space-1) var(--space-4);
		margin: 0;
		font-size: var(--text-sm);
	}
	.facts dt {
		color: var(--text-secondary);
	}
	.facts dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	.confirm {
		display: grid;
		gap: var(--space-2);
	}
</style>
