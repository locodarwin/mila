<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import CircleQuestionMark from '@lucide/svelte/icons/circle-question-mark';
	import type { Snippet } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import { layout } from '../../layout.svelte';

	// A small link to a help topic, beside the controls it explains. The
	// open task stays open beside the topic where there is room for both;
	// where the task covers the page (a phone's sheet, or a task expanded
	// to the full page) it closes, or the topic would open unseen under it.
	// With a label and no words it is the question mark alone, right after the
	// name of what it explains; the label is its name and tooltip.
	interface Props {
		topic: string;
		section?: string;
		/** Runs as the link is followed: closing the dialog it is in, say. */
		onclick?: () => void;
		label?: string;
		children?: Snippet;
	}

	let { topic, section, onclick, label, children }: Props = $props();

	// As the app layout decides it puts the task over the page.
	const phone = new MediaQuery('max-width: 720px');
	const covers = $derived(phone.current || layout.expanded);
	const task = $derived(covers ? null : page.url.searchParams.get('task'));
	const href = $derived(
		resolve('/(app)/help/[topic]', { topic }) +
			(task ? `?task=${encodeURIComponent(task)}` : '') +
			(section ? `#${section}` : '')
	);
</script>

<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved, with a query and a fragment -->
<a class="help-link" class:mark={!children} aria-label={label} title={label} {href} {onclick}
	><CircleQuestionMark size={14} aria-hidden="true" />{@render children?.()}</a
>

<style>
	.help-link {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		text-decoration: none;
	}
	.help-link:hover {
		color: var(--text-primary);
		text-decoration: underline;
	}
	.mark {
		flex: none;
		justify-content: center;
		min-width: max(var(--tap-min), 20px);
		min-height: max(var(--tap-min), 20px);
		border-radius: var(--radius-full);
	}
	.mark:hover {
		background: var(--accent-soft);
		color: var(--accent-text);
	}
</style>
