<script lang="ts">
	import AppearanceControls from '#lib/components/AppearanceControls.svelte';
	// The theme manager loads apart, so Settings stays small for everyone else.
</script>

<svelte:head><title>Appearance · Mila</title></svelte:head>

<section>
	<h2>Appearance</h2>
	<AppearanceControls />
</section>

{#await import('#lib/components/themes/ThemeManager.svelte') then { default: ThemeManager }}
	<ThemeManager />
{/await}
