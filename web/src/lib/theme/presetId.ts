// The ids of the themes Mila ships made from Omarchy's (omarchy.ts makes
// them, presets.svelte.ts serves them). They share a start, so any part of
// the app can tell one from a person's own or a workspace's without
// loading the list, and never asks the server for one.

export const PRESET_PREFIX = '0a4a7c4e-0000-4000-8000-';

/** Whether a theme id is one of the themes Mila ships. */
export function isPreset(id: string | null | undefined): id is string {
	return typeof id === 'string' && id.startsWith(PRESET_PREFIX);
}
