<script lang="ts">
	import { menu, taskActions } from '../../contextMenu.svelte';
	import { focusOnShow } from '../../focus';
	import { announce } from '../../announce.svelte';
	import { liftDragImage } from '../../drag';
	import { uuid } from '../../uuid';
	import { collapse, flip } from '../../motion';
	import { createTaskIn, placeTask, writeColumn } from '../../sync/data';
	import { formatDuration, sayDuration } from '../../duration';
	import { engine } from '../../sync/engine.svelte';
	import type { Task } from '../../sync/entities';
	import { openTask } from '../../taskLink';
	import type { CommandId } from '../../keys/commands';
	import { keymap } from '../../keys/keymap.svelte';
	import {
		ACTS_ON_MANY,
		SELECT_COMMANDS,
		addsToSelection,
		clickSelects,
		commandSelects,
		sayCount,
		selection
	} from '../../selection.svelte';
	import { toasts } from '../../toasts.svelte';
	import type { BoardColumn, ColumnWrite } from '../../viewDisplay';
	import Check from '@lucide/svelte/icons/check';
	import PriorityIcon from '../PriorityIcon.svelte';
	import StatusIcon from '../StatusIcon.svelte';
	import Avatar from '../task/Avatar.svelte';
	import Splitter from '../Splitter.svelte';
	import { BOARD_COLUMN, layout } from '../../layout.svelte';
	import TaskCardBody from './TaskCardBody.svelte';

	interface Props {
		ws: string;
		/** The project, or null for a board across projects (given its tasks and columns). */
		projectId: string | null;
		/** Narrows the cards, for a saved view. */
		show?: (task: Task) => boolean;
		/** The cards, filtered and in order (a saved view's); else the project's top-level tasks. */
		tasks?: Task[];
		/** The columns (a saved view's, from boardColumns); else one per status. */
		columns?: BoardColumn[];
		/** False when the cards are sorted: moving within a column then does nothing. */
		manual?: boolean;
		/** What the cards are sorted by, in words, said when a card cannot move up or down. */
		sortedBy?: string;
		/** The fields cards show; without it, the usual ones. */
		shown?: Set<string>;
		/** Why cards cannot move up or down, when that is not their sort (across projects). */
		fixed?: string;
		/**
		 * Makes a task added to a column; null for none. Without it, the task goes
		 * in the project, and a board across projects offers none.
		 */
		addTask?: ((title: string, write: ColumnWrite) => Promise<unknown>) | null;
	}

	let {
		ws,
		projectId,
		show,
		tasks: given,
		columns: spec,
		manual = true,
		sortedBy,
		shown,
		fixed,
		addTask
	}: Props = $props();

	const adds = $derived(
		addTask === undefined
			? projectId === null
				? null
				: (t: string, write: ColumnWrite) => createTaskIn(ws, projectId, t, write)
			: addTask
	);

	const replica = $derived(engine.replica(ws));
	// A column's time, when cards show it: what its tasks have logged and estimated.
	const timeShown = $derived(!shown || shown.has('estimate') || shown.has('logged'));
	function columnTime(list: Task[]) {
		let estimate = 0;
		let logged = 0;
		for (const t of list) {
			estimate += t.estimateMinutes ?? 0;
			logged += replica?.logged(t.id) ?? 0;
		}
		return { estimate, logged };
	}
	// Without columns given, one per status, as boardColumns makes them; a
	// task whose status is gone is in the first. (Not imported from there,
	// so a project page does not load the sorting and grouping.)
	const specs = $derived.by((): BoardColumn[] => {
		if (spec) return spec;
		const statuses = projectId ? (replica?.statuses(projectId) ?? []) : [];
		const known = new Set(statuses.map((s) => s.id));
		return statuses.map((s, i) => ({
			id: `s:${s.id}`,
			title: s.name,
			icon: { kind: 'status', category: s.category },
			has: (t: Task) => t.statusId === s.id || (i === 0 && !known.has(t.statusId ?? '')),
			write: { status: s.id }
		}));
	});
	const columns = $derived.by(() => {
		const tasks = given ?? (replica?.tasks(projectId) ?? []).filter((t) => !show || show(t));
		const out = specs.map((column) => ({ column, tasks: [] as Task[] }));
		for (const t of tasks) out.find((c) => c.column.has(t))?.tasks.push(t);
		return out;
	});

	// Selecting several: the cards in board order,
	// column by column.
	const order = () => columns.flatMap((c) => c.tasks.map((t) => t.id));
	$effect(() => selection.register(order));
	const selecting = $derived(selection.on(ws));

	function cardClick(e: MouseEvent, task: Task) {
		// The check on a card, plainly clicked, adds it or takes it out.
		const onMark = (e.target as Element).closest('.pickmark') && !e.shiftKey && !addsToSelection(e);
		if (onMark) selection.toggle(ws, task.id);
		if (onMark || clickSelects(e, ws, task.id, order)) {
			announce(sayCount());
			return;
		}
		void openTask(task.id);
	}

	/** Whether a card can go into a column: one that writes nothing (Overdue) takes none from elsewhere. */
	const takes = (column: BoardColumn, task: Task | undefined) =>
		!!column.write || (!!task && column.has(task));
	const refused = (column: BoardColumn) =>
		announce(`Cards can't move into ${column.title}. Choose another column.`);

	/** The selected cards a drag of this one carries: all of them when it is one. */
	function carried(id: string): string[] {
		if (!selection.has(ws, id)) return [id];
		const picked = selection.shown();
		return picked.length > 1 ? picked : [id];
	}

	/** Puts several cards in a column, in order, as one change with one undo. */
	async function moveMany(ids: string[], columnId: string, index: number) {
		const to = columns.find((c) => c.column.id === columnId);
		if (!to) return;
		const write = to.column.write;
		if (!write) return refused(to.column);
		const all = columns.flatMap((c) => c.tasks);
		const moving = ids.map((id) => all.find((t) => t.id === id)).filter((t): t is Task => !!t);
		const others = to.tasks.filter((t) => !ids.includes(t.id));
		let before = others[index - 1]?.sortKey ?? null;
		const after = others[index]?.sortKey ?? null;
		const { done, undo } = engine.batch(ws, async () => {
			for (const t of moving) {
				await writeColumn(ws, t, write);
				if (manual) {
					await placeTask(ws, t.id, before, after === before ? null : after);
					before = engine.replica(ws)?.task(t.id)?.sortKey ?? before;
				}
			}
		});
		await done;
		toasts.show(`Moved ${moving.length} tasks to ${to.column.title}`, {
			onundo: () => void undo()
		});
	}

	// Dragging: the card, and where it would land.
	let dragging = $state<string | null>(null);
	let target = $state<{ column: string; index: number } | null>(null);

	function landingIndex(e: DragEvent, column: HTMLElement): number {
		const cards = [...column.querySelectorAll<HTMLElement>('[data-card]')].filter(
			(c) => c.dataset.card !== dragging
		);
		const y = e.clientY;
		const i = cards.findIndex((c) => {
			const r = c.getBoundingClientRect();
			return y < r.top + r.height / 2;
		});
		return i < 0 ? cards.length : i;
	}

	/**
	 * Puts a task in a column: what the column stands for (a status, a
	 * priority, a field's value), and in manual order a sort key between
	 * its new neighbors.
	 */
	async function moveTo(task: Task, columnId: string, index: number) {
		const to = columns.find((c) => c.column.id === columnId);
		if (!to) return false;
		if (!takes(to.column, task)) {
			refused(to.column);
			return false;
		}
		const write = to.column.write;
		const others = to.tasks.filter((t) => t.id !== task.id);
		const before = others[index - 1]?.sortKey ?? null;
		const after = others[index]?.sortKey ?? null;
		await engine.group(async () => {
			// Writes only what differs: a task whose status is gone, shown in
			// the first column, gets that column's status.
			if (write) await writeColumn(ws, task, write);
			if (manual) await placeTask(ws, task.id, before, after === before ? null : after);
		});
		return true;
	}

	function drop(e: DragEvent, columnId: string) {
		e.preventDefault();
		const task = columns.flatMap((c) => c.tasks).find((t) => t.id === dragging);
		const many = dragging ? carried(dragging) : [];
		if (task && target && many.length > 1) void moveMany(many, columnId, target.index);
		else if (task && target) void moveTo(task, columnId, target.index);
		dragging = null;
		target = null;
	}

	// The board is one stop for Tab (a roving tabindex): the card last
	// focused, else the first. Arrows walk between cards and columns.
	let current = $state<string | null>(null);
	const helpId = `board-help-${uuid()}`;
	const active = $derived.by(() => {
		const all = columns.flatMap((c) => c.tasks);
		return all.find((t) => t.id === current)?.id ?? all[0]?.id ?? null;
	});

	function focusCard(id: string) {
		requestAnimationFrame(() =>
			document.querySelector<HTMLElement>(`[data-card="${CSS.escape(id)}"]`)?.focus()
		);
	}

	/** The card an arrow key goes to from one: up and down in its column, sideways to the nearest card in the next column that has any. */
	function neighbor(key: string, col: number, index: number): Task | undefined {
		if (key === 'ArrowUp') return columns[col].tasks[index - 1];
		if (key === 'ArrowDown') return columns[col].tasks[index + 1];
		if (key === 'Home') return columns[col].tasks[0];
		if (key === 'End') return columns[col].tasks.at(-1);
		const step = key === 'ArrowLeft' ? -1 : key === 'ArrowRight' ? 1 : 0;
		if (!step) return undefined;
		for (let c = col + step; c >= 0 && c < columns.length; c += step) {
			const tasks = columns[c].tasks;
			if (tasks.length) return tasks[Math.min(index, tasks.length - 1)];
		}
		return undefined;
	}

	// The commands that move a card, and which way.
	const MOVES: Partial<Record<CommandId, 'left' | 'right' | 'up' | 'down'>> = {
		'board.move-left': 'left',
		'board.move-right': 'right',
		'board.move-up': 'up',
		'board.move-down': 'down'
	};

	// Keyboard: the move commands (Alt and the arrows, unless changed) move
	// a card, between columns or within one; Enter opens it; arrows move
	// focus.
	function cardKeys(e: KeyboardEvent, task: Task, col: number, index: number) {
		// Selecting, and the task keys on a selection, are a list's keys on a card too.
		const listCmd = keymap.match('list', e);
		if (listCmd && SELECT_COMMANDS.includes(listCmd)) {
			const to = commandSelects(listCmd, ws, task.id, order());
			if (to === null) return;
			e.preventDefault();
			if (to !== task.id) {
				current = to;
				focusCard(to);
			}
			announce(sayCount());
			return;
		}
		if (listCmd && ACTS_ON_MANY.includes(listCmd) && selection.on(ws) && selection.shown().length) {
			e.preventDefault();
			void import('../../manyActions').then((m) => m.runCommand(ws, listCmd));
			return;
		}
		const cmd = keymap.match('board', e);
		const way = cmd && MOVES[cmd];
		if (way) {
			// Taken even at an edge, so a key kept on both the board and
			// anywhere never does both.
			e.preventDefault();
			moveCard(way, task, col, index);
			return;
		}
		if (e.key === 'Enter') {
			e.preventDefault();
			void openTask(task.id);
			return;
		}
		if (!e.altKey) {
			const to = neighbor(e.key, col, index);
			if (to) {
				e.preventDefault();
				current = to.id;
				focusCard(to.id);
			}
		}
	}

	function moveCard(way: 'left' | 'right' | 'up' | 'down', task: Task, col: number, index: number) {
		const here = columns[col];
		if (!manual && (way === 'up' || way === 'down')) {
			announce(
				fixed ?? `${sortedBy ?? 'Sorted'}. Choose manual order to move cards within a column.`
			);
			return;
		}
		let next: { column: string; index: number } | null = null;
		if (way === 'left' && col > 0)
			next = { column: columns[col - 1].column.id, index: columns[col - 1].tasks.length };
		if (way === 'right' && col < columns.length - 1)
			next = { column: columns[col + 1].column.id, index: columns[col + 1].tasks.length };
		if (way === 'up' && index > 0) next = { column: here.column.id, index: index - 1 };
		if (way === 'down' && index < here.tasks.length - 1)
			next = { column: here.column.id, index: index + 1 };
		if (!next) return;
		const to = next;
		void moveTo(task, to.column, to.index).then((moved) => {
			if (!moved) return;
			const column = columns.find((c) => c.column.id === to.column);
			const at = (column?.tasks.findIndex((t) => t.id === task.id) ?? to.index) + 1;
			announce(
				`Moved ${task.title} to ${column?.column.title ?? 'another column'}, ${at} of ${column?.tasks.length ?? at}`
			);
			focusCard(task.id);
		});
	}

	// Each column's width as drawn, which a resize starts from until one is set.
	let drawn = $state<number[]>([]);

	let adding = $state<string | null>(null);
	let title = $state('');
	function add(e: SubmitEvent, column: BoardColumn) {
		e.preventDefault();
		const t = title.trim();
		title = '';
		if (!t) {
			adding = null;
			return;
		}
		if (column.write) void adds?.(t, column.write);
	}
</script>

<p id={helpId} hidden>
	Arrow keys go between cards. {keymap.describe(
		['board.move-left', 'board.move-right', 'board.move-up', 'board.move-down'],
		'move this one'
	)}
</p>
<div
	class="board"
	role="group"
	aria-label="Board"
	style:--column-width={layout.boardColumnWidth ? `${layout.boardColumnWidth}px` : undefined}
>
	{#each columns as { column, tasks }, col (column.id)}
		<section
			class="column"
			class:over={target?.column === column.id}
			aria-label={column.title}
			bind:clientWidth={drawn[col]}
			ondragover={(e) => {
				if (!dragging) return;
				const card = columns.flatMap((c) => c.tasks).find((t) => t.id === dragging);
				if (!takes(column, card)) return;
				e.preventDefault();
				const index = landingIndex(e, e.currentTarget as HTMLElement);
				// dragover fires constantly; only a new spot changes anything.
				if (target?.column !== column.id || target.index !== index)
					target = { column: column.id, index };
			}}
			ondragleave={(e) => {
				if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node)) target = null;
			}}
			ondrop={(e) => drop(e, column.id)}
		>
			<!-- Every column is one width, kept on this device; the edge after any
			     column sets it, and the first is the one Tab reaches. -->
			<span class="edge">
				<Splitter
					label="Resize the board's columns"
					value={layout.boardColumnWidth ?? drawn[0] ?? BOARD_COLUMN.initial}
					min={BOARD_COLUMN.min}
					max={BOARD_COLUMN.max}
					grows="right"
					share={col + 1}
					tabbable={col === 0}
					onreset={() => layout.setBoardColumnWidth(null)}
					onresize={(px) => layout.setBoardColumnWidth(px)}
				/>
			</span>
			<header
				use:menu={column.id.startsWith('s:') && projectId
					? () =>
							import('../../menus').then((m) =>
								m.statusMenu(
									ws,
									projectId,
									column.id.slice(2),
									column.title,
									() => (adding = column.id)
								)
							)
					: () => null}
			>
				{#if column.icon?.kind === 'status'}<StatusIcon category={column.icon.category} />
				{:else if column.icon?.kind === 'priority'}<PriorityIcon priority={column.icon.priority} />
				{:else if column.icon?.kind === 'color'}<span
						class="dot"
						aria-hidden="true"
						style:--chip={`var(--palette-${column.icon.color})`}
					></span>
				{:else if column.icon?.kind === 'person'}<Avatar name={column.icon.name} size={18} />{/if}
				<h2>{column.title}</h2>
				<span class="count tabular">{tasks.length}</span>
				{#if timeShown}
					{@const t = columnTime(tasks)}
					{#if t.estimate || t.logged}<span
							class="count tabular time"
							title="Time logged and estimated"
							><span aria-hidden="true"
								>{formatDuration(t.logged)}{t.estimate
									? ` / ${formatDuration(t.estimate)}`
									: ''}</span
							><span class="visually-hidden"
								>{sayDuration(t.logged)} logged{t.estimate
									? ` of ${sayDuration(t.estimate)} estimated`
									: ''}</span
							></span
						>{/if}
				{/if}
			</header>
			<ol>
				{#each tasks as task, i (task.id)}
					<li animate:flip>
						<!-- Where the card would land: it opens up, and the cards below
						     slide aside. A sorted board has no place to choose. -->
						{#if manual && target?.column === column.id && target.index === i}<div
								class="slot"
								transition:collapse
							></div>{/if}
						<div
							class="card"
							class:picked={selection.has(ws, task.id)}
							class:lifted={dragging === task.id ||
								(dragging !== null && carried(dragging).includes(task.id))}
							role="button"
							tabindex={task.id === active ? 0 : -1}
							draggable="true"
							data-card={task.id}
							use:menu={taskActions(ws, task.id)}
							aria-describedby={helpId}
							onfocus={() => (current = task.id)}
							onclick={(e) => cardClick(e, task)}
							onmousedown={(e) => e.shiftKey && e.preventDefault()}
							onkeydown={(e) => cardKeys(e, task, col, i)}
							ondragstart={(e) => {
								dragging = task.id;
								const many = carried(task.id).length;
								e.dataTransfer?.setData('text/plain', many > 1 ? `${many} tasks` : task.title);
								liftDragImage(e, e.currentTarget as HTMLElement);
							}}
							ondragend={() => {
								dragging = null;
								target = null;
							}}
						>
							{#if selecting}<span class="pickmark" aria-hidden="true"
									>{#if selection.has(ws, task.id)}<Check size={11} strokeWidth={3} />{/if}</span
								>{/if}
							<TaskCardBody {ws} {task} {shown} />
							{#if selection.has(ws, task.id)}<span class="visually-hidden">, selected</span>{/if}
						</div>
					</li>
				{/each}
				{#if manual && target?.column === column.id && target.index >= tasks.length}<div
						class="slot"
						transition:collapse
					></div>{/if}
			</ol>
			{#if !adds || !column.write}
				<!-- Nowhere to add a task: it would not be in what the board shows. -->
			{:else if adding === column.id}
				<form onsubmit={(e) => add(e, column)}>
					<input
						aria-label={`New task in ${column.title}`}
						placeholder="Task title"
						bind:value={title}
						use:focusOnShow
						onblur={() => !title && (adding = null)}
						onkeydown={(e) => {
							// Handled here, so the open task stays open (TaskDetail).
							if (e.key === 'Escape') {
								e.preventDefault();
								adding = null;
							}
						}}
					/>
				</form>
			{:else}
				<button
					title="Add a task to this column"
					class="add"
					type="button"
					onclick={() => (adding = column.id)}>+ Add a task</button
				>
			{/if}
		</section>
	{/each}
</div>

<style>
	.board {
		display: grid;
		grid-auto-columns: var(--column-width, minmax(260px, 300px));
		grid-auto-flow: column;
		gap: var(--space-3);
		align-items: start;
		height: 100%;
		padding: var(--space-4);
		overflow-x: auto;
	}
	.column {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		max-height: 100%;
		padding: var(--space-2);
		border-radius: var(--radius-lg);
		background: var(--board-column-bg);
		transition: background-color var(--dur-micro) var(--ease);
	}
	/* In the gap after the column, where its edge is dragged. */
	.edge {
		position: absolute;
		top: 0;
		right: calc(var(--space-3) / -2);
		bottom: 0;
	}
	.column.over {
		background: var(--accent-soft);
	}
	header {
		/* Holds its hidden words, which would otherwise stretch the page. */
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-1) var(--space-2);
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--chip);
	}
	h2 {
		font-family: var(--font-ui);
		font-size: var(--text-sm);
		font-weight: 650;
	}
	.count {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.time {
		margin-left: auto;
	}
	ol {
		display: grid;
		gap: var(--space-2);
		min-height: 24px;
		margin: 0;
		padding: 0;
		overflow-y: auto;
		list-style: none;
	}
	.card {
		display: grid;
		gap: var(--space-2);
		padding: calc(var(--space-3) * var(--control-scale));
		border: var(--border-width) solid var(--card-border);
		border-radius: var(--card-radius);
		background: var(--card-bg);
		box-shadow: var(--card-shadow);
		position: relative;
		cursor: grab;
		transition:
			box-shadow var(--dur-panel) var(--ease),
			transform var(--dur-panel) var(--ease-spring),
			opacity var(--dur-micro) var(--ease);
	}
	/* Selecting several: room at a card's start for its check. */
	.card:has(.pickmark) {
		padding-left: calc(var(--space-3) * var(--control-scale) + 22px * var(--ui-scale));
	}
	.pickmark {
		position: absolute;
		top: calc(var(--space-3) * var(--control-scale) + 1px);
		left: calc(var(--space-3) * var(--control-scale));
		display: grid;
		place-items: center;
		width: calc(16px * var(--ui-scale));
		height: calc(16px * var(--ui-scale));
		border: 1.5px solid var(--border-control);
		border-radius: 50%;
		color: var(--checkbox-mark);
		cursor: pointer;
	}
	.card.picked {
		border-color: var(--accent);
		background: var(--row-selected-bg);
	}
	.card.picked .pickmark {
		border-color: var(--checkbox-on-bg);
		background: var(--checkbox-on-bg);
	}
	.card:hover {
		border-color: var(--border-strong);
	}
	.card:focus-visible {
		border-color: var(--accent);
		box-shadow: var(--focus-ring);
	}
	/* The card being dragged stays behind as an outline of itself. */
	.card.lifted {
		border-style: dashed;
		box-shadow: none;
		opacity: 0.45;
	}
	.slot {
		height: 44px;
		margin-bottom: var(--space-2);
		border: 2px dashed var(--accent);
		border-radius: var(--card-radius);
	}
	.add {
		padding: var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		background: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		text-align: left;
		cursor: pointer;
	}
	.add:hover {
		background: var(--surface-3);
		color: var(--text-primary);
	}
	form input {
		width: 100%;
		padding: var(--space-2);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-md);
		background: var(--surface-0);
	}
	@media (max-width: 720px) {
		.board {
			grid-auto-columns: 85vw;
			scroll-snap-type: x mandatory;
			padding: var(--space-3);
		}
		.column {
			scroll-snap-align: center;
		}
		/* A phone's columns are its screen's width. */
		.edge {
			display: none;
		}
	}
</style>
