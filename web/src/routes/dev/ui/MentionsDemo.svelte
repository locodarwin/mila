<script lang="ts">
	// A comment's box and what it shows, loaded apart from the style guide so it
	// stays inside its script budget. Nothing here is saved.
	import MentionInput from '#lib/components/task/MentionInput.svelte';
	import MentionText from '#lib/components/task/MentionText.svelte';
	import type { Person } from '#lib/mentions.js';
	import sampleAvatar from './sample-avatar.jpg';

	const people: Person[] = [
		{ userId: 'u-dana', name: 'Dana Whitfield', avatar: sampleAvatar },
		{ userId: 'u-lee', name: 'Lee Park' },
		{ userId: 'u-sam', name: 'Sam Ortega' }
	];
	let comment = $state('Looks good, @Lee Park. ');
	let picked = $state<Person[]>([people[1]!]);
	const task = '0192f0a0-0000-7000-8000-000000000001';
	const gone = '0192f0a0-0000-7000-8000-000000000002';
</script>

<div class="field">
	<MentionInput label="Sample comment" bind:value={comment} bind:picked {people} />
</div>
<p>
	<MentionText
		body="Thanks @[Dana Whitfield](user:u-dana), and @[Lee](user:u-lee) for the photos."
		nameOf={(id) => people.find((p) => p.userId === id)?.name}
	/>
</p>
<p>
	<MentionText
		body={`Do [Order seeds](/?task=${task}) first, then [Hidden plans](/?task=${gone}). More at https://example.com/seeds.`}
		titleOf={(t) => (t.id === task ? 'Order seeds' : undefined)}
	/>
</p>

<style>
	.field {
		max-width: 420px;
		margin-bottom: var(--space-3);
	}
	p {
		margin: var(--space-2) 0;
	}
</style>
