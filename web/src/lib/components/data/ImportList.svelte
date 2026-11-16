<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Schemas } from '../../api/client';
	import { relative } from '../../time';
	import { counted, STATUS_NAMES } from './format';

	interface Props {
		items: Schemas['Import'][];
	}

	let { items }: Props = $props();
</script>

{#if items.length}
	<ul class="list" aria-label="Imports">
		{#each items as im (im.id)}
			<li>
				<span class="what">
					{im.fileName}
					<span class="sub">
						<span class="status {im.status}">{STATUS_NAMES[im.status]}</span>
						· {im.keepIds ? 'original IDs' : 'new IDs'} · {relative(im.finishedAt ?? im.createdAt)}
						{#if im.status === 'failed'}· {im.error}{/if}
					</span>
				</span>
				{#if im.status === 'ready' && im.report}
					<p class="report">
						{#if im.workspaceId}
							<!-- Opening the app with ?ws switches to the new workspace. -->
							<a href="{resolve('/(app)')}?ws={encodeURIComponent(im.workspaceId)}"
								>Open {im.workspaceName}</a
							>:
						{/if}
						{counted(im.report.restored) || 'nothing'} came in{im.report.files
							? `, with ${im.report.files} ${im.report.files === 1 ? 'file' : 'files'}`
							: ''}.
						{#if Object.values(im.report.dropped).some((n) => n > 0) || im.report.missingFiles}
							Left out: {counted(im.report.dropped)}{im.report.missingFiles
								? `${Object.values(im.report.dropped).some((n) => n > 0) ? ', ' : ''}${im.report.missingFiles} ${im.report.missingFiles === 1 ? 'attachment' : 'attachments'} with no file in the zip`
								: ''}. Rows are left out when they are private to someone else (their My Day,
							reminders, personal views and assignments) or point at something the export did not
							hold.
						{/if}
						{#if im.report.credited}
							{im.report.credited} by other people are credited to you.
						{/if}
					</p>
				{/if}
			</li>
		{/each}
	</ul>
{:else}
	<p class="empty">No imports yet.</p>
{/if}

<style>
	.list {
		margin: 0;
		padding: 0;
		list-style: none;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
	}
	li {
		display: grid;
		gap: var(--space-2);
		padding: var(--pad-y) var(--space-4);
	}
	li + li {
		border-top: 1px solid var(--border);
	}
	.sub {
		display: block;
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.status {
		font-weight: 600;
	}
	.status.failed {
		color: var(--danger);
	}
	.report {
		margin: 0;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.report a {
		color: var(--accent-text);
		font-weight: 550;
	}
	.empty {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
