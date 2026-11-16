<script lang="ts">
	import { RAIL_ICONS, type RailIconName } from '#lib/railIcons.js';

	// A Settings page's icon, drawn from data (railIcons.ts); decoration only.
	let { name, size = 16 }: { name: RailIconName; size?: number } = $props();
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	<path d={RAIL_ICONS[name]} />
</svg>
