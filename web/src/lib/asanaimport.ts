// Asana import: how Asana's CSV export of a project writes its values (a
// Dialect for the CSV import's column screen and dry run, csvimport.ts),
// and which of its columns go where, matched before the person sees them.
// Asana's file has no comments or attachments. The page is
// routes/(app)/settings/w/[ws]/data/apps.
import { ANY_CSV, type Dialect, type Target } from './csvimport';

/** Asana's headings, squashed, and what they feed; any other column is left out. */
const HEADERS: Record<string, Target> = {
	taskid: 'key',
	createdat: 'made',
	completedat: 'completed',
	lastmodified: 'skip',
	name: 'title',
	sectioncolumn: 'status',
	// The address finds the person more surely than the name.
	assignee: 'skip',
	assigneeemail: 'assignees',
	startdate: 'start',
	duedate: 'due',
	tags: 'labels',
	notes: 'notes',
	projects: 'project',
	parenttask: 'parent',
	blockedbydependencies: 'blockedBy',
	blockingdependencies: 'blocking',
	priority: 'priority'
};

/**
 * A task's address in Asana from its ID. Asana's own links take a project
 * and a task; 0 for the project opens the task wherever it is.
 */
export const asanaLink = (id: string) =>
	/^\d{1,30}$/.test(id) ? `https://app.asana.com/0/0/${id}` : null;

/** Asana's CSV export of a project. */
export const ASANA: Dialect = {
	...ANY_CSV,
	app: 'Asana',
	headers: HEADERS,
	// A project's first section, when nobody named it.
	blank: (s) => /^untitled section$/i.test(s),
	// A task in several projects names them all, with commas between.
	projects: (s) =>
		s
			.split(',')
			.map((x) => x.trim())
			.filter(Boolean),
	listStrangers: true,
	linkName: 'Asana task',
	linkOf: asanaLink,
	behind: "Comments and attachments are not in Asana's file, so they stay in Asana"
};

/** Whether a file's headings are Asana's: its own names for a task's ID, title and section. */
export const isAsana = (headers: string[]) => {
	const keys = new Set(headers.map((h) => h.toLowerCase().replace(/[^a-z0-9]/g, '')));
	return keys.has('taskid') && keys.has('name') && keys.has('sectioncolumn');
};
