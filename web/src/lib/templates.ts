// Templates: a project, a task or a wiki page saved
// to make more from. What a template holds and how the app reads one, small
// enough for every page; taking a snapshot and making rows from one are
// in templatePlan.ts, loaded when a template is saved or used. The server
// keeps the snapshot as the template row's data and checks its shape
// (syncer/shapes.go, templateShape).
import type {
	FieldOption,
	FieldType,
	FieldValue,
	Priority,
	StatusCategory,
	ViewDisplay,
	ViewFilter,
	ViewType
} from './sync/entities';

export type TemplateKind = 'project' | 'task' | 'page';

/** At most this many tasks, and this many bytes, in one template. */
export const MAX_TASKS = 500;
/** At most this many pages in a page template, the page itself among them. */
export const MAX_PAGES = 100;
export const MAX_BYTES = 1_000_000;
/** At most this many templates in a workspace. */
export const MAX_TEMPLATES = 200;

export interface TemplateTask {
	/** The ID of the task it was saved from. */
	id: string;
	title: string;
	/** Another task's id in the template, or null. */
	parent: string | null;
	notes: string;
	priority: Priority;
	/** Its status's id in a project template; its category for a task template. */
	status: string | null;
	category: StatusCategory | null;
	/** Whole days from the template's first date. */
	start: number | null;
	due: number | null;
	due_time: string | null;
	recurrence: string | null;
	recurrence_mode: 'schedule' | 'completion';
	/** Its own estimate in minutes, or null; absent from templates saved before estimates. */
	estimate?: number | null;
	labels: { id: string; name: string; color: string }[];
	/** Field ID to value. */
	values: Record<string, FieldValue>;
	checklist: string[];
	assignees?: string[];
	/** The ids of tasks in the template it waits for. */
	waits: string[];
}

export interface TemplateProject {
	/** The ID of the project it was saved from. */
	id: string;
	name: string;
	description: string;
	icon: string;
	color: string;
	statuses: { id: string; name: string; category: StatusCategory }[];
	fields: { id: string; name: string; type: FieldType; options: FieldOption[] }[];
	views: { name: string; type: ViewType; filter: ViewFilter; display: ViewDisplay }[];
	pages: { id: string; parent: string | null; title: string; body: string }[];
	/** Its rules, made switched off in a project from the template. */
	automations?: { name: string; rule: unknown }[];
}

/** A wiki page in a template: the first of a page template's is the page itself. */
export interface TemplatePage {
	/** The ID of the page it was saved from. */
	id: string;
	parent: string | null;
	title: string;
	body: string;
}

export interface TemplateData {
	v: 1;
	tasks: TemplateTask[];
	project?: TemplateProject;
	/** A page template's pages. */
	pages?: TemplatePage[];
}

export interface Template {
	id: string;
	kind: TemplateKind;
	name: string;
	data: TemplateData;
	savedBy: string | null;
	savedAt: string | null;
	sortKey: string;
}

/** A stored template's data as the app can read it, or null. */
export function readTemplate(v: unknown): TemplateData | null {
	if (!v || typeof v !== 'object' || Array.isArray(v)) return null;
	const d = v as Partial<TemplateData>;
	if (d.v !== 1 || !Array.isArray(d.tasks)) return null;
	const tasks = d.tasks.filter(
		(t): t is TemplateTask => !!t && typeof t.id === 'string' && typeof t.title === 'string'
	);
	return {
		v: 1,
		tasks: tasks.map((t) => ({
			...t,
			parent: typeof t.parent === 'string' ? t.parent : null,
			notes: typeof t.notes === 'string' ? t.notes : '',
			labels: Array.isArray(t.labels) ? t.labels : [],
			values: t.values && typeof t.values === 'object' ? t.values : {},
			checklist: Array.isArray(t.checklist) ? t.checklist.filter((c) => typeof c === 'string') : [],
			waits: Array.isArray(t.waits) ? t.waits : [],
			start: typeof t.start === 'number' ? t.start : null,
			due: typeof t.due === 'number' ? t.due : null,
			estimate: typeof t.estimate === 'number' ? t.estimate : null
		})),
		...(Array.isArray(d.pages)
			? {
					pages: d.pages
						.filter((p): p is TemplatePage => !!p && typeof p.id === 'string')
						.map((p) => ({
							id: p.id,
							parent: typeof p.parent === 'string' ? p.parent : null,
							title: typeof p.title === 'string' ? p.title : '',
							body: typeof p.body === 'string' ? p.body : ''
						}))
				}
			: {}),
		...(d.project && typeof d.project === 'object'
			? {
					project: {
						...d.project,
						statuses: Array.isArray(d.project.statuses) ? d.project.statuses : [],
						fields: Array.isArray(d.project.fields) ? d.project.fields : [],
						views: Array.isArray(d.project.views) ? d.project.views : [],
						pages: Array.isArray(d.project.pages) ? d.project.pages : [],
						automations: Array.isArray(d.project.automations) ? d.project.automations : []
					}
				}
			: {})
	};
}
