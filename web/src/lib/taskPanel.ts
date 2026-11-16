// The words on the task panel's closed lines (the summary layout of the
// taskPanel preference, PanelSection.svelte): each says what its section holds.

/** How many there are of something, or that there are none. */
export function countOf(n: number, one: string): string {
	return n ? `${n} ${one}${n === 1 ? '' : 's'}` : 'None yet';
}

/** Items in a list that can be finished: subtasks, a checklist. */
export function progressOf(done: number, total: number): string {
	return total ? `${done} of ${total} done` : 'None yet';
}

/** What a task's dates, repeat and reminders come to, already in words. */
export function scheduleOf(parts: {
	due?: string | null;
	start?: string | null;
	repeat?: string | null;
	reminders: number;
}): string {
	const words: string[] = [];
	if (parts.due) words.push(`Due ${parts.due}`);
	else if (parts.start) words.push(`Starts ${parts.start}`);
	if (parts.repeat) words.push(parts.repeat);
	if (parts.reminders) words.push(countOf(parts.reminders, 'reminder'));
	return words.join(' · ') || 'Nothing set';
}

/** Custom fields with a value, out of those a task could have. */
export function fieldsOf(filled: number, total: number): string {
	if (!filled) return total === 1 ? 'Not filled' : `${total} fields, none filled`;
	return `${filled} of ${total} filled`;
}

/** What a task waits for, counting only what is not done, and what waits for it. */
export function dependenciesOf(blockedBy: number, blocking: number): string {
	const words: string[] = [];
	if (blockedBy) words.push(`Blocked by ${countOf(blockedBy, 'task')}`);
	if (blocking) words.push(`Blocking ${countOf(blocking, 'task')}`);
	return words.join(' · ') || 'Not blocked';
}

/** Time logged against the estimate, both already in words. */
export function timeOf(logged: string | null, estimate: string | null): string {
	if (logged && estimate) return `${logged} of ${estimate}`;
	if (logged) return `${logged} logged`;
	if (estimate) return `${estimate} estimated`;
	return 'No estimate';
}

/** The first line of a task's notes with its Markdown marks taken off. */
export function notesOf(markdown: string | null | undefined): string {
	for (const raw of (markdown ?? '').split('\n')) {
		const line = raw
			.replace(/^\s*(?:#{1,6}\s+|>\s*(?:\[![A-Z]+\]\s*)?|[-*+]\s+(?:\[[ xX]\]\s+)?|\d+[.)]\s+)/, '')
			.replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
			.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
			.replace(/[*_`~]/g, '')
			.trim();
		if (line && !/^(?:\||:?-{3,})/.test(line)) return line;
	}
	return 'Empty';
}
