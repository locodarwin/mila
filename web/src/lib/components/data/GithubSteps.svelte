<script lang="ts">
	import Copy from '@lucide/svelte/icons/copy';
	import { announce } from '../../announce.svelte';
	import { copyText } from '../../clipboard';
	import { GITHUB_COMMAND } from '../../githubimport';
	import Button from '../Button.svelte';

	let copied = $state(false);

	async function copy() {
		copied = await copyText(GITHUB_COMMAND);
		announce(copied ? 'Copied the command' : 'Could not copy; select the command and copy it');
	}
</script>

<ol class="steps">
	<li>
		Install GitHub's
		<a class="link" href="https://cli.github.com" target="_blank" rel="noopener noreferrer"
			>gh tool</a
		> and sign in with it, if you haven't.
	</li>
	<li>
		In a terminal, in the repository's folder, run this command. GitHub's site has no export for
		issues, so gh writes the file.
		<div class="command">
			<code>{GITHUB_COMMAND}</code>
			<Button size="sm" title="Copy the command" onclick={copy}>
				<Copy size={14} />{copied ? 'Copied' : 'Copy'}
			</Button>
		</div>
	</li>
	<li>Choose issues.json below. Nothing changes until you have seen what would come in.</li>
</ol>

<style>
	.steps {
		display: grid;
		gap: var(--space-2);
		margin: 0;
		padding-left: var(--space-5);
		font-size: var(--text-sm);
	}
	.command {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		margin-top: var(--space-2);
	}
	code {
		min-width: 0;
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		overflow-wrap: anywhere;
	}
	.link {
		color: var(--accent-text);
		font-weight: 550;
	}
</style>
