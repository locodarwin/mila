<script lang="ts">
	// One thread on a wiki page: the words it is on, its comments and, while it is
	// the thread picked, the box to reply and the button to resolve or reopen it.
	import {
		referencesToDisplay,
		referencesToStored,
		toDisplay,
		toStored,
		type Person,
		type Reference
	} from '../../mentions';
	import { canDeleteOthersComments } from '../../roles';
	import { session } from '../../session.svelte';
	import { engine } from '../../sync/engine.svelte';
	import type { PageComment, PageThread } from '../../sync/entities';
	import {
		deletePageComment,
		editPageComment,
		reopenThread,
		replyToThread,
		resolveThread
	} from '../../sync/pageComments';
	import { relative } from '../../time';
	import { toasts } from '../../toasts.svelte';
	import Button from '../Button.svelte';
	import Avatar from '../task/Avatar.svelte';
	import { referenceSource, referenceTitle } from '../task/commentSources';
	import MentionInput from '../task/MentionInput.svelte';
	import MentionText from '../task/MentionText.svelte';

	interface Props {
		ws: string;
		thread: PageThread;
		comments: PageComment[];
		/** Who can be mentioned here. */
		people: Person[];
		active: boolean;
		/** The words it is on are no longer in the page. */
		gone: boolean;
		onpick: (id: string) => void;
	}

	let { ws, thread, comments, people, active, gone, onpick }: Props = $props();

	const me = $derived(session.user?.id ?? '');
	const references = $derived(referenceSource(ws));
	const titleOf = $derived(referenceTitle(ws));
	const resolved = $derived(thread.resolvedAt !== null);

	let body = $state('');
	let picked = $state<Person[]>([]);
	let refs = $state<Reference[]>([]);
	let editing = $state<string | null>(null);
	let draft = $state('');
	let draftPicked = $state<Person[]>([]);
	let draftRefs = $state<Reference[]>([]);

	function reply(e?: Event) {
		e?.preventDefault();
		if (!body.trim() || !me) return;
		const text = toStored(referencesToStored(body, refs), picked, people);
		void replyToThread(ws, thread.pageId, thread.id, me, text);
		body = '';
		picked = [];
		refs = [];
	}

	function edit(c: PageComment) {
		const shown = referencesToDisplay(c.body);
		({ text: draft, picked: draftPicked } = toDisplay(shown.text));
		draftRefs = shown.refs;
		editing = c.id;
	}

	function save(c: PageComment) {
		const next = toStored(referencesToStored(draft.trim(), draftRefs), draftPicked, people);
		if (next && next !== c.body) void editPageComment(ws, c.id, next);
		editing = null;
	}

	// An admin taking down what someone else wrote, with a way back.
	function removeOthers(id: string) {
		const { undo } = engine.track(() => deletePageComment(ws, id));
		toasts.show('Comment deleted', { onundo: () => void undo() });
	}
</script>

<!-- A click anywhere picks the thread; the button on its words does the same from the keyboard. -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<article
	class="thread"
	class:active
	aria-label={`Comments on: ${thread.quote}`}
	data-thread-card={thread.id}
	onclick={() => {
		if (!active) onpick(thread.id);
	}}
>
	<button
		class="quote"
		class:gone
		type="button"
		title={gone ? 'Pick this thread' : 'Show these words in the page'}
		onclick={(e) => {
			e.stopPropagation();
			onpick(thread.id);
		}}>{thread.quote}</button
	>
	{#if gone && !resolved}
		<p class="note">The text this was on was removed.</p>
	{/if}
	<ul>
		{#each comments as c (c.id)}
			<li>
				<Avatar
					name={session.nameOf(ws, c.authorId)}
					src={session.personOf(ws, c.authorId).avatar}
					size={22}
				/>
				<div class="bubble">
					<p class="meta">
						<strong>{session.nameOf(ws, c.authorId)}</strong>
						<span>{relative(c.createdAt)}{c.editedAt ? ' · edited' : ''}</span>
					</p>
					{#if editing === c.id}
						<MentionInput
							label="Edit comment"
							bind:value={draft}
							bind:picked={draftPicked}
							bind:refs={draftRefs}
							{people}
							{references}
							rows={3}
							autofocus
						/>
						<div class="actions">
							<Button
								title="Save your changes to this comment"
								size="sm"
								variant="primary"
								onclick={() => save(c)}>Save</Button
							>
							<Button
								title="Discard your changes to this comment"
								size="sm"
								variant="ghost"
								onclick={() => (editing = null)}>Cancel</Button
							>
						</div>
					{:else}
						<p class="body">
							<MentionText body={c.body} nameOf={(id) => session.memberName(ws, id)} {titleOf} />
						</p>
						{#if c.authorId === me}
							<div class="links">
								<button title="Edit this comment" type="button" onclick={() => edit(c)}>Edit</button
								>
								<button
									title="Delete this comment"
									type="button"
									onclick={() => void deletePageComment(ws, c.id)}>Delete</button
								>
							</div>
						{:else if session.workspace && canDeleteOthersComments(session.workspace.role)}
							<div class="links">
								<button title="Delete this comment" type="button" onclick={() => removeOthers(c.id)}
									>Delete</button
								>
							</div>
						{/if}
					{/if}
				</div>
			</li>
		{/each}
	</ul>
	{#if active}
		{#if resolved}
			<div class="actions">
				<Button
					title="Bring this thread back, with its highlight"
					size="sm"
					onclick={() => void reopenThread(ws, thread.id)}>Reopen</Button
				>
			</div>
		{:else}
			<form onsubmit={reply}>
				<MentionInput
					label={`Reply to the comments on: ${thread.quote}`}
					placeholder="Reply. @ mentions someone"
					bind:value={body}
					bind:picked
					bind:refs
					{people}
					{references}
					onkeydown={(e) => (e.metaKey || e.ctrlKey) && e.key === 'Enter' && reply(e)}
				/>
				<div class="actions">
					<Button
						title="Post your reply"
						type="submit"
						size="sm"
						variant="primary"
						disabled={!body.trim()}>Reply</Button
					>
					<Button
						title="Hide the highlight and move this thread to Resolved"
						size="sm"
						onclick={() => void resolveThread(ws, thread.id)}>Resolve</Button
					>
				</div>
			</form>
		{/if}
	{/if}
</article>

<style>
	.thread {
		display: grid;
		gap: var(--space-2);
		min-width: 0;
		padding: var(--space-3);
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	.thread.active {
		border-color: var(--accent);
	}
	.quote {
		display: block;
		width: 100%;
		min-height: max(var(--tap-min), 0px);
		padding: 0 0 0 var(--space-2);
		border: none;
		border-left: 2px solid var(--border-strong);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		text-align: left;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		cursor: pointer;
		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		overflow: hidden;
	}
	.quote:hover {
		color: var(--text-primary);
	}
	.quote.gone {
		border-left-color: var(--danger);
	}
	.note {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	ul {
		display: grid;
		gap: var(--space-3);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li {
		display: flex;
		gap: var(--space-2);
		align-items: flex-start;
	}
	.bubble {
		flex: 1;
		min-width: 0;
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		align-items: baseline;
		font-size: var(--text-sm);
	}
	.meta span {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.body {
		font-size: var(--text-sm);
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.links {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-1);
	}
	.links button {
		min-height: max(var(--tap-min), 0px);
		padding: 0;
		border: none;
		background: none;
		font-size: var(--text-xs);
		color: var(--text-secondary);
		cursor: pointer;
	}
	.links button:hover {
		color: var(--text-primary);
		text-decoration: underline;
	}
	form {
		display: grid;
		gap: var(--space-2);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
</style>
