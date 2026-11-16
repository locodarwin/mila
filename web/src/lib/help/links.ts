// Where a help search result goes. Kept apart from the index and search,
// so the palette can link to help without loading them.
import { resolve } from '$app/paths';

export interface HelpPlace {
	topic: string;
	section?: string;
	/** Set for a keyboard command, whose place is the shortcut reference. */
	command?: string;
}

/** A topic, a section of one, or the shortcut reference's table for a command. */
export function helpHref(p: HelpPlace): string {
	const path = p.command
		? resolve('/(app)/help/shortcuts')
		: resolve('/(app)/help/[topic]', { topic: p.topic });
	return p.section ? `${path}#${p.section}` : path;
}
