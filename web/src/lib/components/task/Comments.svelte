<script lang="ts">
	import { menu } from '../../contextMenu.svelte';
	import {
		referencesToDisplay,
		referencesToStored,
		toDisplay,
		toStored,
		type Person,
		type Reference
	} from '../../mentions';
	import { session } from '../../session.svelte';
	import { engine } from '../../sync/engine.svelte';
	import { addComment, deleteComment, editComment } from '../../sync/data';
	import { canDeleteOthersComments } from '../../roles';
	import { toasts } from '../../toasts.svelte';
	import type { Comment } from '../../sync/entities';
	import { relative } from '../../time';
	import { label } from '../../keys/chord';
	import { keymap } from '../../keys/keymap.svelte';
	import Button from '../Button.svelte';
	import Avatar from './Avatar.svelte';
	import { referenceSource, referenceTitle } from './commentSources';
	import MentionInput from './MentionInput.svelte';
	import MentionText from './MentionText.svelte';

	let { ws, taskId, items }: { ws: string; taskId: string; items: Comment[] } = $props();
	let body = $state('');
	let picked = $state<Person[]>([]);
	let refs = $state<Reference[]>([]);
	let editing = $state<string | null>(null);
	let draft = $state('');
	let draftPicked = $state<Person[]>([]);
	let draftRefs = $state<Reference[]>([]);
	const me = $derived(session.user?.id ?? '');
	const replica = $derived(engine.replica(ws));
	// Who can be mentioned: everyone else who can reach this task. The server
	// checks again before telling anyone.
	const people = $derived(session.mentionable(ws, replica?.task(taskId)?.projectId ?? null));

	const references = $derived(referenceSource(ws, taskId));
	const titleOf = $derived(referenceTitle(ws));

	function post(e?: Event) {
		e?.preventDefault();
		if (!body.trim() || !me) return;
		void addComment(ws, taskId, me, toStored(referencesToStored(body, refs), picked, people));
		body = '';
		picked = [];
		refs = [];
	}

	function edit(c: Comment) {
		const shown = referencesToDisplay(c.body);
		({ text: draft, picked: draftPicked } = toDisplay(shown.text));
		draftRefs = shown.refs;
		editing = c.id;
	}

	function save(c: Comment) {
		const next = toStored(referencesToStored(draft.trim(), draftRefs), draftPicked, people);
		if (next && next !== c.body) void editComment(ws, c.id, next);
		editing = null;
	}

	// An admin taking down what someone else wrote, with a way back.
	function removeOthers(id: string) {
		const { undo } = engine.track(() => deleteComment(ws, id));
		toasts.show('Comment deleted', { onundo: () => void undo() });
	}
</script>

<ul aria-label="Comments">
	{#each items as c (c.id)}
		<li
			use:menu={() =>
				import('../../menus').then((m) =>
					m.commentMenu(
						ws,
						taskId,
						c,
						() => edit(c),
						() => removeOthers(c.id)
					)
				)}
		>
			<Avatar
				name={session.nameOf(ws, c.authorId)}
				src={session.personOf(ws, c.authorId).avatar}
				size={26}
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
						<div class="actions">
							<button title="Edit this comment" type="button" onclick={() => edit(c)}>Edit</button>
							<button
								title="Delete this comment"
								type="button"
								onclick={() => void deleteComment(ws, c.id)}>Delete</button
							>
						</div>
					{:else if session.workspace && canDeleteOthersComments(session.workspace.role)}
						<div class="actions">
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
<form onsubmit={post}>
	<MentionInput
		label="New comment"
		placeholder="Write a comment. @ mentions someone, / refers to a task or wiki page"
		bind:value={body}
		bind:picked
		bind:refs
		{people}
		{references}
		onkeydown={(e) => (e.metaKey || e.ctrlKey) && e.key === 'Enter' && post(e)}
	/>
	<div class="send">
		<span class="hint">{label('Mod+Enter', keymap.apple)} to send</span>
		<Button
			title="Post your comment"
			type="submit"
			size="sm"
			variant="primary"
			disabled={!body.trim()}>Comment</Button
		>
	</div>
</form>

<style>
	ul {
		display: grid;
		gap: var(--space-3);
		margin: 0 0 var(--space-3);
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
		gap: var(--space-2);
		align-items: baseline;
		font-size: var(--text-sm);
	}
	.meta span {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.body {
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.actions {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-1);
	}
	.actions button {
		padding: 0;
		border: none;
		background: none;
		font-size: var(--text-xs);
		color: var(--text-secondary);
		cursor: pointer;
	}
	.actions button:hover {
		color: var(--text-primary);
		text-decoration: underline;
	}
	.send {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-3);
		margin-top: var(--space-2);
	}
	.hint {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
</style>
