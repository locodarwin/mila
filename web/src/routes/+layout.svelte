<script lang="ts">
	import '@fontsource-variable/inter';
	import '@fontsource-variable/source-serif-4';
	import '@fontsource-variable/nunito';
	import '@fontsource-variable/jetbrains-mono';
	import '#lib/styles/tokens.css';
	import '#lib/styles/base.css';
	import Announcer from '#lib/components/Announcer.svelte';
	import Toaster from '#lib/components/Toaster.svelte';
	import { onlyMilaMenus } from '#lib/desktop.js';
	import { watchHover } from '#lib/hoverText.js';
	import { markReady } from '#lib/perf.js';
	import { appearance } from '#lib/theme/appearance.svelte.js';
	import { watchUpdates } from '#lib/update.svelte.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	// Re-applies whenever a setting or the OS scheme changes.
	$effect(() => {
		void appearance.current;
		void appearance.scheme;
		void appearance.custom;
		appearance.apply();
	});

	$effect(() => watchHover());

	// In the desktop app, right-click opens only Mila's own menus (and text's).
	$effect(onlyMilaMenus);

	// A new build deployed while the app is open offers a reload.
	$effect(() => watchUpdates());

	// Children mount first, so the first page is in the document here,
	// in place of the frame index.html painted while the scripts loaded.
	$effect(() => {
		document.getElementById('mila-frame')?.remove();
		markReady();
	});
</script>

{@render children()}
<Toaster />
<Announcer />
