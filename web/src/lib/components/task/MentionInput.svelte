<script lang="ts">
	import {
		activeQuery,
		activeSlash,
		insert,
		matching,
		SECTION_MARK,
		shownReference,
		SLASH_COMMANDS,
		slugOf,
		type Person,
		type Query,
		type Reference,
		type SlashQuery
	} from '../../mentions';
	import { uuid } from '../../uuid';
	import Avatar from './Avatar.svelte';

	/** Something `/task` or `/wiki` can refer to. */
	export interface ReferenceChoice {
		id: string;
		title: string;
		href: string;
		where: string;
	}

	/** Where the box finds what slash commands refer to. */
	export interface ReferenceSource {
		tasks: (query: string) => ReferenceChoice[];
		pages: (query: string) => ReferenceChoice[];
		sections: (pageId: string) => string[];
	}

	interface Props {
		/** The text as shown, mentions as "@Name", references as "[[Title]]". */
		value: string;
		/** The people picked from the list, to store as mentions on sending. */
		picked: Person[];
		/** Who can be mentioned. */
		people: Person[];
		/** The tasks and pages picked, to store as links on sending. */
		refs?: Reference[];
		/** Turns on the slash commands. */
		references?: ReferenceSource;
		label: string;
		placeholder?: string;
		rows?: number;
		autofocus?: boolean;
		/** Keys the list does not use (Ctrl+Enter to send). */
		onkeydown?: (e: KeyboardEvent) => void;
	}

	let {
		value = $bindable(''),
		picked = $bindable([]),
		people,
		refs = $bindable([]),
		references,
		label,
		placeholder,
		rows = 2,
		autofocus = false,
		onkeydown
	}: Props = $props();

	type Option =
		| { kind: 'person'; key: string; label: string; person: Person }
		| { kind: 'command'; key: string; label: string; word: string }
		| { kind: 'task' | 'page'; key: string; label: string; where: string; choice: ReferenceChoice }
		| { kind: 'section'; key: string; label: string; section: string | null };

	const listId = `mentions-${uuid()}`;
	let input = $state<HTMLTextAreaElement>();
	let query = $state<Query | null>(null);
	let slash = $state<SlashQuery | null>(null);
	// A page picked with `/wiki` that has sections to choose from.
	let forPage = $state<ReferenceChoice | null>(null);
	let active = $state(0);
	// The chosen one stays in view as the arrows move past the list's edge.
	$effect(() => {
		if (open) document.getElementById(`${listId}-${active}`)?.scrollIntoView({ block: 'nearest' });
	});

	const choices = (list: ReferenceChoice[], kind: 'task' | 'page'): Option[] =>
		list.map((c) => ({ kind, key: c.id, label: c.title, where: c.where, choice: c }));

	const options = $derived.by((): Option[] => {
		if (query) {
			return matching(people, query.query).map((p) => ({
				kind: 'person',
				key: p.userId,
				label: p.name,
				person: p
			}));
		}
		if (forPage && references) {
			return [
				{ kind: 'section', key: 'whole', label: 'The whole page', section: null },
				...references
					.sections(forPage.id)
					.map((h, i): Option => ({ kind: 'section', key: `${i}`, label: h, section: h }))
			];
		}
		if (!slash || !references) return [];
		if (slash.query === null) {
			const word = slash.word;
			return SLASH_COMMANDS.filter((c) => c.word.startsWith(word)).map((c) => ({
				kind: 'command',
				key: c.word,
				label: `/${c.word}: ${c.label}`,
				word: c.word
			}));
		}
		return slash.word === 'task'
			? choices(references.tasks(slash.query), 'task')
			: choices(references.pages(slash.query), 'page');
	});
	const open = $derived(options.length > 0);
	// What the list holds, for its name and what a screen reader hears.
	const what = $derived(
		query
			? { list: 'People to mention', one: 'person to mention', many: 'people to mention' }
			: forPage
				? { list: 'Sections', one: 'section', many: 'sections' }
				: slash?.query === null
					? { list: 'Commands', one: 'command', many: 'commands' }
					: slash?.word === 'task'
						? { list: 'Tasks', one: 'task', many: 'tasks' }
						: { list: 'Wiki pages', one: 'page', many: 'pages' }
	);
	// Typing a name nobody with access has, or searching and finding nothing.
	const none = $derived.by(() => {
		if (open) return '';
		if (query) {
			return people.length
				? 'Nobody with access to this has that name.'
				: 'Nobody else has access to this.';
		}
		if (slash && slash.query !== null && slash.query.trim()) return 'Nothing found by that name.';
		return '';
	});
	// Said when the list opens or narrows: while it is open, Enter and Tab
	// pick from it instead of their usual jobs.
	const announcement = $derived(
		open
			? `${options.length} ${options.length === 1 ? what.one : what.many}. Arrow keys to choose, Enter or Tab to pick.`
			: none
	);

	$effect(() => {
		if (autofocus) input?.focus();
	});

	function update() {
		if (!input) return;
		const caret = input.selectionStart;
		const whole = caret === input.selectionEnd;
		const q = whole ? activeQuery(value, caret) : null;
		const s = whole && !q && references ? activeSlash(value, caret) : null;
		if (
			q?.start !== query?.start ||
			q?.query !== query?.query ||
			s?.start !== slash?.start ||
			s?.word !== slash?.word ||
			s?.query !== slash?.query
		) {
			active = 0;
			forPage = null;
		}
		query = q;
		slash = s;
	}

	/** Replaces what is typed from a position up to the caret. */
	function put(from: number, text: string) {
		if (!input) return;
		const caret = input.selectionStart;
		const rest = value.slice(caret).replace(/^ /, '');
		// Set at once, so a key pressed right after lands after what went in.
		input.value = value = value.slice(0, from) + text + rest;
		input.setSelectionRange(from + text.length, from + text.length);
	}

	function refer(label: string, href: string) {
		if (!slash) return;
		put(slash.start, shownReference(label) + ' ');
		if (!refs.some((r) => r.href === href)) refs = [...refs, { label, href }];
		close();
	}

	function close() {
		query = null;
		slash = null;
		forPage = null;
		active = 0;
	}

	function pick(o: Option) {
		if (!input) return;
		switch (o.kind) {
			case 'person': {
				if (!query) return;
				const next = insert(value, query, input.selectionStart, o.person);
				// Set at once, so a key pressed right after lands after the name.
				input.value = value = next.text;
				input.setSelectionRange(next.caret, next.caret);
				if (!picked.some((x) => x.userId === o.person.userId)) picked = [...picked, o.person];
				close();
				return;
			}
			case 'command':
				if (slash) put(slash.start, `/${o.word} `);
				update();
				return;
			case 'task':
				refer(o.choice.title, o.choice.href);
				return;
			case 'page':
				if (references?.sections(o.choice.id).length) {
					forPage = o.choice;
					active = 0;
				} else {
					refer(o.choice.title, o.choice.href);
				}
				return;
			case 'section':
				if (!forPage) return;
				if (o.section === null) refer(forPage.title, forPage.href);
				else
					refer(forPage.title + SECTION_MARK + o.section, `${forPage.href}#${slugOf(o.section)}`);
		}
	}

	function keydown(e: KeyboardEvent) {
		if (open && !e.ctrlKey && !e.metaKey && !e.altKey) {
			if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
				e.preventDefault();
				const step = e.key === 'ArrowDown' ? 1 : -1;
				active = (active + step + options.length) % options.length;
				return;
			}
			if ((e.key === 'Enter' && !e.shiftKey) || e.key === 'Tab') {
				e.preventDefault();
				pick(options[active]!);
				return;
			}
			if (e.key === 'Escape') {
				// Closes the list, not the task panel.
				e.preventDefault();
				e.stopPropagation();
				close();
				return;
			}
		}
		onkeydown?.(e);
	}
</script>

<div class="mention-input">
	<textarea
		bind:this={input}
		bind:value
		aria-label={label}
		{placeholder}
		{rows}
		aria-autocomplete="list"
		aria-controls={open ? listId : undefined}
		aria-activedescendant={open ? `${listId}-${active}` : undefined}
		oninput={update}
		onclick={update}
		onkeyup={(e) => (e.key === 'ArrowLeft' || e.key === 'ArrowRight') && update()}
		onkeydown={keydown}
		onblur={close}></textarea>
	<span class="visually-hidden" role="status">{announcement}</span>
	{#if open}
		<ul id={listId} role="listbox" aria-label={what.list}>
			{#each options as o, i (o.key)}
				<!-- Picked with the keyboard from the text box; the mouse also works. -->
				<!-- svelte-ignore a11y_click_events_have_key_events -->
				<li
					id="{listId}-{i}"
					role="option"
					aria-selected={i === active}
					onmousedown={(e) => e.preventDefault()}
					onclick={() => pick(o)}
				>
					{#if o.kind === 'person'}<Avatar name={o.person.name} src={o.person.avatar} />{/if}
					<span class="label">{o.label}</span>
					{#if o.kind === 'task' || o.kind === 'page'}<span class="where">{o.where}</span>{/if}
				</li>
			{/each}
		</ul>
	{:else if none}
		<p class="none">{none}</p>
	{/if}
</div>

<style>
	.mention-input {
		position: relative;
	}
	textarea {
		display: block;
		width: 100%;
		padding: var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
		resize: vertical;
	}
	/* A border and a ring inside it, two pixels of accent in all. */
	textarea:focus-visible {
		border-color: var(--accent);
		box-shadow: inset 0 0 0 1px var(--accent);
	}
	ul {
		position: absolute;
		top: calc(100% + var(--space-1));
		left: 0;
		z-index: 10;
		min-width: 200px;
		max-width: 100%;
		max-height: 280px;
		margin: 0;
		padding: var(--space-1);
		overflow-y: auto;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-0);
		box-shadow: var(--shadow-lg);
	}
	li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-md);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	li:hover {
		background: var(--surface-2);
	}
	li[aria-selected='true'] {
		background: var(--accent-soft);
	}
	.label {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.where {
		flex: none;
		margin-left: auto;
		padding-left: var(--space-2);
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.none {
		position: absolute;
		top: calc(100% + var(--space-1));
		left: 0;
		z-index: 10;
		margin: 0;
		padding: var(--space-1) var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		background: var(--surface-0);
		box-shadow: var(--shadow-lg);
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
</style>
