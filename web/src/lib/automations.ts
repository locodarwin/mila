// Automations: what a rule holds, and saying it in words.
// Pure, for the page and its tests. The server reads rules in syncer/automation.go and
// runs them in internal/automations; these lists are the same as its own.
import { matches, type FilterContext } from './filter';
import type { Priority, StatusCategory, Task, ViewFilter } from './sync/entities';

export type StartKind =
	| 'created'
	| 'completed'
	| 'reopened'
	| 'status'
	| 'priority'
	| 'assigned'
	| 'label'
	| 'field'
	| 'comment'
	| 'unblocked'
	| 'email'
	| 'due';

export type ChangeKind =
	| 'status'
	| 'priority'
	| 'due'
	| 'complete'
	| 'assign'
	| 'unassign'
	| 'add_label'
	| 'remove_label'
	| 'move'
	| 'comment'
	| 'subtask'
	| 'notify';

export interface Change {
	do: ChangeKind;
	value?: string;
}

export interface Rule {
	when: { on: StartKind; value?: string };
	config?: { filter?: ViewFilter };
	then: Change[];
}

/** Most changes one rule makes, and rules a workspace holds. */
export const MAX_CHANGES = 10;
export const MAX_AUTOMATIONS = 100;

/** Who a change can name besides a person. */
export const WHO_MADE_TASK = 'creator';
export const WHO_DID_THIS = 'actor';
/** A status by its kind, for a rule across projects with statuses of their own. */
export const KIND = 'kind:';

export type Picks =
	| 'status'
	| 'priority'
	| 'person'
	| 'anyone'
	| 'who'
	| 'label'
	| 'field'
	| 'project'
	| 'soon'
	| 'dueDay'
	| 'text';

export const STARTS: { on: StartKind; label: string; picks?: Picks }[] = [
	{ on: 'created', label: 'A task is created' },
	{ on: 'completed', label: 'A task is completed' },
	{ on: 'reopened', label: 'A task is reopened' },
	{ on: 'status', label: 'A task moves to a status', picks: 'status' },
	{ on: 'priority', label: 'Priority changes to', picks: 'priority' },
	{ on: 'assigned', label: 'Someone is assigned', picks: 'anyone' },
	{ on: 'label', label: 'A label is added', picks: 'label' },
	{ on: 'field', label: 'A custom field changes', picks: 'field' },
	{ on: 'comment', label: 'A comment is added' },
	{ on: 'unblocked', label: 'Its last blocker finishes' },
	{ on: 'email', label: 'It arrives by email' },
	{ on: 'due', label: 'Its due date is near', picks: 'soon' }
];

export const CHANGES: { do: ChangeKind; label: string; picks?: Picks }[] = [
	{ do: 'status', label: 'Set the status to', picks: 'status' },
	{ do: 'priority', label: 'Set the priority to', picks: 'priority' },
	{ do: 'due', label: 'Set the due date to', picks: 'dueDay' },
	{ do: 'complete', label: 'Complete the task' },
	{ do: 'assign', label: 'Assign', picks: 'who' },
	{ do: 'unassign', label: 'Unassign everyone' },
	{ do: 'add_label', label: 'Add the label', picks: 'label' },
	{ do: 'remove_label', label: 'Remove the label', picks: 'label' },
	{ do: 'move', label: 'Move it to', picks: 'project' },
	{ do: 'comment', label: 'Add a comment', picks: 'text' },
	{ do: 'subtask', label: 'Add a subtask', picks: 'text' },
	{ do: 'notify', label: 'Notify', picks: 'who' }
];

export const PRIORITIES: { id: Priority; name: string }[] = [
	{ id: 'urgent', name: 'Urgent' },
	{ id: 'high', name: 'High' },
	{ id: 'medium', name: 'Medium' },
	{ id: 'low', name: 'Low' },
	{ id: 'none', name: 'None' }
];

export const KINDS: { id: StatusCategory; name: string }[] = [
	{ id: 'backlog', name: 'a backlog status' },
	{ id: 'todo', name: 'a to-do status' },
	{ id: 'in-progress', name: 'an in-progress status' },
	{ id: 'done', name: 'a done status' },
	{ id: 'canceled', name: 'a canceled status' }
];

export const SOON: { id: string; name: string }[] = [
	{ id: '0', name: 'is due today' },
	{ id: '1', name: 'is due in 1 day' },
	{ id: '3', name: 'is due in 3 days' },
	{ id: '7', name: 'is due in a week' },
	{ id: 'overdue', name: 'is overdue' }
];

export const DUE_DAYS: { id: string; name: string }[] = [
	{ id: 'today', name: 'today' },
	{ id: 'tomorrow', name: 'tomorrow' },
	{ id: '+3', name: 'in 3 days' },
	{ id: '+7', name: 'in a week' },
	{ id: 'next_monday', name: 'next Monday' }
];

export const WHO: { id: string; name: string }[] = [
	{ id: WHO_MADE_TASK, name: 'whoever made the task' },
	{ id: WHO_DID_THIS, name: 'whoever did this' }
];

/** A rule as the app can read it, or null: from a newer app, or broken. */
export function readRule(v: unknown): Rule | null {
	if (!v || typeof v !== 'object' || Array.isArray(v)) return null;
	const r = v as Partial<Rule>;
	if (!r.when || !STARTS.some((s) => s.on === r.when?.on)) return null;
	if (!Array.isArray(r.then)) return null;
	if (!r.then.every((c) => CHANGES.some((known) => known.do === c?.do))) return null;
	return r as Rule;
}

/** What names in a rule are called now. A name no longer there says so. */
export interface Names {
	status(id: string): string | undefined;
	label(id: string): string | undefined;
	field(id: string): string | undefined;
	person(id: string): string | undefined;
	project(id: string): string | undefined;
}

const named = (list: { id: string; name: string }[], id: string | undefined) =>
	list.find((x) => x.id === id)?.name;

function statusWords(value: string | undefined, names: Names): string {
	if (value?.startsWith(KIND)) {
		return named(KINDS, value.slice(KIND.length)) ?? 'a status no longer here';
	}
	return (value && names.status(value)) || 'a status no longer here';
}

function whoWords(value: string | undefined, names: Names): string {
	return named(WHO, value) ?? ((value && names.person(value)) || 'someone no longer here');
}

/** How a rule's When reads. */
export function whenWords(rule: Rule, names: Names): string {
	const value = rule.when.value;
	switch (rule.when.on) {
		case 'created':
			return 'a task is created';
		case 'completed':
			return 'a task is completed';
		case 'reopened':
			return 'a task is reopened';
		case 'status':
			return `a task moves to ${statusWords(value, names)}`;
		case 'priority':
			return `its priority changes to ${named(PRIORITIES, value) ?? 'one no longer known'}`;
		case 'assigned':
			return value ? `${whoWords(value, names)} is assigned` : 'someone is assigned';
		case 'label':
			return `the label ${(value && names.label(value)) || 'no longer here'} is added`;
		case 'field':
			return `${(value && names.field(value)) || 'a field no longer here'} changes`;
		case 'comment':
			return 'a comment is added';
		case 'unblocked':
			return "a task's last blocker finishes";
		case 'email':
			return 'a task arrives by email';
		case 'due':
			return `a task ${named(SOON, value) ?? 'is near its due date'}`;
	}
}

/** How one change reads. */
export function changeWords(c: Change, names: Names): string {
	switch (c.do) {
		case 'status':
			return `set the status to ${statusWords(c.value, names)}`;
		case 'priority':
			return `set the priority to ${named(PRIORITIES, c.value) ?? 'one no longer known'}`;
		case 'due':
			return `make it due ${named(DUE_DAYS, c.value) ?? 'on a day no longer known'}`;
		case 'complete':
			return 'complete the task';
		case 'assign':
			return `assign ${whoWords(c.value, names)}`;
		case 'unassign':
			return 'unassign everyone';
		case 'add_label':
			return `add the label ${(c.value && names.label(c.value)) || 'no longer here'}`;
		case 'remove_label':
			return `remove the label ${(c.value && names.label(c.value)) || 'no longer here'}`;
		case 'move':
			return `move it to ${(c.value && names.project(c.value)) || 'a project no longer here'}`;
		case 'comment':
			return `add a comment, "${c.value ?? ''}"`;
		case 'subtask':
			return `add a subtask, ${c.value ?? ''}`;
		case 'notify':
			return `notify ${whoWords(c.value, names)}`;
	}
}

const joined = (parts: string[]) =>
	parts.length < 2 ? parts.join('') : `${parts.slice(0, -1).join(', ')} and ${parts.at(-1)}`;

/** The rule as a sentence, for the list. Its Only if counts rather than lists. */
export function sentenceOf(rule: Rule, project: string | null, names: Names, only = 0): string {
	const where = project ? ` in ${names.project(project) ?? 'a project no longer here'}` : '';
	const fits = only ? `, if it fits ${only === 1 ? 'a condition' : `${only} conditions`}` : '';
	const then = rule.then.length
		? joined(rule.then.map((c) => changeWords(c, names)))
		: 'nothing yet';
	return `When ${whenWords(rule, names)}${where}${fits}: ${then}.`;
}

/** What a rule's Only if needs of a task, as the server reads it: done tasks count. */
export function fits(rule: Rule, task: Task, ctx: FilterContext): boolean {
	const filter = rule.config?.filter ?? {};
	return matches(task, { ...filter, showDone: true }, ctx);
}

/** A change's first choice, for one just picked in the builder. */
export function firstOf(
	picks: Picks | undefined,
	choices: { status: string; label: string; field: string; person: string; project: string }
): string | undefined {
	switch (picks) {
		case 'status':
			return choices.status;
		case 'priority':
			return 'high';
		case 'person':
		case 'who':
			return choices.person || WHO_MADE_TASK;
		case 'anyone':
			return '';
		case 'label':
			return choices.label;
		case 'field':
			return choices.field;
		case 'project':
			return choices.project;
		case 'soon':
			return '1';
		case 'dueDay':
			return 'tomorrow';
		case 'text':
			return '';
	}
	return undefined;
}

/** A rule that can be saved: every change has what it needs. */
export function ready(rule: Rule): boolean {
	const pick = (on: Picks | undefined, value: string | undefined) =>
		!on || on === 'anyone' || Boolean(value?.trim());
	return (
		rule.then.length > 0 &&
		rule.then.length <= MAX_CHANGES &&
		pick(STARTS.find((s) => s.on === rule.when.on)?.picks, rule.when.value) &&
		rule.then.every((c) => pick(CHANGES.find((k) => k.do === c.do)?.picks, c.value))
	);
}
