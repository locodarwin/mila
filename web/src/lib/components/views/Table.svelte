<script lang="ts">
	// A view's tasks as a table: one row per task with its subtasks under
	// it, a column per shown field, groups with a summary row each. Headings
	// sort and drag, cells edit in place, rows drag by their handle. A
	// treegrid: one Tab stop, arrows between cells.
	import { menu, taskActions } from '../../contextMenu.svelte';
	import GripVertical from '@lucide/svelte/icons/grip-vertical';
	import Check from '@lucide/svelte/icons/check';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import ArrowUp from '@lucide/svelte/icons/arrow-up';
	import ArrowDown from '@lucide/svelte/icons/arrow-down';
	import Pencil from '@lucide/svelte/icons/pencil';
	import { tick } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { announce } from '../../announce.svelte';
	import { formatDate } from '../../dates';
	import { Growing, upTo } from '../../growing.svelte';
	import type { CommandId } from '../../keys/commands';
	import { keymap } from '../../keys/keymap.svelte';
	import { placeNear } from '../../popover';
	import { scheduler } from '../../schedule.svelte';
	import { session } from '../../session.svelte';
	import {
		localToday,
		placeTask,
		setInMyDay,
		setTaskCompleted,
		setTaskDeleted,
		setTaskFields,
		setTaskTitle,
		writeGroup,
		type ViewEdit
	} from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type {
		FieldValue,
		Grouping,
		Priority,
		SortRule,
		SummaryFn,
		Task,
		ViewDisplay
	} from '../../sync/entities';
	import {
		SUMMARY_LABELS,
		columnSlots,
		columnSort,
		groupRows,
		groupTasksDeep,
		groupWrite,
		headingSort,
		landing,
		moveColumn,
		placeColumn,
		readTable,
		storedSort,
		summarize,
		summaryChoices,
		summaryOf,
		tableColumns,
		withOrder,
		withSummary,
		withWidth,
		type SummaryContext,
		type ColumnSlot,
		type TableColumn,
		type TableRow
	} from '../../table';
	import { openTask } from '../../taskLink';
	import { toasts } from '../../toasts.svelte';
	import {
		ACTS_ON_MANY,
		SELECT_COMMANDS,
		clickSelects,
		commandSelects,
		sayCount,
		selection
	} from '../../selection.svelte';
	import {
		GROUP_LABELS,
		sortRuleValid,
		sortSaid,
		type DisplayLookups,
		type DisplayPath,
		type Section
	} from '../../viewDisplay';
	import Checkbox from '../Checkbox.svelte';
	import EmptyState from '../EmptyState.svelte';
	import BlockedMark from '../BlockedMark.svelte';
	import TableCell from './TableCell.svelte';
	import TablePicker from './TablePicker.svelte';

	type Patch = Partial<Record<DisplayPath, unknown>>;

	interface Props {
		ws: string;
		/** The groups and their top-level tasks, filtered and in order. */
		sections: Section[];
		group: Grouping;
		/** The sort in use. */
		sort: SortRule[];
		display: ViewDisplay;
		look: DisplayLookups;
		/** The fields the columns show, as the view resolves them. */
		shown: string[];
		/** A task's subtasks in the table, filtered and in the view's order. */
		kids: (task: Task) => Task[];
		/** What summaries count, when a row can be there only for a subtask under it. */
		counts?: (task: Task) => boolean;
		/** False when the rows are sorted: dragging them is off. */
		manual: boolean;
		/** What moving a row says instead, when the rows have no one order to change. */
		fixed?: string;
		/** Saves a change to the display, and says what it did once it is saved. */
		onchange: (
			edit: ViewEdit<ViewDisplay, Patch>,
			say?: (display: ViewDisplay) => string
		) => void | Promise<void>;
		label: string;
		emptyTitle: string;
		emptyHint: string;
	}

	let {
		ws,
		sections,
		group,
		sort,
		display,
		look,
		shown,
		kids,
		counts,
		manual,
		fixed,
		onchange,
		label,
		emptyTitle,
		emptyHint
	}: Props = $props();

	const replica = $derived(engine.replica(ws));
	const table = $derived(readTable(display));
	const columns = $derived(tableColumns(shown, table, look.fields));
	const movable = $derived(columns.slice(1).map((c) => c.id));
	const today = localToday();

	// Widths while a heading's edge is dragged, before they are saved.
	let resizing = $state<{ col: string; px: number } | null>(null);
	const widthOf = (c: TableColumn) => (resizing?.col === c.id ? resizing.px : c.width);
	const totalWidth = $derived(columns.reduce((n, c) => n + widthOf(c), 0));

	// Subtasks folded away, remembered on this device.
	const FOLDED_KEY = 'mila.table.folded';
	const folded = new SvelteSet<string>(readFolded());
	function readFolded(): string[] {
		try {
			const v = JSON.parse(localStorage.getItem(FOLDED_KEY) ?? '[]');
			return Array.isArray(v) ? v.filter((x) => typeof x === 'string') : [];
		} catch {
			return [];
		}
	}
	function fold(id: string, on: boolean) {
		if (on) folded.add(id);
		else folded.delete(id);
		try {
			localStorage.setItem(FOLDED_KEY, JSON.stringify([...folded].slice(-500)));
		} catch {
			// Kept for this visit only.
		}
	}

	// What a row drag can drop on: every group that stands for something a
	// drop can set is shown, empty ones too, while a row is dragged.
	let dragging = $state<{ row: TableRow; pointer: number } | null>(null);
	const dropGroups = $derived(
		dragging?.row.depth === 0 && ['status', 'priority', 'project', 'field'].includes(group.by)
	);
	const visible = $derived(sections.filter((s) => s.tasks.length || dropGroups));
	// A long table is drawn a screenful at a time, as it is scrolled.
	const growing = new Growing(() => visible.reduce((n, s) => n + s.tasks.length, 0));
	const drawn = $derived(
		upTo(visible, growing.limit).map(({ group: section, shown: tasks }) => ({
			section,
			rows: groupRows({ ...section, tasks }, kids, folded),
			all: counts ? groupTasksDeep(section, kids).filter(counts) : groupTasksDeep(section, kids)
		}))
	);

	// Summaries
	const values = $derived(
		replica?.fieldValuesByField() ?? new Map<string, Map<string, FieldValue>>()
	);
	const number = new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 });
	const summaryCtx = $derived<SummaryContext>({
		today,
		category: (t) => replica?.status(t.statusId)?.category ?? (t.completedAt ? 'done' : 'todo'),
		projects: replica?.projects() ?? [],
		assignees: (id) => replica?.assignees(id) ?? [],
		labels: (id) => (replica?.taskLabels(id) ?? []).map((l) => l.id),
		values,
		nameOf: (id) => session.nameOf(ws, id),
		count: (t, what) =>
			what === 'subtasks'
				? (replica?.subtasks(t.id).length ?? 0)
				: what === 'checklist'
					? (replica?.checklist(t.id).length ?? 0)
					: what === 'comments'
						? (replica?.comments(t.id).length ?? 0)
						: what === 'reminders' && session.user
							? (replica?.reminders(session.user.id, t.id).length ?? 0)
							: 0,
		formatDate: (d) => formatDate(d),
		formatNumber: (n) => number.format(n),
		logged: (id) => replica?.logged(id) ?? 0
	});

	// The cells, for the keyboard: the heading row, then each group's rows
	// and its summary row. A cell is known by its row's key and column.
	const navRows = $derived([
		'head',
		...drawn.flatMap((g) => [...g.rows.map((r) => r.key), `sum:${g.section.id}`])
	]);
	let active = $state<{ row: string; col: number }>({ row: 'head', col: 0 });
	const at = $derived({
		row: navRows.includes(active.row) ? active.row : (navRows[1] ?? 'head'),
		col: Math.min(active.col, columns.length - 1)
	});
	const isActive = (row: string, col: number) => at.row === row && at.col === col;
	let root = $state<HTMLElement>();

	function cellEl(row: string, col: number): HTMLElement | null {
		return root?.querySelector<HTMLElement>(`[data-cell="${CSS.escape(`${row}|${col}`)}"]`) ?? null;
	}
	async function focusCell(row: string, col: number) {
		active = { row, col };
		await tick();
		cellEl(row, col)?.focus();
	}
	const rowOf = (key: string) => drawn.flatMap((g) => g.rows).find((r) => r.key === key);

	// Selecting several: the rows as drawn, then the
	// tasks not drawn yet, each once.
	const order = () => [
		...new Set([
			...drawn.flatMap((g) => g.rows.map((r) => r.task.id)),
			...visible.flatMap((s) => s.tasks.map((t) => t.id))
		])
	];
	$effect(() => selection.register(order));
	const selecting = $derived(selection.on(ws));
	function selectClick(e: MouseEvent) {
		const tr = (e.target as HTMLElement).closest<HTMLElement>('tr[data-task]');
		if (!tr || (e.target as Element).closest('input, textarea')) return;
		if (clickSelects(e, ws, tr.dataset.task!, order)) announce(sayCount());
	}

	// Sorting by a heading
	function sortBy(col: TableColumn, add: boolean) {
		if (!col.sort) {
			announce(`${col.label} cannot be sorted`);
			return;
		}
		const rule = col.sort;
		void onchange(
			(d) => {
				const next = headingSort(storedSort(d), rule, add);
				return { sort: next.length ? next : undefined };
			},
			(d) =>
				sortSaid(
					storedSort(d).filter((r): r is SortRule => sortRuleValid(r, look)),
					look.fields
				)
		);
	}

	// Moving and resizing columns
	function saveOrder(order: string[], said: string) {
		void onchange(
			(d) => ({ table: withOrder(readTable(d), order) }),
			() => said
		);
	}
	async function shiftColumn(col: TableColumn, by: -1 | 1) {
		const next = moveColumn(movable, col.id, by);
		if (!next) return;
		saveOrder(
			next,
			`${col.label} moved to column ${next.indexOf(col.id) + 2} of ${columns.length}`
		);
		await tick();
		await focusCell('head', next.indexOf(col.id) + 1);
	}

	// A heading held and dragged moves its column; one clicked sorts.
	let columnDrag = $state<{ col: TableColumn; x: number; before: string | null } | null>(null);
	let press: { col: TableColumn; x: number; shift: boolean; id: number } | null = null;
	function headDown(e: PointerEvent, col: TableColumn) {
		if (e.button !== 0) return;
		press = { col, x: e.clientX, shift: e.shiftKey, id: e.pointerId };
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}
	function headMove(e: PointerEvent) {
		if (!press || press.col.id === 'title') return;
		if (!columnDrag && Math.abs(e.clientX - press.x) < 6) return;
		const heads = [...(root?.querySelectorAll<HTMLElement>('th[data-col]') ?? [])].slice(1);
		const before =
			heads.find((h) => {
				const r = h.getBoundingClientRect();
				return e.clientX < r.left + r.width / 2;
			})?.dataset.col ?? null;
		columnDrag = { col: press.col, x: e.clientX, before };
	}
	function headUp(e: PointerEvent) {
		const p = press;
		press = null;
		if (!p) return;
		if (columnDrag) {
			const { col, before } = columnDrag;
			columnDrag = null;
			const next = placeColumn(movable, col.id, before === col.id ? null : before);
			if (next.join() !== movable.join())
				saveOrder(next, `${col.label} moved to column ${next.indexOf(col.id) + 2}`);
			return;
		}
		if (e.type === 'pointerup') sortBy(p.col, p.shift || e.shiftKey);
	}
	const dropX = $derived.by(() => {
		if (!columnDrag || !root) return null;
		const heads = [...root.querySelectorAll<HTMLElement>('th[data-col]')];
		const h = columnDrag.before
			? heads.find((x) => x.dataset.col === columnDrag!.before)
			: heads.at(-1);
		if (!h) return null;
		const r = h.getBoundingClientRect();
		return columnDrag.before ? r.left : r.right;
	});

	let resizeStart: { col: TableColumn; x: number; px: number } | null = null;
	function gripDown(e: PointerEvent, col: TableColumn) {
		e.stopPropagation();
		if (e.button !== 0) return;
		resizeStart = { col, x: e.clientX, px: widthOf(col) };
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}
	function gripMove(e: PointerEvent) {
		if (!resizeStart) return;
		const px = Math.max(60, Math.min(640, resizeStart.px + e.clientX - resizeStart.x));
		resizing = { col: resizeStart.col.id, px };
	}
	function gripUp() {
		const r = resizing;
		const s = resizeStart;
		resizeStart = null;
		if (!r || !s) return;
		void Promise.resolve(
			onchange(
				(d) => ({ table: withWidth(readTable(d), r.col, r.px) }),
				() => `${s.col.label} is ${Math.round(r.px)} pixels wide`
			)
		).finally(() => (resizing = null));
	}

	/** A heading's width a step at a time, from the keyboard. */
	function resizeBy(col: TableColumn, by: number) {
		const px = Math.max(60, Math.min(640, widthOf(col) + by));
		if (px === widthOf(col)) return announce(`${col.label} is ${px} pixels wide`);
		void onchange(
			(d) => ({ table: withWidth(readTable(d), col.id, px) }),
			() => `${col.label} is ${px} pixels wide`
		);
	}

	// Rows: completing, renaming, moving
	function complete(task: Task, on: boolean) {
		const { undo } = engine.track(() => setTaskCompleted(ws, task, on));
		if (on) toasts.show(`Completed "${task.title}"`, { onundo: () => void undo() });
	}

	let renaming = $state<string | null>(null);
	let draft = $state('');
	async function rename(row: TableRow) {
		renaming = row.key;
		draft = row.task.title;
		await tick();
		root?.querySelector<HTMLInputElement>('input.rename')?.select();
	}
	function finishRename(row: TableRow, save: boolean) {
		if (renaming !== row.key) return;
		renaming = null;
		const title = draft.trim();
		if (save && title && title !== row.task.title) void setTaskTitle(ws, row.task, title);
		void focusCell(row.key, 0);
	}

	/** The tasks a row moves among: its group's own, or its parent's subtasks. */
	function siblingsOf(row: TableRow): Task[] {
		if (row.parent) {
			const parent = replica?.task(row.parent);
			return parent ? kids(parent) : [];
		}
		return sections.find((s) => s.id === row.group)?.tasks ?? [];
	}
	const sortedSay = 'The rows are sorted. Clear the sort to move them.';

	function moveRow(row: TableRow, by: -1 | 1) {
		if (!manual || (fixed && !row.parent)) {
			announce(fixed ?? sortedSay);
			return;
		}
		const list = siblingsOf(row);
		const i = list.findIndex((t) => t.id === row.task.id);
		const to = i + by;
		if (i < 0 || to < 0 || to >= list.length) return;
		const { before, after } = landing(list, row.task.id, to);
		void placeTask(ws, row.task.id, before, after);
		announce(`Moved ${row.task.title} to ${to + 1} of ${list.length}`);
		void focusCell(row.key, at.col);
	}

	// Dragging a row by its handle: within its group or to another (which
	// sets what that group stands for), or a subtask among its siblings.
	let dropAt = $state<{ group: string; anchor: string | null; after: boolean; y: number } | null>(
		null
	);
	function handleDown(e: PointerEvent, row: TableRow) {
		if (e.button !== 0) return;
		if (!manual) {
			toasts.show(sortedSay);
			return;
		}
		e.preventDefault();
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		dragging = { row, pointer: e.pointerId };
	}
	function handleMove(e: PointerEvent) {
		if (!dragging || !root) return;
		const row = dragging.row;
		const els = [...root.querySelectorAll<HTMLElement>('tr[data-key], tr[data-empty]')];
		const blocks: { group: string; task: string | null; top: number; bottom: number }[] = [];
		els.forEach((el, i) => {
			if (el.dataset.empty !== undefined) {
				if (row.depth !== 0) return;
				const r = el.getBoundingClientRect();
				blocks.push({ group: el.dataset.group!, task: null, top: r.top, bottom: r.bottom });
				return;
			}
			const depth = Number(el.dataset.depth);
			const same =
				row.depth === 0
					? depth === 0
					: el.dataset.parent === row.parent && el.dataset.group === row.group;
			if (!same) return;
			let j = i;
			while (
				els[j + 1]?.dataset.group === el.dataset.group &&
				Number(els[j + 1].dataset.depth ?? 0) > depth
			)
				j++;
			blocks.push({
				group: el.dataset.group!,
				task: el.dataset.task!,
				top: el.getBoundingClientRect().top,
				bottom: els[j].getBoundingClientRect().bottom
			});
		});
		if (!blocks.length) return;
		const y = e.clientY;
		const hit =
			blocks.find((b) => y >= b.top && y < b.bottom) ??
			(y < blocks[0].top ? blocks[0] : blocks.at(-1)!);
		const after = hit.task !== null && y > (hit.top + hit.bottom) / 2;
		dropAt = { group: hit.group, anchor: hit.task, after, y: after ? hit.bottom : hit.top };
	}
	async function handleUp() {
		const d = dragging;
		const target = dropAt;
		dragging = null;
		dropAt = null;
		if (!d || !target) return;
		const row = d.row;
		if (target.anchor === row.task.id) return;
		const moved = row.group !== target.group;
		const write = moved ? groupWrite(group, target.group) : null;
		if (moved && !write) {
			const by = group.by === 'field' ? 'that field' : GROUP_LABELS[group.by].toLowerCase();
			toasts.show(`Tasks grouped by ${by} can only move within their group.`);
			return;
		}
		const list =
			row.depth === 0
				? (sections.find((s) => s.id === target.group)?.tasks ?? [])
				: siblingsOf(row);
		const others = list.filter((t) => t.id !== row.task.id);
		const anchorAt = target.anchor ? others.findIndex((t) => t.id === target.anchor) : -1;
		const index = anchorAt < 0 ? 0 : anchorAt + (target.after ? 1 : 0);
		const { before, after } = landing(list, row.task.id, index);
		let ok = true;
		await engine.group(async () => {
			if (write) ok = await writeGroup(ws, row.task, write);
			if (ok && !(fixed && row.depth === 0)) await placeTask(ws, row.task.id, before, after);
		});
		if (!ok) toasts.show(`"${row.task.title}" has no status of that kind in its project.`);
		else
			announce(
				moved
					? `Moved ${row.task.title} to ${sections.find((s) => s.id === target.group)?.title}`
					: `Moved ${row.task.title} to ${index + 1} of ${list.length}`
			);
	}

	// The picker: a cell's, or a summary's choices.
	let picker = $state<HTMLElement>();
	let editing = $state<
		| { kind: 'cell'; row: TableRow; col: TableColumn; cell: string }
		| { kind: 'sum'; col: TableColumn; cell: string }
		| null
	>(null);
	const anchors = typeof CSS !== 'undefined' && CSS.supports?.('anchor-name: --a');

	// The columns rows draw: those near what shows, moved along only every
	// half a screen, since each move redraws cells in every row.
	let viewWidth = $state(typeof innerWidth === 'number' ? innerWidth : 1600);
	let windowLeft = $state(0);
	function scrolled(e: Event) {
		const el = e.currentTarget as HTMLElement;
		if (Math.abs(el.scrollLeft - windowLeft) > el.clientWidth / 2) windowLeft = el.scrollLeft;
	}
	const openCol = $derived(columns.findIndex((c) => c.id === editing?.col.id));
	const focusCol = $derived(at.col);
	let lastSlots: ColumnSlot[] = [];
	let lastColumns: TableColumn[] = [];
	const slots = $derived.by(() => {
		const next = columnSlots(columns, windowLeft, viewWidth, [focusCol, openCol]);
		// The same slots are the same array, so rows only redraw for a change.
		const same =
			columns === lastColumns &&
			next.length === lastSlots.length &&
			next.every((s, i) => s.key === lastSlots[i].key);
		if (same) return lastSlots;
		lastColumns = columns;
		lastSlots = next;
		return next;
	});

	async function openPicker(next: NonNullable<typeof editing>) {
		const anchor = root?.querySelector<HTMLElement>(`[data-cell="${CSS.escape(next.cell)}"]`);
		if (!anchor) return;
		root?.querySelectorAll<HTMLElement>('[data-cell]').forEach((c) => (c.style.anchorName = ''));
		anchor.style.anchorName = '--mila-table-cell';
		editing = next;
		await tick();
		if (!picker) return;
		picker.showPopover();
		if (!anchors) {
			const { top, left } = placeNear(
				anchor.getBoundingClientRect(),
				{ width: picker.offsetWidth, height: picker.offsetHeight },
				{ width: innerWidth, height: innerHeight }
			);
			picker.style.top = `${top}px`;
			picker.style.left = `${left}px`;
		}
		// The chosen choice has focus, else the first thing to choose.
		(
			picker.querySelector<HTMLElement>('[aria-checked="true"]') ??
			picker.querySelector<HTMLElement>('[data-opt], input, select, button')
		)?.focus();
	}
	function closePicker() {
		if (picker?.matches(':popover-open')) picker.hidePopover();
	}
	function pickerToggled(e: ToggleEvent) {
		if (e.newState !== 'closed' || !editing) return;
		const cell = editing.cell;
		editing = null;
		// Back to the cell, unless focus went somewhere on purpose.
		const now = document.activeElement;
		if (!now || now === document.body || picker?.contains(now)) {
			const [row, col] = cell.split('|');
			void focusCell(row, Number(col));
		}
	}

	function activate(row: TableRow, col: TableColumn, index: number) {
		const cell = `${row.key}|${index}`;
		active = { row: row.key, col: index };
		if (col.kind === 'title') return void openTask(row.task.id);
		if (col.id === 'due') {
			scheduler.open(ws, row.task.id, cellEl(row.key, index));
			return;
		}
		if (!col.editable) return void openTask(row.task.id);
		void openPicker({ kind: 'cell', row, col, cell });
	}
	function chooseSummary(col: TableColumn, fn: SummaryFn) {
		closePicker();
		void onchange(
			(d) => ({ table: withSummary(readTable(d), col, fn) }),
			() => `${col.label} summary: ${SUMMARY_LABELS[fn]}`
		);
	}

	// The keyboard, on any cell.
	const PRIORITY_OF: Partial<Record<CommandId, Priority>> = {
		'task.priority.urgent': 'urgent',
		'task.priority.high': 'high',
		'task.priority.medium': 'medium',
		'task.priority.low': 'low',
		'task.priority.none': 'none'
	};
	function onkeydown(e: KeyboardEvent) {
		const cell = (e.target as HTMLElement).closest<HTMLElement>('[data-cell]');
		if (!cell || e.target !== cell) return;
		const [rowKey, colText] = cell.dataset.cell!.split('|');
		const c = Number(colText);
		const col = columns[c];
		const row = rowOf(rowKey);
		// Selecting comes first, so Shift and an arrow stretch the selection.
		const sel = row ? keymap.match('list', e) : null;
		if (row && sel && SELECT_COMMANDS.includes(sel)) {
			const to = commandSelects(sel, ws, row.task.id, order());
			if (to === null) return;
			e.preventDefault();
			const key = drawn.flatMap((g) => g.rows).find((r) => r.task.id === to)?.key;
			if (key && key !== rowKey) void focusCell(key, c);
			announce(sayCount());
			return;
		}
		const cmd = keymap.match('table', e) ?? (row ? keymap.match('list', e) : null);
		if (!cmd) return;
		e.preventDefault();
		if (row && ACTS_ON_MANY.includes(cmd) && selection.on(ws) && selection.shown().length) {
			void import('../../manyActions').then((m) => m.runCommand(ws, cmd));
			return;
		}
		const r = navRows.indexOf(rowKey);
		switch (cmd) {
			case 'table.walk': {
				if (e.key === 'ArrowLeft' && row && c === 0) {
					if (row.kids && row.open) return fold(row.task.id, true);
					if (row.parent) {
						const up = drawn
							.flatMap((g) => g.rows)
							.find((x) => x.group === row.group && x.task.id === row.parent);
						if (up) void focusCell(up.key, 0);
					}
					return;
				}
				if (e.key === 'ArrowRight' && row && c === 0 && row.kids && !row.open)
					return fold(row.task.id, false);
				const to =
					e.key === 'ArrowUp'
						? { row: navRows[Math.max(0, r - 1)], col: c }
						: e.key === 'ArrowDown'
							? { row: navRows[Math.min(navRows.length - 1, r + 1)], col: c }
							: e.key === 'ArrowLeft'
								? { row: rowKey, col: Math.max(0, c - 1) }
								: e.key === 'ArrowRight'
									? { row: rowKey, col: Math.min(columns.length - 1, c + 1) }
									: e.key === 'Home'
										? { row: rowKey, col: 0 }
										: { row: rowKey, col: columns.length - 1 };
				return void focusCell(to.row, to.col);
			}
			case 'table.edit':
				if (rowKey === 'head') return sortBy(col, e.shiftKey);
				if (rowKey.startsWith('sum:'))
					return void openPicker({ kind: 'sum', col, cell: cell.dataset.cell! });
				if (row) activate(row, col, c);
				return;
			case 'table.rename':
				if (row) void rename(row);
				return;
			case 'table.column-left':
			case 'table.column-right':
				if (rowKey === 'head' && c > 0) void shiftColumn(col, cmd === 'table.column-left' ? -1 : 1);
				return;
			case 'table.narrower':
			case 'table.wider':
				if (rowKey === 'head') resizeBy(col, cmd === 'table.wider' ? 16 : -16);
				return;
		}
		if (!row) return;
		const task = row.task;
		const priority = PRIORITY_OF[cmd];
		if (cmd === 'task.next' || cmd === 'task.prev') {
			const step = cmd === 'task.next' ? 1 : -1;
			const rows = navRows.filter((k) => k !== 'head' && !k.startsWith('sum:'));
			const next = rows[rows.indexOf(rowKey) + step];
			if (next) void focusCell(next, c);
		} else if (cmd === 'task.open') {
			void openTask(task.id);
		} else if (cmd === 'task.move-up' || cmd === 'task.move-down') {
			moveRow(row, cmd === 'task.move-up' ? -1 : 1);
		} else if (cmd === 'task.complete') {
			complete(task, task.completedAt === null);
		} else if (cmd === 'task.my-day' && session.user) {
			const on = !replica?.inMyDay(session.user.id, task.id, localToday());
			void setInMyDay(ws, session.user.id, task.id, on);
			toasts.show(on ? 'Added to My Day' : 'Taken out of My Day');
		} else if (priority) {
			void setTaskFields(ws, task.id, { priority });
			announce(`Priority: ${priority === 'none' ? 'none' : priority}`);
		} else if (cmd === 'task.delete') {
			void setTaskDeleted(ws, task.id, true);
			toasts.show(`Deleted "${task.title}"`, {
				onundo: () => void setTaskDeleted(ws, task.id, false)
			});
		}
	}

	const sortIcon = (col: TableColumn) => columnSort(sort, col);
	const ariaSort = (col: TableColumn) => {
		const s = sortIcon(col);
		return !s || s.index > 0 ? undefined : s.dir === 'asc' ? 'ascending' : 'descending';
	};
	const statusOf = (t: Task) => replica?.status(t.statusId);
	const valueOf = (t: Task, col: TableColumn) =>
		col.field ? values.get(col.field.id)?.get(t.id) : undefined;
	const rowCount = $derived(drawn.reduce((n, g) => n + g.rows.length, 0));
</script>

{#if !sections.some((s) => s.tasks.length)}
	<EmptyState title={emptyTitle} hint={emptyHint} />
{:else}
	<div class="scroll" data-table onscroll={scrolled} bind:clientWidth={viewWidth}>
		<!-- A table, so the browser lays the columns out, in the treegrid role,
		     which is what its keyboard is. -->
		<!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
		<table
			bind:this={root}
			role="treegrid"
			aria-label={label}
			aria-colcount={columns.length}
			style:width={`${totalWidth}px`}
			class:dragging={!!dragging || !!columnDrag}
			aria-multiselectable="true"
			{onkeydown}
			onclickcapture={selectClick}
			onmousedown={(e) =>
				e.shiftKey && (e.target as Element).closest('tr[data-task]') && e.preventDefault()}
		>
			<colgroup>
				{#each columns as c (c.id)}<col style:width={`${widthOf(c)}px`} />{/each}
			</colgroup>
			<thead>
				<tr>
					{#each columns as c, i (c.id)}
						{@const s = sortIcon(c)}
						<th
							role="columnheader"
							data-col={c.id}
							data-cell={`head|${i}`}
							class:sticky={i === 0}
							class:lifted={columnDrag?.col.id === c.id}
							aria-sort={ariaSort(c)}
							tabindex={isActive('head', i) ? 0 : -1}
							onfocus={() => (active = { row: 'head', col: i })}
						>
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div
								class="head"
								class:sortable={!!c.sort}
								class:movable={i > 0}
								onpointerdown={(e) => headDown(e, c)}
								onpointermove={headMove}
								onpointerup={headUp}
								onpointercancel={() => {
									press = null;
									columnDrag = null;
								}}
							>
								<span class="label">{c.label}</span>
								{#if s}
									<span class="arrow" aria-hidden="true">
										{#if s.dir === 'asc'}<ArrowUp size={12} />{:else}<ArrowDown size={12} />{/if}
										{#if sort.length > 1}<sup>{s.index + 1}</sup>{/if}
									</span>
									<span class="visually-hidden"
										>, sorted {s.dir === 'asc' ? 'ascending' : 'descending'}{sort.length > 1
											? `, key ${s.index + 1}`
											: ''}</span
									>
								{/if}
							</div>
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<span
								class="grip"
								class:on={resizing?.col === c.id}
								title="Drag to resize"
								onpointerdown={(e) => gripDown(e, c)}
								onpointermove={gripMove}
								onpointerup={gripUp}
								onpointercancel={() => {
									resizeStart = null;
									resizing = null;
								}}
							></span>
						</th>
					{/each}
				</tr>
			</thead>
			{#each drawn as g (g.section.id)}
				<tbody>
					<tr class="group">
						<td role="gridcell" colspan={columns.length} aria-colspan={columns.length}>
							<div class="ghead" class:danger={g.section.tone === 'danger'}>
								<h2>{g.section.title}</h2>
								<span class="count tabular"
									>{g.section.tasks.length}<span class="visually-hidden">
										{g.section.tasks.length === 1 ? 'task' : 'tasks'}</span
									></span
								>
							</div>
						</td>
					</tr>
					{#each g.rows as row (row.key)}
						{@const t = row.task}
						<tr
							data-key={row.key}
							data-task={t.id}
							use:menu={taskActions(ws, t.id)}
							data-group={row.group}
							data-depth={row.depth}
							data-parent={row.parent ?? ''}
							aria-level={row.depth + 1}
							aria-expanded={row.kids ? row.open : undefined}
							class:done={t.completedAt !== null}
							class:lifted={dragging?.row.key === row.key}
							class:picked={selection.has(ws, t.id)}
							aria-selected={selection.has(ws, t.id)}
						>
							{#each slots as slot (slot.key)}
								{#if 'gap' in slot}
									<td class="gap" aria-hidden="true" colspan={slot.gap}></td>
								{:else if slot.col.kind === 'title'}
									{@const i = slot.at}
									<td
										role="gridcell"
										aria-colindex={i + 1}
										class="title sticky"
										data-cell={`${row.key}|${i}`}
										tabindex={isActive(row.key, i) ? 0 : -1}
										onfocus={() => (active = { row: row.key, col: i })}
									>
										<div class="titlecell" style:padding-left={`${row.depth * 20}px`}>
											{#if selecting}
												<button
													type="button"
													class="pick"
													tabindex="-1"
													role="checkbox"
													aria-checked={selection.has(ws, t.id)}
													aria-label={`Select ${t.title}`}
													title={selection.has(ws, t.id)
														? 'Take out of the selection'
														: 'Add to the selection'}
													onclick={(e) => {
														e.stopPropagation();
														selection.toggle(ws, t.id);
														announce(sayCount());
													}}
													><span class="dot" aria-hidden="true"
														>{#if selection.has(ws, t.id)}<Check
																size={11}
																strokeWidth={3}
															/>{/if}</span
													></button
												>
											{/if}
											<button
												type="button"
												class="handle"
												tabindex="-1"
												aria-label={`Move ${t.title}`}
												aria-disabled={!manual}
												title={manual ? 'Drag to move' : 'Clear the sort to move rows'}
												onpointerdown={(e) => handleDown(e, row)}
												onpointermove={handleMove}
												onpointerup={handleUp}
												onpointercancel={() => {
													dragging = null;
													dropAt = null;
												}}><GripVertical size={14} /></button
											>
											{#if row.kids}
												<button
													type="button"
													class="twisty"
													tabindex="-1"
													aria-label={`${row.open ? 'Hide' : 'Show'} subtasks of ${t.title}`}
													onclick={() => fold(t.id, row.open)}
													><span class:closed={!row.open}><ChevronDown size={14} /></span></button
												>
											{:else}<span class="twisty-gap"></span>{/if}
											<Checkbox
												label={t.title}
												hideLabel
												checked={t.completedAt !== null}
												onchange={(on) => complete(t, on)}
											/>
											{#if renaming === row.key}
												<input
													class="rename"
													aria-label="Task title"
													bind:value={draft}
													onkeydown={(e) => {
														e.stopPropagation();
														if (e.key === 'Enter') finishRename(row, true);
														if (e.key === 'Escape') {
															e.preventDefault();
															finishRename(row, false);
														}
													}}
													onblur={() => finishRename(row, true)}
												/>
											{:else}
												<button
													type="button"
													class="titletext"
													tabindex="-1"
													onclick={() => openTask(t.id)}>{t.title}</button
												>
												{#if !t.completedAt && replica?.isBlocked(t.id)}<BlockedMark
														names={replica.openBlockers(t.id)}
													/>{/if}
												{#if row.kids && !row.open}<span class="subcount tabular"
														>{row.kids}<span class="visually-hidden"> subtasks hidden</span></span
													>{/if}
												<button
													type="button"
													class="pencil"
													tabindex="-1"
													aria-label={`Rename ${t.title}`}
													title="Rename"
													onclick={() => rename(row)}><Pencil size={13} /></button
												>
											{/if}
										</div>
									</td>
								{:else}
									{@const c = slot.col}
									{@const i = slot.at}
									<td
										role="gridcell"
										aria-colindex={i + 1}
										class="cell"
										class:end={c.end}
										class:open={editing?.kind === 'cell' &&
											editing.row.key === row.key &&
											editing.col.id === c.id}
										data-cell={`${row.key}|${i}`}
										tabindex={isActive(row.key, i) ? 0 : -1}
										onfocus={() => (active = { row: row.key, col: i })}
										onclick={() => activate(row, c, i)}
									>
										<TableCell {ws} task={t} col={c} status={statusOf(t)} value={valueOf(t, c)} />
									</td>
								{/if}
							{/each}
						</tr>
					{:else}
						{#if dropGroups}
							<tr class="empty-group" data-empty data-group={g.section.id}>
								<td colspan={columns.length}>Drop here to move to {g.section.title}</td>
							</tr>
						{/if}
					{/each}
					<tr class="sum">
						{#each slots as slot (slot.key)}
							{#if 'gap' in slot}
								<td class="gap" aria-hidden="true" colspan={slot.gap}></td>
							{:else}
								{@const c = slot.col}
								{@const i = slot.at}
								{@const fn = summaryOf(table, c)}
								{@const s = summarize(fn, c, g.all, summaryCtx)}
								<td
									role="gridcell"
									aria-colindex={i + 1}
									class:sticky={i === 0}
									class:end={c.end}
									data-cell={`sum:${g.section.id}|${i}`}
									tabindex={isActive(`sum:${g.section.id}`, i) ? 0 : -1}
									onfocus={() => (active = { row: `sum:${g.section.id}`, col: i })}
									onclick={() =>
										openPicker({ kind: 'sum', col: c, cell: `sum:${g.section.id}|${i}` })}
									title={`${c.label}: ${SUMMARY_LABELS[fn]}`}
								>
									<span class="visually-hidden">{c.label} summary: </span>
									{#if s?.kind === 'bar'}
										<span class="bar" aria-hidden="true">
											{#each s.parts as p (p.key)}<span
													style:flex-grow={p.count}
													style:background={p.color}
													title={`${p.label}: ${p.count}`}
												></span>{/each}
										</span>
										<span class="visually-hidden">{s.said}</span>
									{:else if s?.text}
										<span class="sumtext" aria-hidden="true">{s.text}</span>
										<span class="visually-hidden">{s.said}</span>
									{:else if s}
										<span class="visually-hidden">{s.said}</span>
									{:else}
										<span class="add" aria-hidden="true">Summarize</span>
										<span class="visually-hidden">None</span>
									{/if}
								</td>
							{/if}
						{/each}
					</tr>
				</tbody>
			{/each}
		</table>
		<div use:growing.more aria-hidden="true"></div>
		<p class="visually-hidden" aria-live="off">{rowCount} rows</p>
	</div>
{/if}

{#if dropAt && root}
	{@const r = root.getBoundingClientRect()}
	<div
		class="dropline"
		aria-hidden="true"
		style:top={`${dropAt.y - 1}px`}
		style:left={`${r.left}px`}
		style:width={`${Math.min(r.width, innerWidth - r.left)}px`}
	></div>
{/if}
{#if dropX !== null && root}
	{@const r = root.getBoundingClientRect()}
	<div
		class="dropline vertical"
		aria-hidden="true"
		style:left={`${dropX - 1}px`}
		style:top={`${r.top}px`}
		style:height={`${Math.min(r.height, innerHeight - r.top)}px`}
	></div>
{/if}

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="menu table-picker"
	popover="auto"
	bind:this={picker}
	style:position-anchor="--mila-table-cell"
	ontoggle={pickerToggled}
	onkeydown={(e) => {
		if (e.key === 'Escape') {
			e.preventDefault();
			closePicker();
		}
	}}
>
	{#if editing?.kind === 'cell'}
		{@const live = replica?.task(editing.row.task.id)}
		{#if live}<TablePicker {ws} task={live} col={editing.col} ondone={closePicker} />{/if}
	{:else if editing?.kind === 'sum'}
		{@const col = editing.col}
		{@const current = summaryOf(table, col)}
		<p class="heading">Summary of {col.label}</p>
		<div role="menu" aria-label={`Summary of ${col.label}`} class="choices">
			{#each summaryChoices(col) as fn (fn)}
				<button
					type="button"
					role="menuitemradio"
					aria-checked={fn === current}
					onclick={() => chooseSummary(col, fn)}
					onkeydown={(e) => {
						if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
						e.preventDefault();
						const all = [
							...(e.currentTarget.parentElement?.querySelectorAll<HTMLElement>('button') ?? [])
						];
						const i = all.indexOf(e.currentTarget);
						all[(i + (e.key === 'ArrowDown' ? 1 : all.length - 1)) % all.length]?.focus();
					}}>{SUMMARY_LABELS[fn]}</button
				>
			{/each}
		</div>
	{/if}
</div>

<style>
	.scroll {
		/* Holds the hidden labels in the cells, which would otherwise stretch
		   the page behind it as tall as the table. */
		position: relative;
		overflow: auto;
		height: 100%;
		/* Room under the last group for a menu to open into. */
		padding-bottom: var(--space-6);
	}
	table {
		border-collapse: separate;
		border-spacing: 0;
		table-layout: fixed;
		min-width: 100%;
		font-size: var(--text-sm);
	}
	table.dragging {
		user-select: none;
	}
	th,
	td {
		height: var(--row-height);
		padding: 0;
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-0);
		text-align: left;
		overflow: hidden;
	}
	tr.picked td {
		background: var(--row-selected-bg);
	}
	.pick {
		display: grid;
		flex: none;
		place-items: center;
		width: calc(22px * var(--ui-scale));
		height: calc(22px * var(--ui-scale));
		padding: 0;
		border: none;
		background: none;
		cursor: pointer;
	}
	.pick .dot {
		display: grid;
		place-items: center;
		width: calc(16px * var(--ui-scale));
		height: calc(16px * var(--ui-scale));
		border: 1.5px solid var(--border-control);
		border-radius: 50%;
		color: var(--checkbox-mark);
	}
	tr.picked .pick .dot {
		border-color: var(--checkbox-on-bg);
		background: var(--checkbox-on-bg);
	}
	th:focus-visible,
	td:focus-visible {
		outline: none;
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	thead th {
		position: sticky;
		top: 0;
		z-index: 3;
		height: calc(34px * var(--control-scale));
		border-bottom-color: var(--border-strong);
		background: var(--surface-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	.sticky {
		position: sticky;
		left: 0;
		z-index: 2;
	}
	thead th.sticky {
		z-index: 4;
	}
	.head {
		display: flex;
		align-items: center;
		gap: 4px;
		height: 100%;
		padding: 0 var(--space-3);
		white-space: nowrap;
		touch-action: none;
	}
	.head.sortable {
		cursor: pointer;
	}
	.head.movable {
		cursor: grab;
	}
	.head:hover {
		background: var(--surface-2);
		color: var(--text-primary);
	}
	.head .label {
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.arrow {
		display: inline-flex;
		align-items: center;
		color: var(--accent-text);
	}
	.arrow sup {
		font-size: 9px;
		font-weight: 700;
	}
	th.lifted {
		background: var(--accent-soft);
	}
	.grip {
		position: absolute;
		top: 6px;
		right: 0;
		bottom: 6px;
		width: 7px;
		border-right: 2px solid transparent;
		cursor: col-resize;
		touch-action: none;
	}
	th {
		position: relative;
	}
	thead th {
		position: sticky;
	}
	/* A heading pointed at shows where its edge can be dragged. */
	th:hover .grip {
		border-right-color: var(--border-strong);
	}
	.grip:hover,
	.grip.on {
		border-right-color: var(--accent);
	}

	.group td {
		height: auto;
		padding: var(--space-4) var(--space-3) var(--space-1);
		border-bottom: none;
	}
	.ghead {
		position: sticky;
		left: var(--space-3);
		display: flex;
		align-items: center;
		gap: var(--space-2);
		width: max-content;
		font-weight: 650;
	}
	.ghead h2 {
		font-family: var(--font-ui);
		font-size: var(--text-sm);
		font-weight: inherit;
	}
	.ghead.danger {
		color: var(--danger);
	}
	.count {
		font-size: var(--text-xs);
		font-weight: 500;
		color: var(--text-secondary);
	}

	tr[data-key]:hover > td {
		background: var(--surface-1);
	}
	tr.lifted > td {
		opacity: 0.45;
	}
	.cell {
		padding: 0 var(--space-3);
		cursor: pointer;
	}
	.cell:hover {
		box-shadow: inset 0 0 0 1px var(--border-strong);
	}
	.cell.open {
		box-shadow: inset 0 0 0 2px var(--accent);
	}
	.titlecell {
		display: flex;
		align-items: center;
		gap: 6px;
		height: 100%;
		padding-right: var(--space-2);
	}
	.handle,
	.twisty,
	.pencil {
		flex: none;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 20px;
		height: 22px;
		padding: 0;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-muted);
	}
	.handle {
		width: 16px;
		cursor: grab;
		opacity: 0;
		touch-action: none;
	}
	.handle[aria-disabled='true'] {
		cursor: not-allowed;
	}
	tr:hover .handle,
	td:focus-visible .handle {
		opacity: 1;
	}
	.twisty {
		color: var(--text-secondary);
		cursor: pointer;
	}
	.twisty:hover {
		background: var(--surface-3);
	}
	.twisty .closed {
		display: inline-flex;
		transform: rotate(-90deg);
	}
	.twisty-gap {
		flex: none;
		width: 20px;
	}
	.titletext {
		flex: 1;
		min-width: 0;
		padding: 0;
		border: none;
		background: none;
		color: var(--text-primary);
		font: inherit;
		text-align: left;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		cursor: pointer;
	}
	.done .titletext {
		color: var(--text-muted);
		text-decoration: line-through;
		text-decoration-thickness: 1px;
	}
	.subcount {
		flex: none;
		padding: 0 5px;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.pencil {
		opacity: 0;
		cursor: pointer;
	}
	tr:hover .pencil,
	td:focus-visible .pencil {
		opacity: 1;
	}
	.pencil:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	.rename {
		flex: 1;
		min-width: 0;
		padding: 2px var(--space-2);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-sm);
		background: var(--input-bg);
		color: var(--input-text);
	}

	.empty-group td {
		height: calc(34px * var(--control-scale));
		padding: 0 var(--space-3) 0 calc(var(--space-6) + var(--space-4));
		border: var(--border-width) dashed var(--border-strong);
		color: var(--text-muted);
	}

	.sum td {
		height: calc(30px * var(--control-scale));
		padding: 0 var(--space-3);
		background: var(--surface-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		white-space: nowrap;
		cursor: pointer;
	}
	.sum td:hover {
		box-shadow: inset 0 0 0 1px var(--border-strong);
		color: var(--text-primary);
	}
	.sum td.end {
		text-align: right;
	}
	.sum .add {
		opacity: 0;
		color: var(--text-muted);
	}
	.sum td:hover .add,
	.sum td:focus-visible .add {
		opacity: 1;
	}
	.bar {
		display: flex;
		width: 100%;
		height: 8px;
		border-radius: var(--radius-full);
		overflow: hidden;
		background: var(--surface-3);
	}
	.bar span {
		height: 100%;
	}

	.dropline {
		position: fixed;
		z-index: 50;
		height: 2px;
		border-radius: 1px;
		background: var(--accent);
		pointer-events: none;
	}
	.dropline.vertical {
		width: 2px;
	}

	.table-picker {
		padding: var(--space-1);
	}
	.choices {
		display: grid;
		gap: 1px;
	}
	.choices button {
		min-height: max(var(--tap-min), 30px);
		padding: 0 var(--space-2);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-primary);
		font-size: var(--text-sm);
		text-align: left;
		cursor: pointer;
	}
	.choices button:hover,
	.choices button:focus-visible {
		background: var(--surface-2);
	}
	.choices button[aria-checked='true'] {
		font-weight: 600;
		color: var(--accent-text);
	}
</style>
