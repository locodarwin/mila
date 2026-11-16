<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import LogIn from '@lucide/svelte/icons/log-in';
	import LogOut from '@lucide/svelte/icons/log-out';
	import Mail from '@lucide/svelte/icons/mail';
	import ShieldCheck from '@lucide/svelte/icons/shield-check';
	import UserPlus from '@lucide/svelte/icons/user-plus';
	import type { Schemas } from '../api/client';
	import { formatClock, formatDate } from '../dates';
	import { deviceName } from '../device';
	import Button from './Button.svelte';

	type Event = Schemas['SecurityEvent'];

	interface Props {
		events: Event[];
		/** Older events remain; "Show older" calls onmore. */
		more?: boolean;
		busy?: boolean;
		onmore?: () => void;
	}

	let { events, more = false, busy = false, onmore }: Props = $props();

	const METHODS: Record<string, string> = {
		passkey: 'with a passkey',
		magic_link: 'with an email link',
		setup: 'while setting up Mila',
		invite: 'by accepting an invitation',
		desktop: 'in the desktop app'
	};

	/** What happened, in words. */
	function says(e: Event): string {
		switch (e.kind) {
			case 'sign_in':
				return `Signed in ${METHODS[e.method] ?? ''}`.trim();
			case 'invite_accepted':
				return `Joined ${e.detail}`;
			case 'passkey_added':
				return `Added the passkey ${e.detail}`;
			case 'passkey_removed':
				return e.method === 'admin'
					? `The passkey ${e.detail} was removed, signing out on every device`
					: `Removed the passkey ${e.detail}`;
			case 'session_revoked':
				return e.method === 'admin'
					? `Signed out on every device by ${e.detail}, an admin`
					: `Signed out ${deviceName(e.detail)}`;
			case 'admin_granted':
				return e.method === 'command'
					? 'Made an admin from the server'
					: `Made an admin by ${e.detail}`;
			case 'admin_revoked':
				return `No longer an admin, by ${e.detail}`;
			case 'email_changed': {
				const [from, to] = e.detail.split('\n');
				return `Changed the email address from ${from} to ${to}`;
			}
			case 'token_made':
				return `Made the API token ${e.detail}`;
			case 'token_revoked':
				return e.method === 'command'
					? `The app ${e.detail} was deleted from the server, ending its token`
					: `Revoked the API token ${e.detail}`;
			case 'app_connected':
				return `Connected the app ${e.detail}`;
			case 'email_address_made':
				return `Made an address for email to task, in ${e.detail}`;
			case 'email_address_removed':
				return `Deleted an address for email to task, in ${e.detail}`;
			case 'email_sender_added':
				return `Let another sender use an address for email to task, in ${e.detail}`;
			case 'webhook_changed':
				return `Set a webhook's address or secret, for ${e.detail}`;
		}
		return e.kind;
	}

	const icons = {
		sign_in: LogIn,
		invite_accepted: UserPlus,
		passkey_added: KeyRound,
		webhook_changed: KeyRound,
		passkey_removed: KeyRound,
		session_revoked: LogOut,
		admin_granted: ShieldCheck,
		admin_revoked: ShieldCheck,
		token_made: KeyRound,
		token_revoked: KeyRound,
		app_connected: KeyRound,
		email_address_made: Mail,
		email_address_removed: Mail,
		email_sender_added: Mail,
		email_changed: Mail
	};

	function when(at: string): string {
		const d = new Date(at);
		return `${formatDate(d, { weekday: 'short' })}, ${formatClock(d)}`;
	}
</script>

{#if events.length}
	<ul class="log" aria-label="Recent activity">
		{#each events as e (e.id)}
			{@const Icon = icons[e.kind] ?? LogIn}
			<li>
				<Icon size={16} aria-hidden="true" />
				<span class="grow">
					{says(e)}
					{#if e.newDevice}<span class="badge">New device</span>{/if}
					<span class="sub">
						<time datetime={e.createdAt}>{when(e.createdAt)}</time>
						{#if e.userAgent}· {deviceName(e.userAgent)}{/if}
						{#if e.ip}· {e.ip}{/if}
					</span>
				</span>
			</li>
		{/each}
	</ul>
	{#if more}
		<div>
			<Button title="Show older activity" size="sm" onclick={() => onmore?.()} disabled={busy}
				>Show older</Button
			>
		</div>
	{/if}
{:else}
	<p class="empty">Nothing yet.</p>
{/if}

<style>
	.log {
		margin: 0;
		padding: 0;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	li {
		display: flex;
		align-items: flex-start;
		gap: var(--space-3);
		min-height: var(--row-height);
		padding: var(--pad-y) var(--space-4);
	}
	li :global(svg) {
		flex: none;
		margin-top: 2px;
		color: var(--text-secondary);
	}
	li + li {
		border-top: 1px solid var(--border);
	}
	.grow {
		flex: 1;
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.sub {
		display: block;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.badge {
		margin-left: var(--space-1);
		padding: 1px var(--space-2);
		border-radius: var(--radius-full);
		background: var(--accent-soft);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	.empty {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
