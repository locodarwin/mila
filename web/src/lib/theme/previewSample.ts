// A tiny workspace for the theme preview's card: one task with labels, a
// checklist and a due date, held by engine.sample in memory only, so the
// real card component draws it and nothing is ever saved or sent. Each
// preview on the page borrows it; the last one to go drops it.
import { DEFAULTS, type Entity } from '../sync/entities';
import { engine } from '../sync/engine.svelte';
import type { Row } from '../sync/replica.svelte';

export const PREVIEW_WS = 'theme-preview';
export const PREVIEW_TASK = 't-preview';

const row = (entity: Entity, id: string, values: Record<string, unknown>): Row => ({
	entity,
	id,
	values: { ...DEFAULTS[entity], ...values },
	clocks: {}
});

function day(offset: number): string {
	const d = new Date();
	d.setDate(d.getDate() + offset);
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function rows(): Row[] {
	return [
		row('project', 'p-preview', {
			name: 'Home',
			icon: 'lucide:house',
			color: 'blue',
			sort_key: 'a0'
		}),
		row('status', 's-preview', {
			project_id: 'p-preview',
			name: 'Doing',
			category: 'in-progress',
			sort_key: 'a0'
		}),
		row('task', PREVIEW_TASK, {
			project_id: 'p-preview',
			status_id: 's-preview',
			title: 'Paint the hallway',
			priority: 'high',
			due_date: day(2),
			sort_key: 'a0'
		}),
		row('label', 'l-weekend', { name: 'Weekend', color: 'teal', sort_key: 'a0' }),
		row('label', 'l-errand', { name: 'Errand', color: 'amber', sort_key: 'a1' }),
		row('task_label', 'tl-a', { task_id: PREVIEW_TASK, label_id: 'l-weekend' }),
		row('task_label', 'tl-b', { task_id: PREVIEW_TASK, label_id: 'l-errand' }),
		row('checklist_item', 'c-a', {
			task_id: PREVIEW_TASK,
			text: 'Buy paint',
			done: true,
			sort_key: 'a0'
		}),
		row('checklist_item', 'c-b', { task_id: PREVIEW_TASK, text: 'Tape the edges', sort_key: 'a1' })
	];
}

let users = 0;

/** Borrows the sample workspace; call the function it returns when done with it. */
export function borrowSample(): () => void {
	if (users++ === 0 || !engine.replica(PREVIEW_WS)) engine.sample(PREVIEW_WS, rows());
	let returned = false;
	return () => {
		if (returned) return;
		returned = true;
		if (--users === 0) engine.dropSample(PREVIEW_WS);
	};
}
