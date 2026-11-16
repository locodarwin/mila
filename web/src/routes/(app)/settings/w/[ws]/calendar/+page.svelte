<script lang="ts">
	import { settingsWorkspace } from '#lib/settingsWorkspace.svelte.js';
	import { tick } from 'svelte';
	import { page } from '$app/state';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import { api, call, type Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import CalendarFeedLink from '#lib/components/CalendarFeedLink.svelte';
	import { session } from '#lib/session.svelte.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import { showFailure } from '#lib/signInAgain.js';
	import { relative } from '#lib/time.js';
	import { toasts } from '#lib/toasts.svelte.js';

	type Feed = Schemas['CalendarFeed'];

	const ws = $derived(settingsWorkspace()?.id ?? '');
	const me = $derived(session.user?.id ?? '');
	const replica = $derived(engine.replica(ws));
	const projects = $derived(replica?.projects() ?? []);
	const views = $derived(replica?.views(me) ?? []);

	let feeds = $state<Feed[]>([]);
	let made = $state<Schemas['NewCalendarFeed'] | null>(null);
	let busy = $state(false);
	let confirming = $state<string | null>(null);
	let pick = $state<HTMLSelectElement>();
	let shown = $state<ReturnType<typeof CalendarFeedLink>>();
	// "me", "project:<id>" or "view:<id>". A project's or view's page links
	// here with it chosen.
	const preset = $derived.by(() => {
		const q = page.url.searchParams;
		return q.get('project')
			? `project:${q.get('project')}`
			: q.get('view')
				? `view:${q.get('view')}`
				: 'me';
	});
	let choice = $derived(preset);

	async function load() {
		if (!ws) return;
		feeds = await call(
			api.GET('/workspaces/{workspaceId}/calendar-feeds', {
				params: { path: { workspaceId: ws } }
			})
		);
	}

	$effect(() => {
		made = null;
		load().catch((err) => showFailure(err));
	});

	async function create(e: SubmitEvent) {
		e.preventDefault();
		const [scope, targetId] = choice.split(':') as ['me' | 'project' | 'view', string?];
		busy = true;
		try {
			made = await call(
				api.POST('/workspaces/{workspaceId}/calendar-feeds', {
					params: { path: { workspaceId: ws } },
					body: { scope, ...(targetId ? { targetId } : {}) }
				})
			);
			// Straight to the new link, so a screen reader hears it and that
			// it is shown only this once.
			await tick();
			shown?.focus();
			await load();
		} catch (err) {
			showFailure(err);
		} finally {
			busy = false;
		}
	}

	// Revoking cannot be undone, so it takes a second click.
	async function revoke(feed: Feed) {
		if (confirming !== feed.id) {
			confirming = feed.id;
			return;
		}
		confirming = null;
		try {
			await call(
				api.DELETE('/workspaces/{workspaceId}/calendar-feeds/{feedId}', {
					params: { path: { workspaceId: ws, feedId: feed.id } }
				})
			);
			if (made?.feed.id === feed.id) made = null;
			toasts.show(`Revoked the feed ${feed.name}`);
			await load();
		} catch (err) {
			showFailure(err);
		}
	}

	const scopes: Record<Feed['scope'], string> = {
		me: 'Your tasks and My Day',
		project: 'Project',
		view: 'Saved view'
	};
</script>

<svelte:head><title>Calendar feeds · Mila</title></svelte:head>

<section>
	<h2>Calendar feeds</h2>
	<p class="hint">
		See your tasks in Google Calendar, Apple Calendar or Outlook. Tasks with a due date show as
		events, all day unless they have a time, and each links back to its task. A feed only reads; it
		never changes anything in Mila.
	</p>
	<form class="inline-form" onsubmit={create}>
		<label class="pick grow">
			<span>Show in the feed</span>
			<select bind:this={pick} bind:value={choice}>
				<option value="me">My tasks: assigned to me, and My Day</option>
				{#if projects.length}
					<optgroup label="Projects">
						{#each projects as p (p.id)}<option value="project:{p.id}">{p.name}</option>{/each}
					</optgroup>
				{/if}
				{#if views.length}
					<optgroup label="Saved views">
						{#each views as v (v.id)}<option value="view:{v.id}">{v.name}</option>{/each}
					</optgroup>
				{/if}
			</select>
		</label>
		<Button type="submit" variant="primary" disabled={busy || !ws}>
			<CalendarDays size={16} />Make a feed
		</Button>
	</form>
	{#if made}
		<CalendarFeedLink
			bind:this={shown}
			url={made.url}
			name={made.feed.name}
			ondone={() => {
				made = null;
				pick?.focus();
			}}
		/>
	{/if}
</section>

<section>
	<h2>Your feeds</h2>
	{#if feeds.length}
		<ul class="rows" aria-label="Calendar feeds">
			{#each feeds as feed (feed.id)}
				<li>
					<CalendarDays size={16} />
					<span class="grow">
						{feed.name}
						<span class="sub">
							{scopes[feed.scope]} · made {relative(feed.createdAt)} · {feed.lastUsedAt
								? `last read ${relative(feed.lastUsedAt)}`
								: 'not read yet'}
						</span>
					</span>
					<Button
						size="sm"
						variant="danger"
						aria-label={`${confirming === feed.id ? 'Confirm revoke' : 'Revoke'} ${feed.name}`}
						onclick={() => revoke(feed)}
					>
						{confirming === feed.id ? 'Confirm revoke' : 'Revoke'}
					</Button>
				</li>
			{/each}
		</ul>
	{:else}
		<p class="hint">You have no calendar feeds in this workspace.</p>
	{/if}
	<p class="hint">
		A feed shows what you can see when your calendar app reads it, so leaving the workspace or
		losing a project ends it. Revoke a feed to stop its link working at once.
	</p>
</section>

<style>
	/* Wide enough for its longest choice; the button wraps below on a phone. */
	.pick.grow {
		display: grid;
		flex: 1 1 20rem;
		gap: var(--space-1);
	}
	.pick span {
		font-size: var(--text-xs);
		font-weight: 600;
		color: var(--text-secondary);
	}
	select {
		width: 100%;
		height: calc(34px * var(--control-scale));
		padding: 0 var(--space-2);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
</style>
