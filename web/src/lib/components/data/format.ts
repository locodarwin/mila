// Words for exports and imports, shared by their lists.
import type { Schemas } from '../../api/client';

export const FORMAT_NAMES: Record<Schemas['ExportFormat'], string> = {
	json: 'JSON',
	csv: 'CSV',
	markdown: 'Markdown'
};

export const STATUS_NAMES: Record<Schemas['JobStatus'], string> = {
	queued: 'Waiting to start',
	running: 'Under way',
	ready: 'Ready',
	failed: 'Failed'
};

/** A size in bytes as people say it. */
export function fileSize(n: number): string {
	if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)} GB`;
	if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
	if (n >= 1 << 10) return `${Math.round(n / (1 << 10))} KB`;
	return `${n} bytes`;
}

const ENTITY_NAMES: Record<string, [string, string]> = {
	project: ['project', 'projects'],
	status: ['status', 'statuses'],
	label: ['label', 'labels'],
	custom_field: ['field', 'fields'],
	saved_view: ['saved view', 'saved views'],
	task: ['task', 'tasks'],
	task_label: ['label on a task', 'labels on tasks'],
	task_assignee: ['assignment', 'assignments'],
	checklist_item: ['checklist item', 'checklist items'],
	comment: ['comment', 'comments'],
	attachment: ['attachment', 'attachments'],
	my_day_entry: ['My Day entry', 'My Day entries'],
	reminder: ['reminder', 'reminders'],
	custom_field_value: ['field value', 'field values'],
	unknown: ['unknown row', 'unknown rows']
};

/** "3 tasks, 1 project": counts by entity, biggest first. */
export function counted(counts: Record<string, number>): string {
	return Object.entries(counts)
		.filter(([, n]) => n > 0)
		.sort((a, b) => b[1] - a[1])
		.map(([entity, n]) => {
			const [one, many] = ENTITY_NAMES[entity] ?? [entity, entity];
			return `${n} ${n === 1 ? one : many}`;
		})
		.join(', ');
}
