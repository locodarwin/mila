<script lang="ts">
	// A running timer's time so far, ticking once a second: "0:42:15", or
	// "0:42" short. Screen readers hear it only when asked, never each tick.
	import { stopwatch } from '../duration';

	let { since, short = false }: { since: string | null; short?: boolean } = $props();
	let now = $state(Date.now());
	$effect(() => {
		const id = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(id);
	});
	const text = $derived.by(() => {
		const s = stopwatch(since ? (now - new Date(since).getTime()) / 1000 : 0);
		return short ? s.slice(0, -3) : s;
	});
</script>

<span class="tabular">{text}</span>
