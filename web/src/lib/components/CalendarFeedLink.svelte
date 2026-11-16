<script lang="ts">
	import Copy from '@lucide/svelte/icons/copy';
	import { copyText } from '../clipboard';
	import { uuid } from '../uuid';
	import Button from './Button.svelte';

	interface Props {
		/** The feed's link, which Mila shows only this once. */
		url: string;
		/** What the feed shows, such as "My tasks". */
		name: string;
		ondone?: () => void;
	}

	let { url, name, ondone }: Props = $props();

	let input = $state<HTMLInputElement>();
	let status = $state('');
	// Calendar apps open webcal: links and subscribe, rather than download.
	const webcal = $derived(url.replace(/^https?:/, 'webcal:'));
	const id = `feed-${uuid()}`;

	/** Moves focus to the link, whose description says it is shown once. */
	export function focus() {
		input?.focus();
	}

	async function copy() {
		if (await copyText(url, input)) status = 'Copied the link';
		else {
			input?.select();
			status = 'Copying did not work here. The link is selected; copy it from the field.';
		}
	}
</script>

<div class="feed" role="group" aria-labelledby="{id}-title">
	<p class="title" id="{id}-title">Your link for {name}</p>
	<p class="hint" id="{id}-once">
		This is the only time Mila shows it. Anyone with the link can see these tasks, so keep it to
		yourself; revoke the feed if it gets out.
	</p>
	<div class="row">
		<input
			bind:this={input}
			readonly
			value={url}
			aria-label="Calendar feed link"
			aria-describedby="{id}-once"
			onfocus={(e) => e.currentTarget.select()}
		/>
		<Button title="Copy the feed's address" onclick={copy}><Copy size={15} />Copy</Button>
	</div>
	<p class="hint">
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- a link out to a calendar app -->
		<a href={webcal}>Open in a calendar app</a>, or paste the link where your calendar subscribes to
		one (in Google Calendar: Other calendars, From URL). Calendar apps look for changes every hour
		or so.
	</p>
	<p class="status" role="status">{status}</p>
	{#if ondone}
		<div><Button title="Close this" variant="ghost" size="sm" onclick={ondone}>Done</Button></div>
	{/if}
</div>

<style>
	.feed {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-4);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-lg);
		background: var(--surface-1);
	}
	.title {
		font-weight: 600;
	}
	.hint {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.hint a {
		color: var(--text-primary);
		font-weight: 550;
		text-decoration: underline;
	}
	.row {
		display: flex;
		gap: var(--space-2);
		min-width: 0;
	}
	input {
		flex: 1;
		min-width: 0;
		height: calc(32px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--input-border);
		border-radius: var(--input-radius);
		background: var(--input-bg);
		color: var(--input-text);
		font-family: var(--font-mono);
		font-size: var(--text-xs);
	}
	input::placeholder {
		color: var(--input-placeholder);
	}
	.status {
		min-height: 1.4em;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
