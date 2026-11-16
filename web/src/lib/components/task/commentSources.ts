// What a comment box offers after / and how a reference shows, whether the comment
// is on a task or on a wiki page.
import { linkChoices } from '../../docs/mentions';
import { headingsIn } from '../../mentions';
import { engine } from '../../sync/engine.svelte';
import { pageTitle } from '../../wiki';

/** The tasks and pages a comment can refer to; taskId leaves out the task it is on. */
export function referenceSource(ws: string, taskId = '') {
	const replica = () => engine.replica(ws);
	const projectOf = (id: string | null) => (id ? replica()?.project(id) : undefined);
	return {
		tasks: (query: string) =>
			linkChoices(ws, query, [], replica()?.allTasks() ?? [], projectOf, taskId),
		pages: (query: string) =>
			linkChoices(ws, query, replica()?.allPages() ?? [], [], projectOf, ''),
		sections: (pageId: string) => headingsIn(replica()?.page(pageId)?.body)
	};
}

/** What a reference shows now, or undefined for what this person cannot open. */
export function referenceTitle(ws: string) {
	return (target: { kind: 'page' | 'task'; id: string }): string | undefined => {
		const replica = engine.replica(ws);
		if (target.kind === 'task') return replica?.task(target.id)?.title;
		const p = replica?.page(target.id);
		return p && pageTitle(ws, p, p.projectId ? replica?.project(p.projectId) : undefined);
	};
}
