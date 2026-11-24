// The help topics: their titles, summaries, sections and the words people
// search for. Each topic's text is a Svelte component in ./topics, loaded
// only when it is read, so none of it costs a page that shows no help.
// help/index.spec.ts holds the sections here to the headings in the files.
import type { Component } from 'svelte';

export interface Section {
	/** The heading's id in the topic, and the link's #fragment. */
	id: string;
	title: string;
	keywords?: string[];
}

export interface Topic {
	slug: string;
	title: string;
	summary: string;
	keywords: string[];
	sections: Section[];
	related?: string[];
}

export const HELP: Topic[] = [
	{
		slug: 'quick-add',
		title: 'Quick add',
		summary: 'Type a task with its date, project, priority, people, labels and repeat in one line.',
		keywords: ['add', 'new task', 'syntax', 'natural language', 'type'],
		sections: [
			{ id: 'project', title: 'Projects', keywords: ['#', 'hash', 'tag'] },
			{ id: 'priority', title: 'Priority', keywords: ['!', 'urgent', 'high', 'low'] },
			{ id: 'people', title: 'People', keywords: ['@', 'assign', 'assignee'] },
			{ id: 'labels', title: 'Labels', keywords: ['+', 'label', 'tag'] },
			{ id: 'dates', title: 'Dates and times', keywords: ['due', 'tomorrow', 'today', 'time'] },
			{ id: 'repeats', title: 'Repeats', keywords: ['every', 'repeat', 'recurring', 'daily'] },
			{ id: 'reminders', title: 'Reminders', keywords: ['remind', 'alert', 'before'] },
			{ id: 'where', title: 'Where the task goes', keywords: ['inbox', 'my day', 'enter'] }
		],
		related: ['recurrence', 'reminders-and-notifications', 'my-day-and-upcoming']
	},
	{
		slug: 'views-and-filters',
		title: 'Views and filters',
		summary:
			'Lists, boards, calendars and tables, statuses, and saved views with their filters, sort and grouping.',
		keywords: [
			'board',
			'kanban',
			'calendar',
			'list',
			'table',
			'filter',
			'view',
			'status',
			'sort',
			'group'
		],
		sections: [
			{
				id: 'layouts',
				title: 'List, board, calendar, table and timeline',
				keywords: ['kanban', 'agenda', 'gantt']
			},
			{
				id: 'tables',
				title: 'Tables',
				keywords: [
					'table',
					'spreadsheet',
					'columns',
					'resize',
					'summary',
					'sum',
					'average',
					'subtasks',
					'rename'
				]
			},
			{ id: 'statuses', title: 'Statuses', keywords: ['column', 'done', 'canceled', 'backlog'] },
			{ id: 'saved-views', title: 'Saved views', keywords: ['save', 'share', 'personal', 'guest'] },
			{
				id: 'display',
				title: 'Sort, group and show',
				keywords: [
					'sort',
					'order',
					'group',
					'group by',
					'columns',
					'board columns',
					'fields',
					'display',
					'show',
					'hide'
				]
			},
			{
				id: 'filters',
				title: 'Filters',
				keywords: ['filter', 'due', 'assignee', 'find', 'custom field', 'condition']
			}
		],
		related: ['my-day-and-upcoming']
	},
	{
		slug: 'writing',
		title: 'Writing in notes and pages',
		summary:
			'Formatting, checklists, tables, images, diagrams, drawings, boxes and Markdown, in task notes and wiki pages.',
		keywords: [
			'notes',
			'editor',
			'format',
			'formatting',
			'markdown',
			'bold',
			'table',
			'checklist',
			'image',
			'picture',
			'code',
			'callout',
			'box'
		],
		sections: [
			{
				id: 'formatting',
				title: 'Formatting',
				keywords: ['bold', 'italic', 'link', 'heading', 'bar', 'highlight', 'font', 'size']
			},
			{
				id: 'toolbar',
				title: 'The toolbar in a page',
				keywords: ['toolbar', 'insert', 'buttons', 'diagram', 'svg', 'hide']
			},
			{
				id: 'blocks',
				title: 'Lists, tables and more',
				keywords: [
					'slash',
					'checklist',
					'table',
					'code',
					'quote',
					'note',
					'tip',
					'warning',
					'divider'
				]
			},
			{
				id: 'columns',
				title: 'Columns and foldable sections',
				keywords: ['columns', 'side by side', 'fold', 'toggle', 'collapse', 'details', 'expand']
			},
			{
				id: 'views',
				title: 'A view in a page',
				keywords: ['view', 'saved view', 'embed', 'tasks', 'board', 'table', 'show as']
			},
			{
				id: 'reports',
				title: 'A report in a page',
				keywords: ['report', 'chart', 'graph', 'burnup', 'embed']
			},
			{
				id: 'diagrams',
				title: 'Diagrams',
				keywords: [
					'diagram',
					'mermaid',
					'flowchart',
					'sequence',
					'org chart',
					'architecture',
					'mind map',
					'gantt',
					'pie'
				]
			},
			{
				id: 'drawings',
				title: 'Drawings',
				keywords: [
					'drawing',
					'draw',
					'sketch',
					'whiteboard',
					'shapes',
					'arrows',
					'flowchart',
					'org chart',
					'pen'
				]
			},
			{
				id: 'images',
				title: 'Images',
				keywords: ['picture', 'photo', 'upload', 'attachment', 'caption', 'size', 'float']
			},
			{
				id: 'type',
				title: "A page's font and size",
				keywords: ['font', 'size', 'typeface', 'serif', 'theme', 'pages']
			},
			{
				id: 'links',
				title: 'Links to pages and tasks',
				keywords: ['link', '[[', 'backlinks', 'linked from', 'mention', 'rename']
			},
			{ id: 'markdown', title: 'Markdown', keywords: ['source', 'export', 'api', 'html'] },
			{ id: 'wide', title: 'Room to write', keywords: ['wide', 'expand', 'full'] }
		],
		related: ['wiki', 'offline-and-sync']
	},
	{
		slug: 'templates',
		title: 'Templates',
		summary: 'Save a project, a task or a wiki page as a template, and start new ones from it.',
		keywords: ['template', 'templates', 'copy', 'duplicate', 'reuse', 'checklist', 'onboarding'],
		sections: [
			{ id: 'saving', title: 'Saving a template', keywords: ['save', 'replace', 'assignees'] },
			{
				id: 'projects',
				title: 'A project from a template',
				keywords: ['new project', 'start date', 'dates']
			},
			{ id: 'tasks', title: 'A task from a template', keywords: ['new task', 'menu', 'palette'] },
			{
				id: 'pages',
				title: 'A wiki page from a template',
				keywords: ['wiki', 'page', 'subpages', 'today', 'date', 'meeting notes']
			},
			{
				id: 'managing',
				title: 'Managing templates',
				keywords: ['rename', 'delete', 'change', 'settings']
			}
		],
		related: ['views-and-filters', 'timeline-and-dependencies']
	},
	{
		slug: 'time-tracking',
		title: 'Time tracking',
		summary:
			'Estimates, logging time and the timer, on tasks and on projects, and the Time page that adds it up.',
		keywords: [
			'time',
			'estimate',
			'estimates',
			'timer',
			'stopwatch',
			'hours',
			'log time',
			'timesheet',
			'report'
		],
		sections: [
			{ id: 'estimates', title: 'Estimates', keywords: ['estimate', 'hours', 'subtasks'] },
			{ id: 'logging', title: 'Logging time', keywords: ['log', 'entry', 'edit', 'delete'] },
			{ id: 'timer', title: 'The timer', keywords: ['start', 'stop', 'running', 'stopwatch'] },
			{
				id: 'project-time',
				title: 'Time on a project itself',
				keywords: ['project', 'meeting', 'planning', 'travel', 'not on a task', 'time tab']
			},
			{ id: 'views', title: 'Time in views', keywords: ['table', 'filter', 'sort', 'board'] },
			{ id: 'report', title: 'The Time page', keywords: ['report', 'export', 'csv', 'week'] }
		],
		related: ['views-and-filters', 'timeline-and-dependencies']
	},
	{
		slug: 'reports',
		title: 'Reports',
		summary:
			'Project status, workload, and throughput and cycle time, for one project or all, with charts, tables and CSV.',
		keywords: [
			'report',
			'reports',
			'chart',
			'charts',
			'burnup',
			'workload',
			'throughput',
			'cycle time',
			'overdue',
			'csv',
			'progress',
			'project',
			'pie',
			'priority',
			'cumulative flow'
		],
		sections: [
			{
				id: 'page',
				title: 'The Reports page',
				keywords: ['offline', 'guest', 'trash', 'reopened']
			},
			{ id: 'project', title: 'One project', keywords: ['project', 'one', 'link'] },
			{ id: 'filter', title: 'The filter and the period', keywords: ['filter', 'period', 'weeks'] },
			{
				id: 'status',
				title: 'Project status',
				keywords: ['burnup', 'overdue', 'attention', 'priority', 'label', 'share']
			},
			{ id: 'workload', title: 'Workload', keywords: ['person', 'hours', 'capacity', 'due'] },
			{
				id: 'flow',
				title: 'Throughput and cycle time',
				keywords: ['median', 'made', 'done', 'cumulative flow', 'scatter']
			},
			{
				id: 'saved',
				title: 'Saved reports',
				keywords: ['save as view', 'saved report', 'share', 'view']
			},
			{ id: 'charts', title: 'Charts, tables and CSV', keywords: ['table', 'csv', 'download'] }
		],
		related: ['views-and-filters', 'time-tracking']
	},
	{
		slug: 'timeline-and-dependencies',
		title: 'Timeline and dependencies',
		summary:
			'Tasks that wait for others: blocked tasks, what holds them up, and the timeline that shows it.',
		keywords: [
			'dependency',
			'dependencies',
			'blocked',
			'blocking',
			'waits',
			'timeline',
			'gantt',
			'schedule'
		],
		sections: [
			{
				id: 'dependencies',
				title: 'Tasks that wait for others',
				keywords: ['blocked by', 'blocking', 'add', 'remove', 'loop']
			},
			{ id: 'blocked', title: 'Blocked tasks', keywords: ['blocked', 'mark', 'filter', 'red'] },
			{
				id: 'conflicts',
				title: 'Dates that clash',
				keywords: ['conflict', 'starts before', 'late']
			},
			{
				id: 'unblocked',
				title: 'When a task can start',
				keywords: ['notification', 'done', 'finished', 'ready']
			},
			{ id: 'timeline', title: 'The timeline', keywords: ['gantt', 'bar', 'zoom', 'today'] },
			{
				id: 'moving',
				title: 'Moving tasks on the timeline',
				keywords: ['drag', 'dates', 'stretch', 'keyboard', 'move later']
			},
			{
				id: 'linking',
				title: 'Linking tasks on the timeline',
				keywords: ['arrow', 'dot', 'dependency', 'wait']
			},
			{ id: 'unscheduled', title: 'Tasks with no dates', keywords: ['tray', 'schedule'] }
		],
		related: ['views-and-filters', 'reminders-and-notifications']
	},
	{
		slug: 'importing',
		title: 'Importing',
		summary:
			'Bring tasks and pages in from a CSV file or from another app, such as GitHub Issues, ClickUp, Asana, Todoist, monday.com, Notion or Confluence.',
		keywords: [
			'import',
			'csv',
			'spreadsheet',
			'github',
			'issues',
			'clickup',
			'asana',
			'todoist',
			'monday',
			'excel',
			'notion',
			'confluence',
			'wiki',
			'migrate',
			'move',
			'other app'
		],
		sections: [
			{
				id: 'dry-run',
				title: 'Checking first',
				keywords: ['dry run', 'limit', 'offline', 'twice', 'trash']
			},
			{
				id: 'csv',
				title: 'A CSV file',
				keywords: [
					'csv',
					'columns',
					'spreadsheet',
					'headings',
					'subtasks',
					'checklist',
					'dependencies'
				]
			},
			{
				id: 'github',
				title: 'GitHub Issues',
				keywords: ['github', 'gh', 'issues', 'labels', 'milestone', 'comments', 'assignees']
			},
			{
				id: 'clickup',
				title: 'ClickUp',
				keywords: ['clickup', 'list', 'space', 'lead', 'account', 'attachments', 'subtasks']
			},
			{
				id: 'asana',
				title: 'Asana',
				keywords: ['asana', 'section', 'tags', 'dependencies', 'subtasks', 'priority']
			},
			{
				id: 'todoist',
				title: 'Todoist',
				keywords: ['todoist', 'template', 'section', 'labels', 'repeat', 'p1', 'comments']
			},
			{
				id: 'monday',
				title: 'monday.com',
				keywords: ['monday', 'excel', 'board', 'group', 'subitems', 'updates', 'xlsx']
			},
			{
				id: 'notion',
				title: 'Notion',
				keywords: ['notion', 'pages', 'wiki', 'database', 'markdown', 'zip', 'callout']
			},
			{
				id: 'confluence',
				title: 'Confluence',
				keywords: ['confluence', 'atlassian', 'pages', 'wiki', 'markdown', 'space']
			}
		],
		related: ['templates', 'offline-and-sync']
	},
	{
		slug: 'email-to-task',
		title: 'Email to task',
		summary: 'Forward an email to a private address and it becomes a task.',
		keywords: ['email', 'mail', 'forward', 'inbox', 'address', 'imap', 'capture', 'send'],
		sections: [
			{
				id: 'addresses',
				title: 'Your addresses',
				keywords: ['address', 'copy', 'project', 'inbox']
			},
			{
				id: 'the-task',
				title: 'What the task holds',
				keywords: ['subject', 'title', 'notes', 'attachment', 'file']
			},
			{
				id: 'senders',
				title: 'Who can send',
				keywords: ['sender', 'forged', 'listed', 'password']
			},
			{
				id: 'refused',
				title: 'Mail that is refused',
				keywords: ['refused', 'missing', 'spam', 'recent', 'limit', 'reply']
			}
		],
		related: ['api', 'reminders-and-notifications']
	},
	{
		slug: 'desktop-app',
		title: 'The desktop app',
		summary: 'Mila in a window of its own, connected to a server or kept on this computer.',
		keywords: [
			'desktop',
			'app',
			'windows',
			'linux',
			'appimage',
			'install',
			'local',
			'offline',
			'computer'
		],
		sections: [
			{
				id: 'places',
				title: 'Where your work lives',
				keywords: [
					'server',
					'computer',
					'local',
					'switch',
					'menu',
					'places',
					'choose a place',
					'rename'
				]
			},
			{
				id: 'signing-in',
				title: 'Signing in to a server',
				keywords: ['browser', 'passkey', 'sign in']
			},
			{
				id: 'this-computer',
				title: 'Keeping it on this computer',
				keywords: ['local', 'backup', 'export', 'import', 'move', 'folder', 'delete', 'remove']
			},
			{
				id: 'shortcut',
				title: 'Quick add from anywhere',
				keywords: ['shortcut', 'global', 'hotkey', 'wayland']
			},
			{
				id: 'notifications',
				title: 'Desktop notifications',
				keywords: ['notification', 'bell', 'reminder']
			}
		],
		related: ['quick-add', 'offline-and-sync']
	},
	{
		slug: 'automations',
		title: 'Automations',
		summary: 'When something happens to a task, and it fits, Mila makes the changes you chose.',
		keywords: ['automation', 'rule', 'when', 'then', 'trigger', 'action', 'automatic', 'workflow'],
		sections: [
			{ id: 'rules', title: 'Making a rule', keywords: ['when', 'only if', 'then', 'test'] },
			{
				id: 'starts',
				title: 'What can start a rule',
				keywords: ['trigger', 'overdue', 'due', 'blocker', 'email']
			},
			{
				id: 'changes',
				title: 'What a rule can do',
				keywords: ['action', 'assign', 'label', 'move', 'subtask', 'notify']
			},
			{
				id: 'who',
				title: 'Whose rule it is',
				keywords: ['member', 'admin', 'guest', 'runs as', 'permission']
			},
			{
				id: 'limits',
				title: 'What to expect',
				keywords: ['offline', 'undo', 'history', 'paused', 'loop']
			}
		],
		related: ['views-and-filters', 'reminders-and-notifications', 'templates']
	},
	{
		slug: 'api',
		title: 'API tokens and webhooks',
		summary: 'Let a script or another app use Mila as you, or hear when something changes.',
		keywords: [
			'api',
			'token',
			'tokens',
			'webhook',
			'webhooks',
			'integration',
			'script',
			'zapier',
			'bearer',
			'agent'
		],
		sections: [
			{ id: 'tokens', title: 'API tokens', keywords: ['bearer', 'revoke', 'expire', 'read only'] },
			{
				id: 'actions',
				title: 'Actions for scripts and agents',
				keywords: ['agent', 'batch', 'undo', 'find', 'names']
			},
			{ id: 'webhooks', title: 'Webhooks', keywords: ['events', 'post', 'json', 'project'] },
			{
				id: 'signatures',
				title: 'Checking a delivery came from Mila',
				keywords: ['hmac', 'signature', 'secret', 'replay']
			},
			{
				id: 'failures',
				title: 'When deliveries fail',
				keywords: ['retry', 'resend', 'log', 'stopped', 'network']
			}
		],
		related: ['offline-and-sync']
	},
	{
		slug: 'wiki',
		title: 'The wiki',
		summary:
			"Pages for what is not a task: the workspace's wiki, each project's, and their subpages.",
		keywords: ['wiki', 'page', 'pages', 'docs', 'documents', 'notes', 'knowledge', 'markdown'],
		sections: [
			{ id: 'wikis', title: "The workspace's wiki and each project's", keywords: ['tab', 'guest'] },
			{ id: 'pages', title: 'Pages and subpages', keywords: ['new page', 'tree', 'move', 'fold'] },
			{ id: 'writing', title: 'Writing', keywords: ['markdown', 'together', 'live', 'format'] },
			{
				id: 'comments',
				title: 'Comments on a page',
				keywords: ['comment', 'discuss', 'highlight', 'thread', 'resolve', 'reopen', 'review']
			},
			{ id: 'history', title: 'History', keywords: ['versions', 'restore', 'changes', 'undo'] },
			{ id: 'deleting', title: 'Deleting pages', keywords: ['delete', 'trash', 'restore', 'undo'] },
			{ id: 'finding', title: 'Finding a page', keywords: ['search', 'palette', 'link', 'copy'] },
			{
				id: 'exports',
				title: 'Exports',
				keywords: ['export', 'import', 'backup', 'markdown', 'json']
			}
		],
		related: ['writing', 'offline-and-sync', 'views-and-filters']
	},
	{
		slug: 'my-day-and-upcoming',
		title: 'My Day and Upcoming',
		summary: 'Pick what you will do today, and see what is coming up.',
		keywords: ['today', 'plan', 'my day', 'upcoming', 'week', 'overdue'],
		sections: [
			{ id: 'adding', title: 'Adding to My Day', keywords: ['add', 'today', 'plan'] },
			{ id: 'earlier', title: 'From earlier', keywords: ['yesterday', 'carry over'] },
			{ id: 'due', title: 'Due today or overdue', keywords: ['overdue', 'suggested'] },
			{ id: 'upcoming', title: 'Upcoming', keywords: ['week', 'later', 'next'] },
			{
				id: 'views',
				title: 'Other ways to see them',
				keywords: ['calendar', 'table', 'timeline', 'filter', 'display', 'reset', 'inbox']
			}
		],
		related: ['quick-add', 'touch-gestures']
	},
	{
		slug: 'task-panel',
		title: 'The task panel',
		summary: 'Everything about a task in one place, laid out the way you prefer.',
		keywords: ['task', 'panel', 'details', 'fields', 'sections', 'layout', 'boxes', 'summary'],
		sections: [
			{
				id: 'layouts',
				title: 'Labeled boxes or summary lines',
				keywords: ['preference', 'layout', 'boxes', 'summary', 'collapse', 'open', 'close']
			},
			{
				id: 'sections',
				title: 'What each section holds',
				keywords: ['schedule', 'status', 'priority', 'assignees', 'labels', 'dependencies', 'time']
			},
			{
				id: 'comments',
				title: 'Comments',
				keywords: ['mention', '@', 'slash', '/task', '/wiki', 'link', 'reference', 'guest']
			},
			{ id: 'help', title: 'The question marks', keywords: ['help', 'question mark'] }
		],
		related: ['recurrence', 'reminders-and-notifications', 'time-tracking']
	},
	{
		slug: 'recurrence',
		title: 'Repeating tasks',
		summary: 'Tasks that come back on a schedule, or a while after you finish them.',
		keywords: ['repeat', 'recurring', 'recurrence', 'every', 'routine'],
		sections: [
			{ id: 'setting', title: 'Setting a repeat', keywords: ['custom', 'weekday', 'monthly'] },
			{
				id: 'modes',
				title: 'On a schedule or after completion',
				keywords: ['after completion', 'schedule']
			},
			{ id: 'completing', title: 'Completing a repeating task', keywords: ['next', 'complete'] },
			{ id: 'ending', title: 'Ending a repeat', keywords: ['until', 'stop', 'end'] }
		],
		related: ['quick-add', 'reminders-and-notifications']
	},
	{
		slug: 'reminders-and-notifications',
		title: 'Reminders and notifications',
		summary: 'Reminders on tasks, and where notifications reach you: the app, push and email.',
		keywords: ['remind', 'notification', 'alert', 'push', 'email', 'bell'],
		sections: [
			{ id: 'reminders', title: 'Reminders', keywords: ['remind', 'before', 'due time'] },
			{ id: 'channels', title: 'Where notifications go', keywords: ['bell', 'email', 'app'] },
			{ id: 'push', title: 'Push notifications', keywords: ['phone', 'device', 'browser'] },
			{ id: 'digest', title: 'The daily digest', keywords: ['email', 'summary', 'daily'] },
			{
				id: 'weekly',
				title: 'The weekly report',
				keywords: ['email', 'summary', 'week', 'report']
			},
			{ id: 'mentions', title: 'Mentions', keywords: ['@', 'comment', 'mention'] },
			{ id: 'calendar', title: 'Calendar feeds', keywords: ['google', 'outlook', 'ical'] }
		],
		related: ['recurrence', 'quick-add']
	},
	{
		slug: 'offline-and-sync',
		title: 'Offline and sync',
		summary: 'What the sync dot means, working without a connection, undo and new versions.',
		keywords: ['offline', 'sync', 'connection', 'pending', 'undo', 'update', 'reload'],
		sections: [
			{ id: 'dot', title: 'The sync dot', keywords: ['pending', 'syncing', 'synced'] },
			{
				id: 'offline',
				title: 'Working offline',
				keywords: ['connection', 'airplane', 'picture', 'avatar']
			},
			{
				id: 'conflicts',
				title: 'When two people edit',
				keywords: ['conflict', 'same task', 'notes', 'merge', 'together', 'live']
			},
			{ id: 'undo', title: 'Undo', keywords: ['undo', 'mistake', 'trash', 'restore'] },
			{ id: 'updates', title: 'New versions', keywords: ['update', 'reload', 'version'] }
		],
		related: ['touch-gestures']
	},
	{
		slug: 'themes',
		title: 'Themes',
		summary:
			'Make a theme of your own: colors, fonts, corners, spacing and motion, with contrast checks; use one a workspace offers.',
		keywords: [
			'theme',
			'color',
			'appearance',
			'dark',
			'dark mode',
			'light',
			'font',
			'custom',
			'look',
			'theme editor'
		],
		sections: [
			{
				id: 'choosing',
				title: 'Choosing a theme',
				keywords: ['preferences', 'switch', 'standard', 'dark mode', 'density', 'font size']
			},
			{
				id: 'omarchy',
				title: 'Themes from Omarchy',
				keywords: ['omarchy', 'tokyo night', 'catppuccin', 'gruvbox', 'nord', 'rose pine']
			},
			{ id: 'making', title: 'Making a theme', keywords: ['new', 'customize', 'copy', 'aaa'] },
			{
				id: 'editing',
				title: 'Editing a theme',
				keywords: ['theme editor', 'editor', 'accent', 'hue', 'hex', 'preview', 'scheme', 'font']
			},
			{ id: 'undo', title: 'Undo and resets', keywords: ['undo', 'redo', 'reset', 'revert'] },
			{
				id: 'contrast',
				title: 'Contrast and Fix',
				keywords: ['contrast', 'fix', 'readable', 'aa']
			},
			{ id: 'sharing', title: 'Sharing themes', keywords: ['export', 'import', 'file', 'share'] },
			{
				id: 'workspace',
				title: 'Workspace themes',
				keywords: [
					'workspace',
					'team',
					'publish',
					'suggest',
					'suggestion',
					'admin',
					'unpublish',
					'settings'
				]
			}
		],
		related: ['offline-and-sync']
	},
	{
		slug: 'touch-gestures',
		title: 'Touch gestures',
		summary: 'Swipes on a phone or tablet: complete, schedule, close a task, and search.',
		keywords: ['swipe', 'phone', 'tablet', 'touch', 'gesture', 'mobile'],
		sections: [
			{ id: 'complete', title: 'Swipe right to complete', keywords: ['done', 'reopen'] },
			{ id: 'schedule', title: 'Swipe left to schedule', keywords: ['date', 'later'] },
			{ id: 'sheet', title: 'Closing a task', keywords: ['pull down', 'sheet', 'close'] },
			{ id: 'search', title: 'Search and quick add', keywords: ['search', 'add', 'button'] }
		],
		related: ['my-day-and-upcoming', 'quick-add', 'right-click-menus']
	},
	{
		slug: 'right-click-menus',
		title: 'Right-click menus',
		summary: 'What a right-click, Shift F10 or a held finger offers for a task, view or project.',
		keywords: ['right-click', 'context menu', 'menu', 'long press', 'hold', 'duplicate', 'copy'],
		sections: [
			{
				id: 'menus',
				title: 'What a right-click does',
				keywords: ['where', 'wiki', 'comment', 'file']
			},
			{ id: 'tasks', title: "A task's menu", keywords: ['duplicate', 'copy', 'move', 'priority'] },
			{ id: 'keys', title: 'From the keyboard', keywords: ['shift f10', 'menu key', 'keyboard'] },
			{
				id: 'browser',
				title: "Your browser's own menu",
				keywords: ['shift', 'inspect', 'spelling']
			},
			{ id: 'phones', title: 'On a phone', keywords: ['hold', 'long press', 'touch', 'sheet'] }
		],
		related: ['touch-gestures', 'quick-add', 'selecting-tasks']
	},
	{
		slug: 'selecting-tasks',
		title: 'Selecting several tasks',
		summary: 'Select tasks in a list, board or table and change them all at once.',
		keywords: ['select', 'selection', 'bulk', 'many', 'several', 'multiple', 'shift', 'ctrl'],
		sections: [
			{
				id: 'select',
				title: 'Selecting tasks',
				keywords: ['ctrl', 'command', 'shift', 'click', 'circle']
			},
			{
				id: 'keys',
				title: 'From the keyboard',
				keywords: ['space', 'shift', 'select all', 'escape']
			},
			{
				id: 'bar',
				title: 'The bar',
				keywords: ['complete', 'schedule', 'labels', 'assign', 'move', 'delete', 'undo']
			},
			{ id: 'stays', title: 'What stays selected', keywords: ['board', 'drag', 'clear'] },
			{
				id: 'others',
				title: 'Rules and other people',
				keywords: ['automation', 'notification', 'push']
			},
			{ id: 'phones', title: 'On a phone', keywords: ['hold', 'tap', 'done', 'all'] }
		],
		related: ['right-click-menus', 'views-and-filters', 'automations']
	}
];

export interface HelpGroup {
	id: string;
	title: string;
	/** Its topics' slugs, in the order they are read. */
	topics: string[];
}

/** How Help lays out its topics; going through them in order is its reading order. */
export const HELP_GROUPS: HelpGroup[] = [
	{
		id: 'in',
		title: 'Getting work in',
		topics: ['quick-add', 'email-to-task', 'templates', 'recurrence', 'importing']
	},
	{
		id: 'seeing',
		title: 'Seeing your work',
		topics: [
			'views-and-filters',
			'selecting-tasks',
			'my-day-and-upcoming',
			'timeline-and-dependencies',
			'task-panel',
			'time-tracking',
			'reports'
		]
	},
	{ id: 'writing', title: 'Writing', topics: ['writing', 'wiki'] },
	{
		id: 'step',
		title: 'Staying in step',
		topics: ['automations', 'reminders-and-notifications', 'offline-and-sync']
	},
	{
		id: 'yours',
		title: 'Making it yours',
		topics: ['themes', 'touch-gestures', 'right-click-menus']
	},
	{ id: 'beyond', title: 'Beyond the browser', topics: ['desktop-app', 'api'] }
];

const loaders = import.meta.glob<{ default: Component }>('./topics/*.svelte');

/** The slug a topic file is for: ./topics/quick-add.svelte is quick-add. */
export const slugOf = (path: string) => path.replace(/^.*\//, '').replace(/\.svelte$/, '');

const bySlug = new Map(HELP.map((t) => [t.slug, t]));
const loaderBySlug = new Map(Object.entries(loaders).map(([path, load]) => [slugOf(path), load]));

/** A topic by its slug, if there is one. */
export function topic(slug: string): Topic | undefined {
	return bySlug.get(slug);
}

/** The group a topic is in. */
export function groupOf(slug: string): HelpGroup | undefined {
	return HELP_GROUPS.find((g) => g.topics.includes(slug));
}

/** The topics before and after one, in reading order. */
export function neighbors(slug: string): { previous?: Topic; next?: Topic } {
	const order = HELP_GROUPS.flatMap((g) => g.topics);
	const at = order.indexOf(slug);
	if (at < 0) return {};
	return { previous: topic(order[at - 1]), next: topic(order[at + 1]) };
}

/** Loads a topic's text; undefined for a slug that names none. */
export async function loadTopic(slug: string): Promise<Component | undefined> {
	const load = bySlug.has(slug) ? loaderBySlug.get(slug) : undefined;
	return load ? (await load()).default : undefined;
}

/** The slugs of the topic files there are, for the tests. */
export const topicFiles = () => [...loaderBySlug.keys()];
