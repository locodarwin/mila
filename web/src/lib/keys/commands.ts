// The commands the keyboard can run, with their default keys: the one
// list every key handler, hint, empty state, the ? sheet, the help pages
// and Settings, Keyboard read. A new shortcut is an entry here, matched
// with keymap.match(scope, e). Plain data, so node tests can import it.
import type { Binding } from './chord';

/** Where a key works: anywhere, or while one kind of thing has focus. */
export type Scope =
	| 'anywhere'
	| 'list'
	| 'table'
	| 'board'
	| 'calendar'
	| 'timeline'
	| 'panel'
	| 'elsewhere'
	| 'system';

export interface Command {
	id: string;
	name: string;
	scope: Scope;
	/** The default keys, which the person can change. */
	keys: readonly Binding[];
	/** Keys that always work, are shown, and cannot be changed. */
	fixed?: readonly Binding[];
	/** A row for help only: its keys are fixed, and its handler stays in its widget. */
	remappable?: false;
	/** Works while typing, with an overlay open, or after another handler took the key. */
	inFields?: true;
	/** Acts on key repeat (moves); everything else ignores a held key. */
	repeat?: true;
	/** One longer sentence for the help pages. */
	help?: string;
}

/**
 * The scopes in the order they are shown, and which others each shares
 * keys with: a key in one is also heard in those it overlaps. List, board
 * and calendar never overlap, since only one thing has focus at a time. A
 * table's rows are tasks in a list too: its cells hear the list's keys
 * after its own, so a key in both does the table's thing in a table.
 */
export const SCOPES = [
	{
		id: 'anywhere',
		name: 'Anywhere',
		overlaps: ['list', 'table', 'board', 'calendar', 'timeline', 'panel']
	},
	{ id: 'list', name: 'On a task in a list', overlaps: ['anywhere'] },
	{ id: 'table', name: 'In a table', overlaps: ['anywhere'] },
	{ id: 'board', name: 'On a board card', overlaps: ['anywhere'] },
	{ id: 'calendar', name: 'On a calendar entry', overlaps: ['anywhere'] },
	{ id: 'timeline', name: 'On a timeline bar', overlaps: ['anywhere'] },
	{ id: 'panel', name: 'In the open task', overlaps: ['anywhere'] },
	{ id: 'elsewhere', name: 'Elsewhere', overlaps: [] },
	// Heard by the desktop app from any program, and by nothing else.
	{ id: 'system', name: 'Anywhere on this computer', overlaps: [] }
] as const satisfies readonly { id: Scope; name: string; overlaps: readonly Scope[] }[];

/** Every command, in the order they are shown. */
export const COMMANDS = [
	{
		id: 'palette',
		name: 'Command palette',
		scope: 'anywhere',
		keys: ['Mod+k'],
		inFields: true,
		help: 'Go anywhere and do anything by typing: tasks, projects, views and commands.'
	},
	{ id: 'search', name: 'Search tasks', scope: 'anywhere', keys: ['/'] },
	{
		id: 'quick-add',
		name: 'Add a task',
		scope: 'anywhere',
		keys: ['q'],
		help: 'Quick add reads dates, times, #projects, @people, !priority and repeats as you type.'
	},
	{ id: 'help', name: 'Show keyboard shortcuts', scope: 'anywhere', keys: ['?', 'Mod+/'] },
	{ id: 'go.inbox', name: 'Go to Inbox', scope: 'anywhere', keys: ['g i'] },
	{ id: 'go.today', name: 'Go to My Day', scope: 'anywhere', keys: ['g d'] },
	{ id: 'go.upcoming', name: 'Go to Upcoming', scope: 'anywhere', keys: ['g u'] },
	{ id: 'go.projects', name: 'Go to Projects', scope: 'anywhere', keys: ['g p'] },
	{ id: 'go.wiki', name: 'Go to the Workspace Wiki', scope: 'anywhere', keys: ['g w'] },
	{ id: 'go.time', name: 'Go to Time', scope: 'anywhere', keys: ['g l'] },
	{ id: 'go.reports', name: 'Go to Reports', scope: 'anywhere', keys: ['g r'] },
	{
		id: 'wiki.tree',
		name: "Show or hide the wiki's pages",
		scope: 'anywhere',
		keys: ['Mod+.'],
		inFields: true,
		help: 'On a wiki page: the tree of pages beside it folds away and comes back.'
	},
	{ id: 'go.help', name: 'Open help', scope: 'anywhere', keys: ['g h'] },
	{ id: 'go.trash', name: 'Open the trash', scope: 'anywhere', keys: [] },
	{ id: 'go.settings', name: 'Open settings', scope: 'anywhere', keys: [] },
	{ id: 'go.preferences', name: 'Open preferences', scope: 'anywhere', keys: [] },
	{ id: 'go.keyboard', name: 'Open keyboard settings', scope: 'anywhere', keys: [] },
	{ id: 'scheme.light', name: 'Switch to light', scope: 'anywhere', keys: [] },
	{ id: 'scheme.dark', name: 'Switch to dark', scope: 'anywhere', keys: [] },
	{ id: 'scheme.system', name: 'Follow the system light or dark', scope: 'anywhere', keys: [] },
	{ id: 'undo', name: 'Undo the last change', scope: 'anywhere', keys: ['Mod+z'] },

	{
		id: 'task.next',
		name: 'Next task',
		scope: 'list',
		keys: ['j'],
		fixed: ['ArrowDown'],
		repeat: true
	},
	{
		id: 'task.prev',
		name: 'Previous task',
		scope: 'list',
		keys: ['k'],
		fixed: ['ArrowUp'],
		repeat: true
	},
	{ id: 'task.open', name: 'Open the task', scope: 'list', keys: [], fixed: ['Enter'] },
	{ id: 'task.complete', name: 'Complete or reopen', scope: 'list', keys: ['x'] },
	{ id: 'task.my-day', name: 'Add to My Day, or take out', scope: 'list', keys: ['t'] },
	{ id: 'task.priority.urgent', name: 'Priority: urgent', scope: 'list', keys: ['1'] },
	{ id: 'task.priority.high', name: 'Priority: high', scope: 'list', keys: ['2'] },
	{ id: 'task.priority.medium', name: 'Priority: medium', scope: 'list', keys: ['3'] },
	{ id: 'task.priority.low', name: 'Priority: low', scope: 'list', keys: ['4'] },
	{ id: 'task.priority.none', name: 'Priority: none', scope: 'list', keys: ['0'] },
	{
		id: 'task.move-up',
		name: 'Move up',
		scope: 'list',
		keys: ['Alt+ArrowUp'],
		repeat: true
	},
	{
		id: 'task.move-down',
		name: 'Move down',
		scope: 'list',
		keys: ['Alt+ArrowDown'],
		repeat: true
	},
	{
		id: 'task.delete',
		name: 'Delete the task',
		scope: 'list',
		keys: ['Delete', 'Backspace'],
		help: 'The toast that follows has Undo, and the trash keeps it for 30 days.'
	},
	{
		id: 'select.toggle',
		name: 'Select the task, or take it out of the selection',
		scope: 'list',
		keys: ['Space'],
		help: 'While anything is selected, the task keys act on all of it.'
	},
	{
		id: 'select.down',
		name: 'Select down too',
		scope: 'list',
		keys: ['Shift+j'],
		fixed: ['Shift+ArrowDown'],
		repeat: true
	},
	{
		id: 'select.up',
		name: 'Select up too',
		scope: 'list',
		keys: ['Shift+k'],
		fixed: ['Shift+ArrowUp'],
		repeat: true
	},
	{
		id: 'select.all',
		name: 'Select every task in the list',
		scope: 'list',
		keys: [],
		fixed: ['Mod+a']
	},
	{ id: 'select.clear', name: 'Clear the selection', scope: 'list', keys: [], fixed: ['Escape'] },

	{
		id: 'table.walk',
		name: 'Go between cells',
		scope: 'table',
		keys: [],
		fixed: ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'],
		remappable: false,
		repeat: true,
		help: "On a task's title, Left folds its subtasks away and Right shows them."
	},
	{
		id: 'table.edit',
		name: 'Edit the cell, sort by the heading, or choose the summary',
		scope: 'table',
		keys: [],
		fixed: ['Enter'],
		remappable: false,
		help: 'On a heading, Shift with Enter adds it to the sort. Escape closes a picker.'
	},
	{ id: 'table.rename', name: 'Rename the task', scope: 'table', keys: ['F2'] },
	{
		id: 'table.column-left',
		name: 'Move the column left',
		scope: 'table',
		keys: ['Alt+ArrowLeft'],
		repeat: true,
		help: 'On a heading. The title column stays first.'
	},
	{
		id: 'table.column-right',
		name: 'Move the column right',
		scope: 'table',
		keys: ['Alt+ArrowRight'],
		repeat: true
	},
	{
		id: 'table.narrower',
		name: 'Make the column narrower',
		scope: 'table',
		keys: ['Alt+Shift+ArrowLeft'],
		repeat: true,
		help: 'On a heading.'
	},
	{
		id: 'table.wider',
		name: 'Make the column wider',
		scope: 'table',
		keys: ['Alt+Shift+ArrowRight'],
		repeat: true
	},

	{
		id: 'board.walk',
		name: 'Go between cards',
		scope: 'board',
		keys: [],
		fixed: ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'],
		remappable: false,
		repeat: true
	},
	{ id: 'board.open', name: 'Open the card', scope: 'board', keys: [], fixed: ['Enter'] },
	{
		id: 'board.move-left',
		name: 'Move the card left',
		scope: 'board',
		keys: ['Alt+ArrowLeft'],
		repeat: true
	},
	{
		id: 'board.move-right',
		name: 'Move the card right',
		scope: 'board',
		keys: ['Alt+ArrowRight'],
		repeat: true
	},
	{
		id: 'board.move-up',
		name: 'Move the card up',
		scope: 'board',
		keys: ['Alt+ArrowUp'],
		repeat: true
	},
	{
		id: 'board.move-down',
		name: 'Move the card down',
		scope: 'board',
		keys: ['Alt+ArrowDown'],
		repeat: true
	},

	{
		id: 'calendar.earlier-day',
		name: 'Move a day earlier',
		scope: 'calendar',
		keys: ['Alt+ArrowLeft'],
		repeat: true
	},
	{
		id: 'calendar.later-day',
		name: 'Move a day later',
		scope: 'calendar',
		keys: ['Alt+ArrowRight'],
		repeat: true
	},
	{
		id: 'calendar.earlier-week',
		name: 'Move a week earlier',
		scope: 'calendar',
		keys: ['Alt+ArrowUp'],
		repeat: true
	},
	{
		id: 'calendar.later-week',
		name: 'Move a week later',
		scope: 'calendar',
		keys: ['Alt+ArrowDown'],
		repeat: true
	},

	{
		id: 'timeline.walk',
		name: 'Go between bars',
		scope: 'timeline',
		keys: [],
		fixed: ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'],
		remappable: false,
		repeat: true,
		help: "Left folds a task's subtasks away and Right shows them."
	},
	{ id: 'timeline.open', name: 'Open the task', scope: 'timeline', keys: [], fixed: ['Enter'] },
	{
		id: 'timeline.earlier',
		name: 'Move earlier',
		scope: 'timeline',
		keys: ['Alt+ArrowLeft'],
		repeat: true,
		help: 'By a day at Day zoom, a week at Week zoom, and a month at Month and Quarter zoom.'
	},
	{
		id: 'timeline.later',
		name: 'Move later',
		scope: 'timeline',
		keys: ['Alt+ArrowRight'],
		repeat: true
	},
	{
		id: 'timeline.due-earlier',
		name: 'Move the due date earlier',
		scope: 'timeline',
		keys: ['Alt+Shift+ArrowLeft'],
		repeat: true
	},
	{
		id: 'timeline.due-later',
		name: 'Move the due date later',
		scope: 'timeline',
		keys: ['Alt+Shift+ArrowRight'],
		repeat: true
	},

	{
		id: 'panel.close',
		name: 'Close the open task or dialog',
		scope: 'panel',
		keys: [],
		fixed: ['Escape'],
		remappable: false
	},

	{
		id: 'comment.send',
		name: 'Send a comment',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Mod+Enter'],
		remappable: false
	},
	{
		id: 'wiki.move',
		name: 'Move a page in the tree',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Alt+ArrowUp', 'Alt+ArrowDown', 'Alt+ArrowLeft', 'Alt+ArrowRight'],
		remappable: false,
		help: 'Up and down among its siblings; right puts it under the page above, left takes it out a level.'
	},
	{
		id: 'doc.toolbar',
		name: 'Go to the formatting bar',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Alt+F10'],
		remappable: false,
		help: 'From notes or a page; Escape goes back to the text.'
	},
	{
		id: 'desktop.quick-add',
		name: 'Quick add, from any program',
		scope: 'system',
		keys: ['Mod+Shift+Space'],
		help: 'In the desktop app. Works while another program is in front.'
	},
	{
		id: 'doc.comment',
		name: 'Comment on the selected words',
		scope: 'elsewhere',
		keys: ['Mod+Alt+m'],
		inFields: true,
		help: 'On a wiki page. Select some words first.'
	},
	{
		id: 'doc.bold',
		name: 'Bold',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Mod+b'],
		remappable: false
	},
	{
		id: 'doc.italic',
		name: 'Italic',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Mod+i'],
		remappable: false
	},
	{
		id: 'doc.strike',
		name: 'Strikethrough',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Mod+Shift+s'],
		remappable: false
	},
	{
		id: 'doc.code',
		name: 'Code',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Mod+e'],
		remappable: false
	},
	{
		id: 'sidebar.move',
		name: 'Move a project or sidebar item',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Alt+ArrowUp', 'Alt+ArrowDown'],
		remappable: false
	},
	{
		id: 'edge.resize',
		name: 'Resize the sidebar or the task panel',
		scope: 'elsewhere',
		keys: [],
		fixed: ['ArrowLeft', 'ArrowRight', 'Shift+ArrowLeft', 'Shift+ArrowRight', 'Home', 'End'],
		remappable: false,
		help: 'On the edge between them; Shift takes bigger steps, Home and End go narrowest and widest.'
	},
	{
		id: 'quickadd.add',
		name: 'Add the task in quick add',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Enter'],
		remappable: false
	},
	{
		id: 'avatar.move',
		name: 'Move the picture you are cropping',
		scope: 'elsewhere',
		keys: [],
		fixed: [
			'ArrowLeft',
			'ArrowRight',
			'ArrowUp',
			'ArrowDown',
			'Shift+ArrowLeft',
			'Shift+ArrowRight',
			'Shift+ArrowUp',
			'Shift+ArrowDown'
		],
		remappable: false,
		help: 'On the picture in the cropper; Shift takes bigger steps.'
	},
	{
		id: 'avatar.zoom',
		name: 'Zoom the picture you are cropping',
		scope: 'elsewhere',
		keys: [],
		fixed: ['+', '-'],
		remappable: false,
		help: 'On the picture in the cropper: plus zooms in and minus zooms out, about the middle.'
	},
	{
		id: 'theme.undo',
		name: 'Undo a change in the theme editor',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Mod+z'],
		remappable: false
	},
	{
		id: 'theme.redo',
		name: 'Redo a change in the theme editor',
		scope: 'elsewhere',
		keys: [],
		fixed: ['Mod+Shift+z', 'Mod+y'],
		remappable: false
	},
	{
		id: 'menu.context',
		name: "Open the menu of what has focus: a task, view or project's actions",
		scope: 'elsewhere',
		keys: [],
		fixed: ['Shift+F10', 'ContextMenu'],
		remappable: false
	},
	{
		id: 'menu.choose',
		name: 'Choose in a menu or list of results',
		scope: 'elsewhere',
		keys: [],
		fixed: ['ArrowUp', 'ArrowDown'],
		remappable: false
	}
] as const satisfies readonly Command[];

export type CommandId = (typeof COMMANDS)[number]['id'];

const byId = new Map<string, Command>(COMMANDS.map((c) => [c.id, c]));

/** A command by its id. */
export function command(id: CommandId): Command {
	return byId.get(id)!;
}

/** Whether a string names a command, for ids read from storage. */
export function isCommand(id: string): id is CommandId {
	return byId.has(id);
}

/**
 * The scope of what has focus, for the ? sheet: a board card, a calendar
 * entry, something in the open task, or else a task in a list.
 */
export function scopeOf(el: Element | null): Scope {
	if (el?.closest('[data-card]')) return 'board';
	if (el?.closest('[data-table]')) return 'table';
	if (el?.closest('[data-chip]')) return 'calendar';
	if (el?.closest('[data-timeline]')) return 'timeline';
	if (el?.closest('#task-panel')) return 'panel';
	return 'list';
}

/** A scope's entry in SCOPES. */
export function scope(id: Scope) {
	return SCOPES.find((s) => s.id === id)!;
}

/** Whether a key in one scope is also heard in the other. */
export function overlapping(a: Scope, b: Scope): boolean {
	return (scope(a).overlaps as readonly Scope[]).includes(b);
}
