// Diagrams in a document: a code block in the language `mermaid`, as GitHub,
// Obsidian and Notion write one, drawn by Mermaid (docs/mermaid.ts, loaded
// only once a diagram is drawn). Its source is the block's text, so it is
// edited together live, kept in history and written to Markdown as it is.
// This part is small and loads with the editor: what the starters are, and
// when a diagram's source shows.
import { Extension } from '@tiptap/core';
import type { Node as PMNode } from '@tiptap/pm/model';
import {
	NodeSelection,
	Plugin,
	PluginKey,
	TextSelection,
	type EditorState
} from '@tiptap/pm/state';
import { Decoration, DecorationSet, type EditorView } from '@tiptap/pm/view';

/** Whether a code block's language makes it a diagram. */
export function isDiagram(language: unknown): boolean {
	return typeof language === 'string' && language.trim().toLowerCase() === 'mermaid';
}

export interface Starter {
	id: string;
	label: string;
	/** What it is for, under its name in the list. */
	detail: string;
	/** More words that find it in the `/` menu. */
	words: string[];
	source: string;
}

/** The diagrams a new one can start from, each a small working example. */
export const STARTERS: Starter[] = [
	{
		id: 'flowchart',
		label: 'Flowchart',
		detail: 'Steps and decisions',
		words: ['flow', 'process', 'chart'],
		source: `flowchart LR
  accTitle: Request flow
  A[Request] --> B{Approved?}
  B -- Yes --> C[Start work]
  B -- No --> D[Revise]
  D --> A`
	},
	{
		id: 'sequence',
		label: 'Sequence diagram',
		detail: 'Messages between people or systems',
		words: ['sequence', 'messages', 'interaction'],
		source: `sequenceDiagram
  accTitle: Sign-in sequence
  participant U as User
  participant A as App
  participant S as Server
  U->>A: Enter email
  A->>S: Request sign-in link
  S-->>U: Email with link`
	},
	{
		id: 'state',
		label: 'State diagram',
		detail: 'States and what moves between them',
		words: ['state', 'status', 'machine'],
		source: `stateDiagram-v2
  accTitle: Task states
  [*] --> Todo
  Todo --> Doing: Start
  Doing --> Done: Finish
  Doing --> Todo: Pause
  Done --> [*]`
	},
	{
		id: 'class',
		label: 'Class diagram',
		detail: 'Types, their fields and relations',
		words: ['class', 'uml', 'model'],
		source: `classDiagram
  accTitle: Project model
  class Project {
    +String name
    +addTask()
  }
  class Task {
    +String title
    +Date due
  }
  Project "1" --> "*" Task`
	},
	{
		id: 'er',
		label: 'Entity relationship diagram',
		detail: 'Tables and how they relate',
		words: ['entity', 'relationship', 'database', 'erd', 'schema'],
		source: `erDiagram
  accTitle: Customers and orders
  CUSTOMER ||--o{ ORDER : places
  ORDER ||--|{ LINE_ITEM : contains
  PRODUCT ||--o{ LINE_ITEM : "appears in"`
	},
	{
		id: 'org',
		label: 'Org chart',
		detail: 'Teams and who reports to whom',
		words: ['org', 'organization', 'hierarchy', 'team'],
		source: `flowchart TB
  accTitle: Team structure
  Lead[Team lead] --> Design[Design]
  Lead --> Eng[Engineering]
  Eng --> Web[Web]
  Eng --> Server[Server]`
	},
	{
		id: 'architecture',
		label: 'Architecture diagram',
		detail: 'Services and how they connect',
		words: ['architecture', 'system', 'infrastructure', 'services'],
		source: `architecture-beta
  accTitle: Web service
  group cloud(cloud)[Hosting]
  service web(internet)[Web] in cloud
  service api(server)[API] in cloud
  service db(database)[Database] in cloud
  web:R --> L:api
  api:R --> L:db`
	},
	{
		id: 'mindmap',
		label: 'Mind map',
		detail: 'Ideas around a central topic',
		words: ['mind', 'map', 'brainstorm', 'ideas'],
		source: `mindmap
  root((Launch))
    Marketing
      Website
      Email
    Product
      Beta
      Documentation`
	},
	{
		id: 'timeline',
		label: 'Timeline',
		detail: 'Events in order',
		words: ['timeline', 'history', 'roadmap'],
		source: `timeline
  title Release plan
  Q1 : Research
  Q2 : Beta
  Q3 : Launch`
	},
	{
		id: 'gantt',
		label: 'Gantt chart',
		detail: 'Work over time',
		words: ['gantt', 'schedule', 'plan'],
		source: `gantt
  accTitle: Launch schedule
  dateFormat YYYY-MM-DD
  section Build
    Design :a1, 2026-01-05, 7d
    Develop :a2, after a1, 14d
  section Release
    Launch :after a2, 1d`
	},
	{
		id: 'pie',
		label: 'Pie chart',
		detail: 'Parts of a whole',
		words: ['pie', 'share', 'proportion'],
		source: `pie
  accTitle: Time by area
  "Design" : 30
  "Build" : 50
  "Testing" : 20`
	}
];

/** What each kind of diagram is called, by the type Mermaid names it. */
const KIND_NAMES: Record<string, string> = {
	flowchart: 'Flowchart',
	'flowchart-v2': 'Flowchart',
	graph: 'Flowchart',
	sequence: 'Sequence diagram',
	stateDiagram: 'State diagram',
	state: 'State diagram',
	class: 'Class diagram',
	classDiagram: 'Class diagram',
	er: 'Entity relationship diagram',
	architecture: 'Architecture diagram',
	mindmap: 'Mind map',
	timeline: 'Timeline',
	gantt: 'Gantt chart',
	pie: 'Pie chart',
	journey: 'User journey',
	quadrantChart: 'Quadrant chart',
	requirement: 'Requirement diagram',
	gitGraph: 'Git graph',
	c4: 'C4 diagram',
	sankey: 'Sankey diagram',
	xychart: 'XY chart',
	block: 'Block diagram',
	packet: 'Packet diagram',
	kanban: 'Kanban board',
	radar: 'Radar chart',
	treemap: 'Treemap'
};

/** A diagram's kind in words, from Mermaid's name for it. */
export function kindName(type: string | undefined): string {
	return (type && KIND_NAMES[type]) || 'Diagram';
}

/** The kind a source names on its first line, before it is drawn. */
export function sourceKind(source: string): string | undefined {
	for (const line of source.split('\n')) {
		const t = line.trim();
		if (!t || t.startsWith('%%')) continue;
		const word = /^[A-Za-z][\w-]*/.exec(t)?.[0];
		if (!word) return undefined;
		if (word === 'graph' || word === 'flowchart') return 'flowchart';
		if (word === 'sequenceDiagram') return 'sequence';
		if (word.startsWith('stateDiagram')) return 'state';
		if (word === 'classDiagram') return 'class';
		if (word === 'erDiagram') return 'er';
		if (word.startsWith('architecture')) return 'architecture';
		return word;
	}
	return undefined;
}

/** The title and description a source gives itself, for screen readers. */
export function accessibleText(source: string): { title: string; description: string } {
	let title = '';
	let description = '';
	const lines = source.split('\n');
	for (let i = 0; i < lines.length; i++) {
		const t = lines[i].trim();
		const m = /^acc(Title|Descr)\s*:\s*(.*)$/.exec(t);
		if (m) {
			if (m[1] === 'Title') title ||= m[2].trim();
			else description ||= m[2].trim();
			continue;
		}
		const block = /^accDescr\s*\{(.*)$/.exec(t);
		if (block && !description) {
			const parts = [block[1]];
			while (!parts[parts.length - 1].includes('}') && i + 1 < lines.length)
				parts.push(lines[++i].trim());
			description = parts.join(' ').replace(/\}.*$/, '').trim();
		}
		// A timeline's or a pie's title names it as well.
		const named = /^title\s+(.+)$/.exec(t);
		if (named && !title) title = named[1].trim();
	}
	return { title, description };
}

/** Mermaid's message about a mistake, and the line it names (1 is the first). */
export function mistakeOf(error: unknown): { message: string; line: number | null } {
	const text =
		error instanceof Error
			? error.message
			: typeof error === 'object' && error && 'str' in error
				? String((error as { str: unknown }).str)
				: String(error);
	const m = /line (\d+)/i.exec(text);
	return {
		message: text.trim() || 'This diagram could not be drawn.',
		line: m ? Number(m[1]) : null
	};
}

// When a diagram's source shows: while the caret is in it and the editor
// has focus. Out of it, the block is only its drawing.
const KEY = new PluginKey<boolean>('diagramEditing');

/** The diagram the selection is in, with its position. */
function diagramAt(state: EditorState): { pos: number; node: PMNode } | null {
	const { $from } = state.selection;
	if (state.selection instanceof NodeSelection) return null;
	for (let d = $from.depth; d > 0; d--) {
		const node = $from.node(d);
		if (node.type.name === 'codeBlock' && isDiagram(node.attrs.language))
			return { pos: $from.before(d), node };
	}
	return null;
}

/** The diagram right after (or before) the textblock the caret is in, if one is. */
function diagramBeside(state: EditorState, dir: 1 | -1): { pos: number; node: PMNode } | null {
	const { $from } = state.selection;
	if ($from.depth < 1) return null;
	const edge = dir > 0 ? $from.after() : $from.before();
	const $edge = state.doc.resolve(edge);
	const node = dir > 0 ? $edge.nodeAfter : $edge.nodeBefore;
	if (!node || node.type.name !== 'codeBlock' || !isDiagram(node.attrs.language)) return null;
	return { pos: dir > 0 ? edge : edge - node.nodeSize, node };
}

/** Whether a node view's decorations say its source shows. */
export function editingIn(decorations: readonly { spec?: unknown }[] | undefined): boolean {
	return !!decorations?.some(
		(d) => (d.spec as { diagramEditing?: boolean } | undefined)?.diagramEditing
	);
}

/** Leaves a diagram's source with the diagram picked, so its source hides. */
export function leaveDiagram(view: EditorView, pos: number) {
	view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, pos)));
	view.focus();
}

/** Puts the caret in a diagram's source, at its start or end. */
export function editDiagram(view: EditorView, pos: number, atEnd = true) {
	const node = view.state.doc.nodeAt(pos);
	if (!node) return;
	const at = atEnd ? pos + node.nodeSize - 1 : pos + 1;
	view.dispatch(
		view.state.tr.setSelection(TextSelection.create(view.state.doc, at)).scrollIntoView()
	);
	view.focus();
}

export const DiagramEditing = Extension.create({
	name: 'diagramEditing',
	addProseMirrorPlugins() {
		return [
			new Plugin<boolean>({
				key: KEY,
				state: {
					init: () => false,
					apply: (tr, focused) => (tr.getMeta(KEY) as boolean | undefined) ?? focused
				},
				props: {
					decorations(state) {
						if (!KEY.getState(state)) return null;
						const at = diagramAt(state);
						if (!at) return null;
						return DecorationSet.create(state.doc, [
							Decoration.node(
								at.pos,
								at.pos + at.node.nodeSize,
								{ class: 'editing' },
								{ diagramEditing: true }
							)
						]);
					},
					handleDOMEvents: {
						focus(view) {
							if (!KEY.getState(view.state)) view.dispatch(view.state.tr.setMeta(KEY, true));
							return false;
						},
						blur(view, e) {
							// The formatting bar and the diagram's own buttons keep it open.
							const to = e.relatedTarget as Node | null;
							const doc = view.dom.closest('.doc');
							if (to && doc?.contains(to)) return false;
							if (KEY.getState(view.state)) view.dispatch(view.state.tr.setMeta(KEY, false));
							return false;
						}
					},
					handleKeyDown(view, e) {
						if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return false;
						const { state } = view;
						const inside = diagramAt(state);
						// Escape leaves the source with the diagram picked; Enter goes back in.
						if (e.key === 'Escape' && inside) {
							e.preventDefault();
							leaveDiagram(view, inside.pos);
							return true;
						}
						const sel = state.selection;
						if (
							e.key === 'Enter' &&
							sel instanceof NodeSelection &&
							sel.node.type.name === 'codeBlock' &&
							isDiagram(sel.node.attrs.language)
						) {
							e.preventDefault();
							editDiagram(view, sel.from);
							return true;
						}
						// The source hides while the caret is elsewhere, so the arrow keys
						// would pass over it: they go into it instead.
						if (inside || !sel.empty) return false;
						const down = e.key === 'ArrowDown' || e.key === 'ArrowRight';
						const up = e.key === 'ArrowUp' || e.key === 'ArrowLeft';
						if (!down && !up) return false;
						const dir = down ? 1 : -1;
						const where =
							e.key === 'ArrowDown' ? 'down' : e.key === 'ArrowUp' ? 'up' : down ? 'right' : 'left';
						if (!view.endOfTextblock(where)) return false;
						const next = diagramBeside(state, dir);
						if (!next) return false;
						e.preventDefault();
						editDiagram(view, next.pos, !down);
						return true;
					}
				}
			})
		];
	}
});
