<script lang="ts">
	import { commentParts, SECTION_MARK } from '../../mentions';
	import { openTask } from '../../taskLink';

	interface Props {
		body: string;
		/** A mentioned person's name now; the name stored with the mention otherwise. */
		nameOf?: (userId: string) => string | undefined;
		/** A task or page's title now, if this person can open it. */
		titleOf?: (target: { kind: 'page' | 'task'; id: string }) => string | undefined;
	}

	let { body, nameOf = () => undefined, titleOf = () => undefined }: Props = $props();
	const parts = $derived(commentParts(body));

	/** A reference's words: the title now, with the section it was made with. */
	function words(label: string, href: string, title: string): string {
		const section = href.includes('#') ? label.split(SECTION_MARK).pop() : undefined;
		return section ? title + SECTION_MARK + section : title;
	}
</script>

<!-- One line of markup: the text keeps its own white space. -->
<!-- eslint-disable svelte/no-navigation-without-resolve -- a reference's address is the app's own, made by pageHref or taskHref; a web address is someone else's -->
{#each parts as part, i (i)}{#if 'userId' in part}<span class="mention"
			>@{nameOf(part.userId) ?? part.name}</span
		>{:else if 'url' in part}<a href={part.url} target="_blank" rel="noopener noreferrer nofollow"
			>{part.url}</a
		>{:else if 'target' in part}{@const title = titleOf(part.target)}{#if title === undefined}<span
				class="unknown">{part.target.kind === 'task' ? 'a task' : 'a page'}</span
			>{:else if part.target.kind === 'task'}{@const id = part.target.id}<a
				href={part.href}
				onclick={(e) => {
					e.preventDefault();
					void openTask(id);
				}}>{words(part.label, part.href, title)}</a
			>{:else}<a href={part.href}>{words(part.label, part.href, title)}</a
			>{/if}{:else}{part.text}{/if}{/each}

<style>
	.mention {
		padding: 0 var(--space-1);
		border-radius: var(--radius-sm);
		background: var(--accent-soft);
		color: var(--text-primary);
		font-weight: 550;
		white-space: nowrap;
	}
	a {
		color: var(--accent-text);
		text-decoration: underline;
		overflow-wrap: anywhere;
	}
	.unknown {
		color: var(--text-secondary);
		font-style: italic;
	}
</style>
