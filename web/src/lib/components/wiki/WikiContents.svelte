<script lang="ts">
	// The contents of the wiki page shown: its headings as they are drawn, so
	// they follow what is being typed, and the one being read is marked.
	import { untrack } from 'svelte';
	import { announce } from '../../announce.svelte';
	import { currentHeading, indents } from '../../wikiContents';

	interface Props {
		/** What holds the page's document. */
		root: HTMLElement | undefined;
		/** What scrolls it. */
		scroller: HTMLElement | undefined;
		/** How much of the top of what scrolls is covered. */
		barHeight: number;
		/** The page is showing as Markdown, so its headings are not drawn. */
		source: boolean;
	}

	let { root, scroller, barHeight, source }: Props = $props();

	let found = $state.raw<{ el: HTMLElement; level: number; text: string }[]>([]);
	let current = $state(-1);
	const steps = $derived(indents(found));

	function locate() {
		if (!scroller) return;
		const line = scroller.getBoundingClientRect().top + barHeight + 16;
		current = currentHeading(
			found.map((h) => h.el.getBoundingClientRect().top),
			line,
			{
				top: scroller.scrollTop,
				height: scroller.scrollHeight,
				client: scroller.clientHeight
			}
		);
	}

	function read() {
		if (!root) return;
		found = [...root.querySelectorAll<HTMLElement>('.doc-body :is(h1, h2, h3, h4, h5, h6)')]
			.map((el) => ({ el, level: Number(el.tagName[1]), text: el.textContent?.trim() ?? '' }))
			.filter((h) => h.text);
		locate();
	}

	// Typing changes the document many times a second; a moment's wait is enough.
	$effect(() => {
		if (!root) return;
		let waiting: ReturnType<typeof setTimeout> | undefined;
		const changes = new MutationObserver(() => {
			waiting ??= setTimeout(() => {
				waiting = undefined;
				read();
			}, 150);
		});
		changes.observe(root, { childList: true, subtree: true, characterData: true });
		untrack(read);
		return () => {
			changes.disconnect();
			clearTimeout(waiting);
		};
	});

	$effect(() => {
		const at = scroller;
		if (!at) return;
		let frame = 0;
		const scrolled = () => {
			frame ||= requestAnimationFrame(() => {
				frame = 0;
				locate();
			});
		};
		at.addEventListener('scroll', scrolled, { passive: true });
		return () => {
			at.removeEventListener('scroll', scrolled);
			cancelAnimationFrame(frame);
		};
	});

	function go(h: { el: HTMLElement; text: string }) {
		h.el.scrollIntoView({ block: 'start' });
		announce(h.text);
	}
</script>

{#if source}
	<p class="empty">Headings show here when the page is not showing Markdown.</p>
{:else if found.length === 0}
	<p class="empty">Headings in this page will show here.</p>
{:else}
	<nav aria-label="Contents">
		<ul>
			{#each found as h, i (i)}
				<li>
					<button
						type="button"
						aria-current={i === current ? 'location' : undefined}
						style:padding-left={`calc(var(--space-2) + ${steps[i]} * var(--space-3))`}
						onclick={() => go(h)}>{h.text}</button
					>
				</li>
			{/each}
		</ul>
	</nav>
{/if}

<style>
	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	button {
		display: block;
		width: 100%;
		min-height: max(var(--tap-min), 28px);
		padding: var(--space-1) var(--space-2);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		text-align: left;
		overflow-wrap: anywhere;
		cursor: pointer;
	}
	button:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	button[aria-current] {
		color: var(--text-primary);
		font-weight: 600;
	}
	.empty {
		margin: 0;
		padding: var(--space-1) var(--space-2);
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
</style>
