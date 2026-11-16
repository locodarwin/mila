// What the app does to automations (docs/PLAN.md section 3): the rows are synced like
// any other, and the server runs them. Saving what a rule does makes it the saver's,
// since a rule runs as whoever last changed it.
import type { Rule } from '../automations';
import { uuid } from '../uuid';
import { engine } from './engine.svelte';
import { keyBetween } from './sortkey';

const now = () => new Date().toISOString();

/** Makes a rule, turned on, and says its ID. */
export async function makeAutomation(
	ws: string,
	me: string,
	fields: { projectId: string | null; name: string; rule: Rule },
	lastKey: string | null
): Promise<string> {
	const id = uuid();
	await engine.mutate(ws, 'automation', id, {
		project_id: fields.projectId,
		name: fields.name,
		enabled: true,
		enabled_at: now(),
		rule: fields.rule,
		made_by: me,
		paused_why: null,
		sort_key: keyBetween(lastKey, null),
		deleted_at: null
	});
	return id;
}

/** Saves what a rule does, which also makes it yours and lets it run again. */
export function saveAutomation(
	ws: string,
	id: string,
	me: string,
	fields: { name: string; rule: Rule }
) {
	return engine.mutate(ws, 'automation', id, {
		name: fields.name,
		rule: fields.rule,
		made_by: me,
		paused_why: null
	});
}

/**
 * Turns a rule off, or on from now on. Turning one on saves it as it's shown here, so
 * what runs, as whoever turned it on, is what they were looking at.
 */
export function setAutomationOn(ws: string, id: string, me: string, rule: unknown, on: boolean) {
	return engine.mutate(
		ws,
		'automation',
		id,
		on
			? { enabled: true, enabled_at: now(), paused_why: null, rule, made_by: me }
			: { enabled: false }
	);
}

export function deleteAutomation(ws: string, id: string, deleted: boolean) {
	return engine.mutate(ws, 'automation', id, { deleted_at: deleted ? now() : null });
}
