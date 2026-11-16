<script lang="ts">
	import { page } from '$app/state';

	// The theme editor, full width and outside Settings' layout (which has
	// an h1 of its own and a narrow column). It loads apart: only people
	// who edit a theme pay for it.
	const id = $derived(page.params.id ?? '');
</script>

{#await import('#lib/components/themes/ThemeEditor.svelte') then { default: ThemeEditor }}
	{#key id}<ThemeEditor {id} />{/key}
{/await}
