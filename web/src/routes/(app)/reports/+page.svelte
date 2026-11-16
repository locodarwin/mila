<script lang="ts">
	// The reports and their charts load apart, so no other page pays for them.
	import { canEditViews } from '#lib/roles.js';
	import { session } from '#lib/session.svelte.js';
</script>

<svelte:head><title>Reports · Mila</title></svelte:head>

<header class="page"><h1>Reports</h1></header>
{#await import('#lib/components/reports/Reports.svelte') then { default: Reports }}
	<Reports
		ws={session.workspace?.id ?? ''}
		saveable={!!session.workspace && canEditViews(session.workspace.role)}
	/>
{/await}
