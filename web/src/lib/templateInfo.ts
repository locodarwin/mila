// What a template holds, in a line, and whether one is too large to keep
// (docs/PLAN.md section 3): for the dialogs and Settings, Templates,
// which load apart. templates.ts, which every page reads, stays small.
import { MAX_BYTES, MAX_PAGES, MAX_TASKS, type Template, type TemplateData } from './templates';

/** The size of a snapshot as stored, and what is wrong with it, if anything. */
export function tooBig(data: TemplateData): string | null {
	if (data.tasks.length > MAX_TASKS)
		return `This has ${data.tasks.length} open tasks; a template holds ${MAX_TASKS}. Archive or finish some first.`;
	if ((data.pages?.length ?? 0) > MAX_PAGES)
		return `This is ${data.pages?.length} pages; a template holds ${MAX_PAGES}. Save fewer subpages.`;
	const bytes = new TextEncoder().encode(JSON.stringify(data)).length;
	if (bytes > MAX_BYTES)
		return `This is ${Math.ceil(bytes / 1000)} KB; a template holds 1,000 KB. Shorten some notes or pages first.`;
	return null;
}

/** "14 tasks, 3 pages, 5 statuses" or "4 subtasks, checklist of 3". */
export function contents(t: Pick<Template, 'kind' | 'data'>): string {
	const n = (k: number, one: string, many = `${one}s`) => `${k} ${k === 1 ? one : many}`;
	if (t.kind === 'project') {
		const p = t.data.project;
		return [
			n(t.data.tasks.length, 'task'),
			n(p?.pages.length ?? 0, 'page'),
			n(p?.statuses.length ?? 0, 'status', 'statuses')
		].join(', ');
	}
	if (t.kind === 'page') {
		const under = (t.data.pages?.length ?? 1) - 1;
		return under ? `with ${n(under, 'subpage')}` : 'one page';
	}
	const root = t.data.tasks[0];
	const subs = t.data.tasks.length - 1;
	const parts = [subs ? n(subs, 'subtask') : 'no subtasks'];
	if (root?.checklist.length) parts.push(`checklist of ${root.checklist.length}`);
	return parts.join(', ');
}

/** The last day a project template's dates reach, in days from its first. */
export function lastDay(data: TemplateData): number | null {
	let last: number | null = null;
	for (const t of data.tasks)
		for (const d of [t.start, t.due]) if (d !== null && (last === null || d > last)) last = d;
	return last;
}
