<script lang="ts">
	// People's pictures, loaded apart from the style guide so it stays
	// inside its script budget. A picture chosen here stays here: an
	// object URL, never sent.
	import Button from '#lib/components/Button.svelte';
	import ProfilePicture from '#lib/components/ProfilePicture.svelte';
	import Avatar from '#lib/components/task/Avatar.svelte';
	import sampleAvatar from './sample-avatar.jpg';

	let demoPicture = $state<string | null>(sampleAvatar);
	let demoCropping = $state<Blob | null>(null);
	const brokenAvatar = '/api/v1/avatars/' + '0'.repeat(32);
</script>

<div class="wrap">
	{#each [20, 26, 28, 96] as size (size)}<Avatar name="Dana Whitfield" {size} />{/each}
</div>
<div class="wrap">
	{#each [20, 26, 28, 96] as size (size)}<Avatar
			name="Dana Whitfield"
			src={sampleAvatar}
			{size}
		/>{/each}
	<Avatar name="Lee Park" src={brokenAvatar} size={28} label="Lee Park, picture missing" />
</div>
<ProfilePicture
	name="Dana Whitfield"
	picture={demoPicture}
	save={async (png) => {
		if (demoPicture?.startsWith('blob:')) URL.revokeObjectURL(demoPicture);
		demoPicture = URL.createObjectURL(png);
	}}
	remove={async () => {
		demoPicture = null;
	}}
/>
<div>
	<Button onclick={async () => (demoCropping = await (await fetch(sampleAvatar)).blob())}
		>Crop the sample</Button
	>
</div>
{#if demoCropping}
	<!-- Loaded when wanted, as Settings loads it. -->
	{#await import('#lib/components/AvatarCrop.svelte') then { default: AvatarCrop }}
		<AvatarCrop
			file={demoCropping}
			onsave={async (png) => {
				if (demoPicture?.startsWith('blob:')) URL.revokeObjectURL(demoPicture);
				demoPicture = URL.createObjectURL(png);
			}}
			onclose={() => (demoCropping = null)}
		/>
	{/await}
{/if}

<style>
	.wrap {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3);
		align-items: center;
	}
</style>
