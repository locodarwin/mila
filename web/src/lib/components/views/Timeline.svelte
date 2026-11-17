<script lang="ts">
	// The Timeline view: each task a row, a bar
	// from its start to its due date (a diamond for one date), arrows from
	// what it waits for, and an Unscheduled tray for tasks with no dates.
	// Bars drag, their ends stretch, the dots beside them make a task wait
	// for another, and the keyboard does the same on the focused row. On a
	// phone the titles ride on the bars. Loaded only when a timeline shows.
	import { menu, taskActions } from '../../contextMenu.svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import GripVertical from '@lucide/svelte/icons/grip-vertical';
	import Avatar from '../task/Avatar.svelte';
	import BlockedMark from '../BlockedMark.svelte';
	import EmptyState from '../EmptyState.svelte';
	import RepeatIcon from '../RepeatIcon.svelte';
	import { announce } from '../../announce.svelte';
	import { formatDate, formatMonth, formatMonthShort, weekStart } from '../../dates';
	import { timeBadge } from '../../duration';
	import { conflicts, finishOf } from '../../dependencies';
	import { keymap } from '../../keys/keymap.svelte';
	import { repeatLabel, tryParseRule, upcoming } from '../../recurrence';
	import { session } from '../../session.svelte';
	import {
		dependencyProblem,
		localToday,
		setDependency,
		setTaskFields,
		shiftDates,
		waitingChain
	} from '../../sync/data';
	import { engine } from '../../sync/engine.svelte';
	import type { Task } from '../../sync/entities';
	import { openTask } from '../../taskLink';
	import {
		DAY_PX,
		ZOOMS,
		arrowPath,
		dayOf,
		isoOf,
		rangeOf,
		rowsOf,
		scaleOf,
		shapeOf,
		spanUnder,
		stepped,
		stretched,
		shifted,
		type Row,
		type Shape,
		type Zoom
	} from '../../timeline';
	import { toasts } from '../../toasts.svelte';
	import { Growing } from '../../growing.svelte';
	import { TIMELINE_NAMES, layout } from '../../layout.svelte';
	import Splitter from '../Splitter.svelte';

	interface Props {
		ws: string;
		sections: { id: string; title: string; tasks: Task[] }[];
		/** A task's subtasks as the view shows them. */
		kids: (task: Task) => Task[];
		/** Whether the sections are groups with headings. */
		grouped: boolean;
		zoom: Zoom;
		onzoom: (zoom: Zoom) => void;
		/** A phone: Week and Month only, titles on the bars, no dots. */
		phone?: boolean;
		label: string;
		emptyTitle: string;
		emptyHint: string;
	}

	let {
		ws,
		sections,
		kids,
		grouped,
		zoom,
		onzoom,
		phone = false,
		label,
		emptyTitle,
		emptyHint
	}: Props = $props();

	const replica = $derived(engine.replica(ws));
	const today = localToday();
	// The task names' width, kept on this device; a phone shows none beside the bars.
	const lead = $derived(phone ? 0 : layout.timelineNamesWidth);
	const rowH = $derived(phone ? 44 : 36);
	const zooms = $derived(phone ? ZOOMS.filter((z) => z.id === 'week' || z.id === 'month') : ZOOMS);
	const shownZoom = $derived<Zoom>(phone && (zoom === 'day' || zoom === 'quarter') ? 'week' : zoom);
	const px = $derived(DAY_PX[shownZoom]);

	// Folded parents, kept on this device.
	const FOLDED = 'mila.timelineFolded';
	const folded = new SvelteSet<string>(readFolded());
	function readFolded(): string[] {
		try {
			const v = JSON.parse(localStorage.getItem(FOLDED) ?? '[]');
			return Array.isArray(v) ? v.filter((x) => typeof x === 'string').slice(-500) : [];
		} catch {
			return [];
		}
	}
	function fold(id: string, shut: boolean) {
		if (shut) folded.add(id);
		else folded.delete(id);
		try {
			localStorage.setItem(FOLDED, JSON.stringify([...folded]));
		} catch {
			// Kept for this visit only.
		}
	}

	const laid = $derived(rowsOf(sections, kids, folded, grouped));
	const growing = new Growing(() => laid.rows.length);
	const rows = $derived(laid.rows.slice(0, growing.limit));
	const taskRows = $derived(
		rows.filter((r): r is Extract<Row, { kind: 'task' }> => r.kind === 'task')
	);

	// A drag in progress shows its dates before they are saved.
	let draft = $state<{ id: string; startDate: string | null; dueDate: string | null } | null>(null);
	const datesOf = (t: Task) => (draft?.id === t.id ? { ...t, ...draft } : t);

	// The span is worked out from the saved dates, so it holds still while
	// a bar is dragged.
	const range = $derived.by(() => {
		const dates: string[] = [];
		for (const r of laid.rows)
			if (r.kind === 'task') {
				if (r.task.startDate) dates.push(r.task.startDate);
				if (r.task.dueDate) dates.push(r.task.dueDate);
			}
		return rangeOf(dates, today, shownZoom, weekStart());
	});
	const width = $derived((range.to - range.from + 1) * px);
	const scale = $derived(scaleOf(range.from, range.to, shownZoom, weekStart()));
	const x = (day: number) => (day - range.from) * px;

	const shapeFor = (t: Task): Shape | null => shapeOf(datesOf(t), range.from, px);

	// Where each task is: its row, or its folded parent's.
	const indexOf = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- made afresh by each derivation
		const m = new Map<string, number>();
		rows.forEach((r, i) => {
			if (r.kind === 'task') m.set(r.task.id, i);
		});
		return m;
	});
	function rowOfTask(id: string): number | undefined {
		let at: string | null = id;
		for (let depth = 0; at && depth < 20; depth++) {
			const i = indexOf.get(at);
			if (i !== undefined) return i;
			at = replica?.task(at)?.parentId ?? null;
		}
		return undefined;
	}

	const categoryOf = (t: Task) =>
		replica?.status(t.statusId)?.category ?? (t.completedAt ? 'done' : 'todo');
	const tone = (t: Task) => {
		const c = categoryOf(t);
		return c === 'done' || c === 'canceled' ? 'done' : c === 'in-progress' ? 'progress' : 'todo';
	};

	const arrows = $derived.by(() => {
		const out: { key: string; d: string; bad: boolean; head: string }[] = [];
		for (const [a, b] of replica?.dependencies() ?? []) {
			const ia = rowOfTask(a);
			const ib = rowOfTask(b);
			const ta = replica?.task(a);
			const tb = replica?.task(b);
			if (ia === undefined || ib === undefined || ia === ib || !ta || !tb) continue;
			const sa = shapeFor(ta);
			const sb = shapeFor(tb);
			if (!sa || !sb) continue;
			const y1 = ia * rowH + rowH / 2;
			const y2 = ib * rowH + rowH / 2;
			const x2 = sb.left;
			out.push({
				key: `${a}>${b}`,
				d: arrowPath(sa.right, y1, x2, y2, rowH),
				bad: conflicts(datesOf(ta) as Task, datesOf(tb) as Task),
				head: `M${x2} ${y2} l-6 -4 v8 z`
			});
		}
		return out;
	});

	// Where repeating tasks fall next, as faint diamonds.
	function ghosts(t: Task): string[] {
		const rule = tryParseRule(t.recurrence);
		if (!rule || !t.dueDate || t.completedAt || t.recurrenceMode !== 'schedule') return [];
		const after = isoOf(dayOf(t.dueDate) + 1);
		return upcoming(rule, t.dueDate, after > today ? after : today, isoOf(range.to), 4);
	}

	function datesSaid(t: { startDate: string | null; dueDate: string | null }): string {
		if (t.startDate && t.dueDate)
			return t.startDate === t.dueDate
				? formatDate(t.dueDate)
				: `${formatDate(t.startDate)} to ${formatDate(t.dueDate)}`;
		if (t.dueDate) return `due ${formatDate(t.dueDate)}`;
		if (t.startDate) return `starts ${formatDate(t.startDate)}`;
		return 'no dates';
	}

	function rowLabel(t: Task): string {
		const waits = replica?.openBlockers(t.id) ?? [];
		const time = timeBadge(t.estimateMinutes, replica?.logged(t.id) ?? 0, {
			estimate: true,
			logged: true,
			remaining: false
		});
		return `${t.title}, ${datesSaid(t)}${waits.length ? `, waits for ${waits.join(' and ')}` : ''}${time ? `, ${time.said}` : ''}`;
	}

	// Scrolling: today a quarter of the way in at first, and the middle
	// kept when the zoom changes.
	let scroller = $state<HTMLDivElement>();
	let centre: number | null = null;
	function scrollToDay(day: number, at: number, smooth = false) {
		if (!scroller) return;
		const seen = scroller.clientWidth - lead;
		scroller.scrollTo({
			left: Math.max(0, x(day) - seen * at),
			behavior:
				smooth && !matchMedia('(prefers-reduced-motion: reduce)').matches ? 'smooth' : 'auto'
		});
	}
	function middleDay(): number {
		if (!scroller) return dayOf(today);
		return range.from + Math.round((scroller.scrollLeft + (scroller.clientWidth - lead) / 2) / px);
	}
	let placed = false;
	$effect(() => {
		void shownZoom;
		void px;
		if (!scroller) return;
		const at = centre;
		centre = null;
		requestAnimationFrame(() => {
			if (at !== null) scrollToDay(at, 0.5);
			else if (!placed) scrollToDay(dayOf(today), 0.25);
			placed = true;
		});
	});
	function setZoom(z: Zoom) {
		if (z === shownZoom) return;
		centre = middleDay();
		onzoom(z);
	}

	// The row that takes Tab: the focused one, or the first.
	let active = $state<string | null>(null);
	const current = $derived(
		active && taskRows.some((r) => r.task.id === active) ? active : (taskRows[0]?.task.id ?? null)
	);
	function focusRow(id: string) {
		active = id;
		requestAnimationFrame(() =>
			scroller?.querySelector<HTMLElement>(`[data-row="${CSS.escape(id)}"]`)?.focus()
		);
	}

	/** Saves a task's new dates, then offers to move what waits for it if it now ends later. */
	async function save(t: Task, next: { startDate: string | null; dueDate: string | null }) {
		if (next.startDate === t.startDate && next.dueDate === t.dueDate) return;
		const before = finishOf(t);
		const { done, undo } = engine.track(() =>
			setTaskFields(ws, t.id, { start_date: next.startDate, due_date: next.dueDate })
		);
		await done;
		const after = finishOf(next);
		const later = before && after ? dayOf(after) - dayOf(before) : 0;
		const moved = { ...t, ...next };
		const hit = replica ? replica.waiting(t.id).filter((w) => conflicts(moved, w)) : [];
		const chain = later > 0 && hit.length && replica ? waitingChain(replica, t.id) : [];
		if (chain.length) {
			const n = chain.length;
			toasts.show(
				`${t.title} now ends after ${hit.map((w) => w.title).join(' and ')} ${hit.length === 1 ? 'starts' : 'start'}`,
				{
					onundo: () => void undo(),
					action: {
						label: `Move the ${n} ${n === 1 ? 'task' : 'tasks'} waiting on it too`,
						run: () => void moveChain(chain, later)
					}
				}
			);
		} else toasts.show(`${t.title}: ${datesSaid(next)}`, { onundo: () => void undo() });
	}

	async function moveChain(chain: Task[], days: number) {
		const { done, undo } = engine.track(() =>
			shiftDates(
				ws,
				chain.map((t) => t.id),
				days
			)
		);
		await done;
		const n = chain.length;
		toasts.show(
			`Moved ${n} ${n === 1 ? 'task' : 'tasks'} ${days} ${days === 1 ? 'day' : 'days'} later`,
			{
				onundo: () => void undo()
			}
		);
	}

	// Dragging a bar, one of its ends, or a dot to make a dependency.
	type Drag =
		| { kind: 'move' | 'start' | 'due'; task: Task; x0: number; moved: boolean }
		| {
				kind: 'link';
				task: Task;
				after: boolean;
				x0: number;
				y0: number;
				x: number;
				y: number;
				over: string | null;
		  };
	let drag = $state<Drag | null>(null);
	let tip = $state<{ text: string; x: number; y: number } | null>(null);

	function down(e: PointerEvent, t: Task) {
		if (e.button !== 0) return;
		const part = (e.target as Element).closest<HTMLElement>('[data-part]')?.dataset.part;
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		if (part === 'dot-before' || part === 'dot-after') {
			e.preventDefault();
			const body = scroller?.querySelector('.body')?.getBoundingClientRect();
			const dot = (e.target as Element).getBoundingClientRect();
			const x0 = dot.left + dot.width / 2 - (body?.left ?? 0) - lead;
			const y0 = dot.top + dot.height / 2 - (body?.top ?? 0);
			drag = {
				kind: 'link',
				task: t,
				after: part === 'dot-after',
				x0,
				y0,
				x: x0,
				y: y0,
				over: null
			};
			return;
		}
		drag = {
			kind: part === 'start' ? 'start' : part === 'due' ? 'due' : 'move',
			task: t,
			x0: e.clientX,
			moved: false
		};
	}

	function move(e: PointerEvent) {
		if (!drag) return;
		if (drag.kind === 'link') {
			const body = scroller?.querySelector('.body')?.getBoundingClientRect();
			drag.x = e.clientX - (body?.left ?? 0) - lead;
			drag.y = e.clientY - (body?.top ?? 0);
			const hit = document
				.elementsFromPoint(e.clientX, e.clientY)
				.map((el) => el.closest<HTMLElement>('[data-bar]'))
				.find((el) => el && el.dataset.bar !== drag!.task.id);
			drag.over = hit?.dataset.bar ?? null;
			return;
		}
		const dx = e.clientX - drag.x0;
		if (!drag.moved && Math.abs(dx) < 4) return;
		drag.moved = true;
		const days = Math.round(dx / px);
		const t = drag.task;
		const next = drag.kind === 'move' ? shifted(t, days) : stretched(t, drag.kind, days);
		draft = { id: t.id, ...next };
		tip = { text: datesSaid(next), x: e.clientX, y: e.clientY };
	}

	async function up() {
		const d = drag;
		drag = null;
		tip = null;
		if (!d) return;
		if (d.kind === 'link') {
			if (d.over) await link(d.task.id, d.over, d.after);
			return;
		}
		const next = draft;
		draft = null;
		if (!d.moved) {
			openTask(d.task.id);
			return;
		}
		if (next) await save(d.task, next);
	}

	/** The dot after a bar makes the other task wait for this one; the dot before, this one wait for it. */
	async function link(from: string, to: string, after: boolean) {
		if (!replica) return;
		const [waiter, blocker] = after ? [to, from] : [from, to];
		const problem = dependencyProblem(replica, waiter, blocker);
		if (problem) {
			toasts.error(problem);
			return;
		}
		await setDependency(ws, waiter, blocker, true);
		const w = replica.task(waiter)?.title ?? '';
		const b = replica.task(blocker)?.title ?? '';
		toasts.show(`${w} now waits for ${b}`, {
			onundo: () => void setDependency(ws, waiter, blocker, false)
		});
	}

	function keys(e: KeyboardEvent, row: Extract<Row, { kind: 'task' }>) {
		const t = row.task;
		const cmd = keymap.match('timeline', e);
		const i = taskRows.findIndex((r) => r.task.id === t.id);
		const go = (j: number) => {
			const r = taskRows[Math.max(0, Math.min(taskRows.length - 1, j))];
			if (r) focusRow(r.task.id);
		};
		const moveBy = (k: number, which: 'both' | 'due') => {
			const next = stepped(t, shownZoom, k, which);
			if (!next) {
				announce(
					which === 'due'
						? 'The due date cannot come before the start.'
						: `${t.title} has no dates.`
				);
				return;
			}
			void save(t, next).then(() => announce(`${t.title}, ${datesSaid(next)}`));
		};
		switch (cmd) {
			case 'timeline.earlier':
				return (e.preventDefault(), moveBy(-1, 'both'));
			case 'timeline.later':
				return (e.preventDefault(), moveBy(1, 'both'));
			case 'timeline.due-earlier':
				return (e.preventDefault(), moveBy(-1, 'due'));
			case 'timeline.due-later':
				return (e.preventDefault(), moveBy(1, 'due'));
		}
		if (e.altKey || e.ctrlKey || e.metaKey) return;
		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				return go(i + 1);
			case 'ArrowUp':
				e.preventDefault();
				return go(i - 1);
			case 'Home':
				e.preventDefault();
				return go(0);
			case 'End':
				e.preventDefault();
				return go(taskRows.length - 1);
			case 'ArrowLeft':
				e.preventDefault();
				if (row.kids && !row.folded) {
					fold(t.id, true);
					announce(`${t.title}: subtasks folded away`);
				} else if (t.parentId && indexOf.has(t.parentId)) focusRow(t.parentId);
				return;
			case 'ArrowRight':
				e.preventDefault();
				if (row.folded) {
					fold(t.id, false);
					announce(`${t.title}: subtasks shown`);
				}
				return;
			case 'Enter':
				e.preventDefault();
				return openTask(t.id);
		}
	}

	// Dragging a task from the tray onto the chart gives it a due date.
	let carrying = $state<{
		task: Task;
		x: number;
		y: number;
		x0: number;
		y0: number;
		day: number | null;
	} | null>(null);
	let dragged = false;
	function trayDown(e: PointerEvent, t: Task) {
		if (e.button !== 0) return;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		dragged = false;
		carrying = { task: t, x: e.clientX, y: e.clientY, x0: e.clientX, y0: e.clientY, day: null };
	}
	function trayMove(e: PointerEvent) {
		if (!carrying || !scroller) return;
		const r = scroller.getBoundingClientRect();
		const inside =
			e.clientX > r.left + lead && e.clientX < r.right && e.clientY > r.top && e.clientY < r.bottom;
		const day = inside
			? range.from + Math.floor((e.clientX - r.left - lead + scroller.scrollLeft) / px)
			: null;
		if (Math.hypot(e.clientX - carrying.x0, e.clientY - carrying.y0) > 4) dragged = true;
		carrying = { ...carrying, x: e.clientX, y: e.clientY, day };
	}
	async function trayUp() {
		const c = carrying;
		carrying = null;
		if (c && dragged && c.day != null) await schedule(c.task, isoOf(c.day));
	}
	async function schedule(t: Task, day: string) {
		const { done, undo } = engine.track(() => setTaskFields(ws, t.id, { due_date: day }));
		await done;
		toasts.show(`${t.title} is due ${formatDate(day)}`, { onundo: () => void undo() });
		focusRow(t.id);
	}

	const people = (t: Task) => (replica?.assignees(t.id) ?? []).slice(0, 2);
	const monthName = (iso: string) => formatMonth(iso.slice(0, 7));
	function bottomLabel(iso: string): string {
		if (shownZoom === 'day' || shownZoom === 'week') return String(Number(iso.slice(8)));
		if (shownZoom === 'month') return formatMonthShort(iso.slice(0, 7));
		return `Q${Math.floor((Number(iso.slice(5, 7)) - 1) / 3) + 1}`;
	}
	const topLabel = (iso: string) =>
		shownZoom === 'day' || shownZoom === 'week' ? monthName(iso) : iso.slice(0, 4);
</script>

<svelte:window onpointermove={trayMove} onpointerup={trayUp} />

<div class="timeline" class:phone data-timeline>
	<div class="tools">
		<div class="zoom" role="group" aria-label="Zoom">
			{#each zooms as z (z.id)}
				<button
					title={`Show the timeline by ${z.label.toLowerCase()}`}
					type="button"
					aria-pressed={shownZoom === z.id}
					onclick={() => setZoom(z.id)}>{z.label}</button
				>
			{/each}
		</div>
		<button class="tool" type="button" onclick={() => scrollToDay(dayOf(today), 0.25, true)}
			>Today</button
		>
		{#if !phone}
			<span class="hint"
				>Drag a bar to move it, an end to change one date, or a dot onto another task to make one
				wait.</span
			>
		{/if}
	</div>

	{#if !laid.rows.length && !laid.unscheduled.length}
		<EmptyState title={emptyTitle} hint={emptyHint} />
	{:else}
		<div class="scroller" bind:this={scroller}>
			<div class="inner" style:width={`${lead + width}px`}>
				<div class="head">
					{#if !phone}<div class="corner" style:width={`${lead}px`}>
							Task
							<span class="edge">
								<Splitter
									label="Resize the task names"
									value={layout.timelineNamesWidth}
									min={TIMELINE_NAMES.min}
									max={TIMELINE_NAMES.max}
									grows="right"
									reset={TIMELINE_NAMES.initial}
									onresize={(px) => layout.setTimelineNamesWidth(px)}
								/>
							</span>
						</div>{/if}
					<div class="scale" style:width={`${width}px`} aria-hidden="true">
						{#each scale.top as tk (tk.day)}
							<span class="top" style:left={`${x(tk.day)}px`}>{topLabel(tk.iso)}</span>
						{/each}
						{#each scale.bottom as tk (tk.day)}
							<span
								class="bot"
								class:weekend={tk.weekend}
								class:today={shownZoom === 'day' && tk.iso === today}
								style:left={`${x(tk.day)}px`}
								style:width={`${tk.days * px}px`}>{bottomLabel(tk.iso)}</span
							>
						{/each}
						<span class="todaymark" style:left={`${x(dayOf(today)) + px / 2}px`}></span>
					</div>
				</div>
				<div class="body" style:height={`${rows.length * rowH}px`}>
					<div class="grid" style:left={`${lead}px`} style:width={`${width}px`} aria-hidden="true">
						{#each scale.weekends as d (d)}
							<span class="weekend" style:left={`${x(d)}px`} style:width={`${px}px`}></span>
						{/each}
						{#each scale.lines as l (l.day)}
							<span class="line" class:strong={l.strong} style:left={`${x(l.day)}px`}></span>
						{/each}
						<span class="todayline" style:left={`${x(dayOf(today)) + px / 2}px`}></span>
					</div>
					<svg
						class="arrows"
						style:left={`${lead}px`}
						{width}
						height={rows.length * rowH}
						aria-hidden="true"
					>
						{#each arrows as a (a.key)}
							<path class="link" class:bad={a.bad} d={a.d} />
							<path class="head" class:bad={a.bad} d={a.head} />
						{/each}
						{#if drag?.kind === 'link'}
							<path class="draft" d={`M${drag.x0} ${drag.y0} L${drag.x} ${drag.y}`} />
						{/if}
					</svg>
					<ul class="rows" aria-label={label}>
						{#each rows as row (row.key)}
							{#if row.kind === 'group'}
								<li class="group" style:height={`${rowH}px`}>
									<span class="grouptitle" style:width={phone ? undefined : `${lead}px`}
										>{row.title} <span class="count tabular">{row.count}</span></span
									>
								</li>
							{:else}
								{@const t = row.task}
								{@const shape = shapeFor(t)}
								{@const blocked = !t.completedAt && (replica?.isBlocked(t.id) ?? false)}
								{@const repeats = repeatLabel(t.recurrence, t.recurrenceMode, t.dueDate)}
								<li
									class="row"
									class:done={!!t.completedAt}
									style:height={`${rowH}px`}
									use:menu={taskActions(ws, t.id)}
								>
									{#if !phone}
										<div
											class="lead"
											style:width={`${lead}px`}
											style:padding-left={`${12 + row.depth * 20}px`}
										>
											{#if row.kids}
												<button
													type="button"
													class="twisty"
													tabindex="-1"
													aria-expanded={!row.folded}
													aria-label={`${row.folded ? 'Show' : 'Hide'} the subtasks of ${t.title}`}
													onclick={() => fold(t.id, !row.folded)}><ChevronDown size={14} /></button
												>
											{:else}<span class="twisty-gap"></span>{/if}
											<button
												type="button"
												class="title"
												data-row={t.id}
												tabindex={current === t.id ? 0 : -1}
												aria-label={rowLabel(t)}
												onfocus={() => (active = t.id)}
												onkeydown={(e) => keys(e, row)}
												onclick={() => openTask(t.id)}>{t.title}</button
											>
											{#if row.folded}<span class="subcount tabular"
													>{row.kids}<span class="visually-hidden">
														subtasks folded away</span
													></span
												>{/if}
											{#if repeats}<RepeatIcon label={repeats} />{/if}
											{#if blocked}<BlockedMark names={replica?.openBlockers(t.id) ?? []} />{/if}
										</div>
									{/if}
									<div class="track" style:width={`${width}px`}>
										{#each ghosts(t) as g (g)}
											<span
												class="ghost"
												style:left={`${x(dayOf(g)) + px / 2}px`}
												title={`${t.title} repeats on ${formatDate(g)}`}
											></span>
										{/each}
										{#if row.kids}
											{@const span = spanUnder(t, kids)}
											{#if span && (row.folded || !shape)}
												<span
													class="bracket"
													style:left={`${x(dayOf(span.from))}px`}
													style:width={`${(dayOf(span.to) - dayOf(span.from) + 1) * px}px`}
												></span>
												{#if phone && !shape}
													<button
														type="button"
														class="parentlabel"
														data-row={t.id}
														tabindex={current === t.id ? 0 : -1}
														aria-label={rowLabel(t)}
														style:left={`${x(dayOf(span.from))}px`}
														onfocus={() => (active = t.id)}
														onkeydown={(e) => keys(e, row)}
														onclick={() => openTask(t.id)}>{t.title}</button
													>
												{/if}
											{/if}
										{/if}
										{#if shape}
											{@const fits =
												shape.kind === 'bar' &&
												shape.right - shape.left > t.title.length * 7.5 + 28}
											<button
												type="button"
												class={shape.kind === 'bar' ? `bar ${tone(t)}` : `mark ${tone(t)}`}
												class:start-only={shape.kind === 'mark' && shape.startOnly}
												class:lifted={draft?.id === t.id}
												class:target={drag?.kind === 'link' && drag.over === t.id}
												data-bar={t.id}
												data-row={phone ? t.id : undefined}
												tabindex={phone && current === t.id ? 0 : -1}
												aria-hidden={phone ? undefined : 'true'}
												aria-label={phone ? rowLabel(t) : undefined}
												style:left={`${shape.kind === 'bar' ? shape.left : shape.center}px`}
												style:width={shape.kind === 'bar'
													? `${Math.max(4, shape.right - shape.left)}px`
													: undefined}
												onfocus={() => (active = t.id)}
												onkeydown={(e) => keys(e, row)}
												onpointerdown={(e) => down(e, t)}
												onpointermove={move}
												onpointerup={up}
												onpointercancel={() => {
													drag = null;
													draft = null;
													tip = null;
												}}
											>
												{#if shape.kind === 'bar' && t.estimateMinutes}
													{@const spent = replica?.logged(t.id) ?? 0}
													<!-- How much of the estimate is logged; red past it. -->
													<span
														class="fill"
														class:over={spent > t.estimateMinutes}
														style:width={`${Math.min(100, (spent / t.estimateMinutes) * 100)}%`}
													></span>
												{/if}
												{#if shape.kind === 'bar'}
													<span class="end start" data-part="start"></span>
													<span class="end due" data-part="due"></span>
												{/if}
												{#if !phone}
													<span class="dot before" data-part="dot-before"></span>
													<span class="dot after" data-part="dot-after"></span>
												{/if}
												{#if phone}
													<span class="label" class:inside={fits && tone(t) === 'todo'}
														>{t.title}</span
													>
												{:else}
													<span class="people">
														{#each people(t) as id (id)}
															{@const p = session.personOf(ws, id)}
															<Avatar name={p.name} src={p.avatar} size={18} />
														{/each}
													</span>
												{/if}
											</button>
										{/if}
									</div>
								</li>
							{/if}
						{/each}
					</ul>
				</div>
				<div use:growing.more></div>
			</div>
		</div>

		<div class="tray" role="region" aria-label="Unscheduled">
			<h2>Unscheduled <span class="count tabular">{laid.unscheduled.length}</span></h2>
			{#each laid.unscheduled as t (t.id)}
				<button
					type="button"
					class="chip"
					aria-label={`${t.title}, no dates. Choose it to make it due today, or drag it onto the timeline.`}
					onpointerdown={(e) => trayDown(e, t)}
					onclick={() => {
						// The click that ends a drag onto the timeline is not a choice.
						if (!dragged) void schedule(t, today);
						dragged = false;
					}}><GripVertical size={12} />{t.title}</button
				>
			{:else}
				<span class="none">Every task here has a date.</span>
			{/each}
		</div>
	{/if}
</div>

{#if tip}<span class="tip tabular" style:left={`${tip.x}px`} style:top={`${tip.y}px`}
		>{tip.text}</span
	>{/if}
{#if carrying}
	<span class="carried" style:left={`${carrying.x + 12}px`} style:top={`${carrying.y + 12}px`}
		>{carrying.task.title}{#if carrying.day !== null}<span class="when">
				due {formatDate(isoOf(carrying.day))}</span
			>{/if}</span
	>
{/if}

<style>
	.timeline {
		display: flex;
		flex-direction: column;
		min-height: 0;
		height: 100%;
	}
	.tools {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-4);
		border-bottom: var(--border-width) solid var(--border);
	}
	.zoom {
		display: inline-flex;
		gap: 2px;
		padding: 2px;
		border-radius: var(--radius-md);
		background: var(--surface-2);
	}
	.zoom button {
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-3);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.zoom button[aria-pressed='true'] {
		background: var(--surface-0);
		color: var(--text-primary);
		font-weight: 600;
		box-shadow: var(--shadow-sm);
	}
	.tool {
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-3);
		border: var(--border-width) solid var(--border-strong);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
		cursor: pointer;
	}
	.hint {
		margin-left: auto;
		color: var(--text-muted);
		font-size: var(--text-xs);
	}
	.scroller {
		position: relative;
		flex: 1;
		min-height: 0;
		overflow: auto;
		overscroll-behavior: contain;
	}
	.inner {
		position: relative;
	}
	.head {
		position: sticky;
		top: 0;
		z-index: 6;
		display: flex;
		border-bottom: var(--border-width) solid var(--border-strong);
		background: var(--surface-1);
	}
	.corner {
		position: sticky;
		left: 0;
		z-index: 7;
		flex: none;
		display: flex;
		align-items: flex-end;
		padding: 0 var(--space-3) var(--space-2);
		border-right: var(--border-width) solid var(--border);
		background: var(--surface-1);
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 600;
	}
	/* The corner's right edge, which sets how wide the names are. */
	.edge {
		position: absolute;
		top: 0;
		right: 0;
		bottom: 0;
	}
	.scale {
		position: relative;
		flex: none;
		height: 50px;
	}
	.scale span {
		position: absolute;
		white-space: nowrap;
		font-size: var(--text-xs);
	}
	.top {
		top: 6px;
		height: 18px;
		padding-left: var(--space-2);
		border-left: var(--border-width) solid var(--border-strong);
		color: var(--text-primary);
		font-weight: 600;
		line-height: 18px;
	}
	.bot {
		top: 28px;
		height: 22px;
		border-left: var(--border-width) solid var(--border);
		color: var(--text-secondary);
		line-height: 20px;
		text-align: center;
		overflow: hidden;
	}
	.bot.weekend {
		color: var(--text-muted);
	}
	.bot.today {
		color: var(--accent-text);
		font-weight: 700;
	}
	.todaymark {
		bottom: 0;
		width: 0;
		height: 0;
		transform: translateX(-50%);
		border: 6px solid transparent;
		border-top: 0;
		border-bottom: 7px solid var(--accent);
	}
	.body {
		position: relative;
	}
	.grid,
	.arrows {
		position: absolute;
		top: 0;
		bottom: 0;
		pointer-events: none;
	}
	.grid span {
		position: absolute;
		top: 0;
		bottom: 0;
	}
	.weekend {
		background: var(--surface-1);
	}
	.line {
		width: 1px;
		background: var(--border);
	}
	.line.strong {
		background: var(--border-strong);
	}
	.todayline {
		width: 2px;
		margin-left: -1px;
		background: var(--accent);
	}
	.arrows {
		z-index: 1;
		overflow: visible;
	}
	.link {
		fill: none;
		stroke: var(--text-muted);
		stroke-width: 1.5;
	}
	.link.bad {
		stroke: var(--danger);
		stroke-width: 2;
	}
	.arrows .head {
		fill: var(--text-muted);
	}
	.arrows .head.bad {
		fill: var(--danger);
	}
	.draft {
		fill: none;
		stroke: var(--accent);
		stroke-width: 2;
		stroke-dasharray: 4 3;
	}
	.rows {
		position: relative;
		z-index: 2;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.row,
	.group {
		display: flex;
	}
	.lead {
		position: sticky;
		left: 0;
		z-index: 4;
		flex: none;
		display: flex;
		align-items: center;
		gap: var(--space-1);
		padding-right: var(--space-2);
		overflow: hidden;
		border-right: var(--border-width) solid var(--border);
		border-bottom: var(--border-width) solid var(--border);
		background: var(--surface-0);
		white-space: nowrap;
	}
	.row:hover .lead,
	.row:focus-within .lead {
		background: var(--surface-1);
	}
	.grouptitle {
		position: sticky;
		left: 0;
		z-index: 4;
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: 0 var(--space-3);
		background: var(--surface-0);
		font-weight: 600;
		white-space: nowrap;
	}
	.count {
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-weight: 500;
	}
	.twisty {
		display: grid;
		flex: none;
		place-items: center;
		width: 20px;
		height: 20px;
		padding: 0;
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text-secondary);
		cursor: pointer;
	}
	.twisty[aria-expanded='false'] :global(svg) {
		transform: rotate(-90deg);
	}
	.twisty-gap {
		flex: none;
		width: 20px;
	}
	.title {
		min-width: 0;
		overflow: hidden;
		padding: var(--space-1);
		border: none;
		border-radius: var(--radius-sm);
		background: none;
		text-align: left;
		text-overflow: ellipsis;
		cursor: pointer;
	}
	.done .title {
		color: var(--text-muted);
		text-decoration: line-through;
	}
	.subcount {
		flex: none;
		padding: 0 5px;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}
	.track {
		position: relative;
		flex: none;
		border-bottom: var(--border-width) solid var(--border);
	}
	.bar,
	.mark {
		position: absolute;
		top: 50%;
		padding: 0;
		border: none;
		touch-action: none;
		cursor: grab;
	}
	.bar {
		height: 20px;
		margin-top: -10px;
		border-radius: var(--radius-sm);
		background: var(--accent);
	}
	.bar.progress {
		background: var(--status-in-progress);
	}
	.fill {
		position: absolute;
		left: 0;
		bottom: 0;
		height: 4px;
		border-radius: 0 0 0 var(--radius-sm);
		background: var(--surface-0);
		opacity: 0.8;
		pointer-events: none;
	}
	.fill.over {
		background: var(--danger);
		opacity: 1;
	}
	.bar.done {
		background: var(--status-done);
		opacity: 0.55;
	}
	.mark {
		width: 14px;
		height: 14px;
		margin: -7px 0 0 -7px;
		border-radius: 3px;
		transform: rotate(45deg);
		background: var(--accent);
	}
	.mark.progress {
		background: var(--status-in-progress);
	}
	.mark.done {
		background: var(--status-done);
		opacity: 0.55;
	}
	.mark.start-only {
		border: 2px solid var(--accent);
		background: var(--surface-0);
	}
	.lifted {
		z-index: 5;
		box-shadow: var(--shadow-md);
		cursor: grabbing;
	}
	.target {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.end {
		position: absolute;
		top: 0;
		bottom: 0;
		width: 8px;
		cursor: ew-resize;
	}
	.end.start {
		left: 0;
	}
	.end.due {
		right: 0;
	}
	.dot {
		position: absolute;
		top: 50%;
		width: 12px;
		height: 12px;
		margin-top: -6px;
		border: 2px solid var(--accent);
		border-radius: var(--radius-full);
		background: var(--surface-0);
		opacity: 0;
		cursor: crosshair;
	}
	.bar .dot.before {
		left: -16px;
	}
	.bar .dot.after {
		right: -16px;
	}
	.mark .dot {
		transform: rotate(-45deg);
	}
	.mark .dot.before {
		top: 12px;
		left: -12px;
	}
	.mark .dot.after {
		top: -2px;
		right: -12px;
	}
	.row:hover .dot,
	.timeline:has(.draft) .dot {
		opacity: 1;
	}
	.people {
		position: absolute;
		top: 50%;
		left: calc(100% + 6px);
		display: flex;
		gap: 2px;
		transform: translateY(-50%);
		pointer-events: none;
	}
	.mark .people {
		left: 14px;
		top: -4px;
		transform: rotate(-45deg);
	}
	.label {
		position: absolute;
		top: 50%;
		left: calc(100% + 6px);
		transform: translateY(-50%);
		color: var(--text-primary);
		font-size: var(--text-xs);
		font-weight: 600;
		white-space: nowrap;
		pointer-events: none;
	}
	.label.inside {
		left: var(--space-2);
		right: var(--space-2);
		overflow: hidden;
		color: var(--accent-contrast);
		text-overflow: ellipsis;
	}
	.mark .label {
		left: 16px;
		top: -2px;
		transform: rotate(-45deg);
		transform-origin: left top;
	}
	.ghost {
		position: absolute;
		top: 50%;
		width: 12px;
		height: 12px;
		margin: -6px 0 0 -6px;
		border: 1.5px dashed var(--border-control);
		border-radius: 3px;
		transform: rotate(45deg);
		pointer-events: none;
	}
	.bracket {
		position: absolute;
		top: 50%;
		height: 10px;
		margin-top: -5px;
		border: 2px solid var(--text-secondary);
		border-top-width: 3px;
		border-bottom: none;
		border-radius: 3px 3px 0 0;
		pointer-events: none;
	}
	.parentlabel {
		position: absolute;
		top: 2px;
		padding: 0 var(--space-1);
		border: none;
		background: none;
		color: var(--text-primary);
		font-size: var(--text-xs);
		font-weight: 600;
		white-space: nowrap;
	}
	.tray {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-4) var(--space-3);
		border-top: var(--border-width) solid var(--border-strong);
		background: var(--surface-1);
	}
	.tray h2 {
		margin: 0 var(--space-2) 0 0;
		font-size: var(--text-sm);
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: max(var(--tap-min), 28px);
		padding: 0 var(--space-3);
		border: var(--border-width) dashed var(--border-control);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
		touch-action: none;
		cursor: grab;
	}
	.none {
		color: var(--text-muted);
		font-size: var(--text-sm);
	}
	.tip,
	.carried {
		position: fixed;
		z-index: 50;
		pointer-events: none;
		white-space: nowrap;
	}
	.tip {
		padding: 2px var(--space-2);
		border-radius: var(--radius-sm);
		background: var(--text-primary);
		color: var(--surface-0);
		font-size: var(--text-xs);
		font-weight: 600;
		transform: translate(-50%, -150%);
	}
	.carried {
		padding: var(--space-1) var(--space-3);
		border: var(--border-width) solid var(--accent);
		border-radius: var(--radius-sm);
		background: var(--surface-0);
		box-shadow: var(--shadow-lg);
	}
	.when {
		color: var(--text-secondary);
	}
	.phone .row .track {
		border-bottom-color: var(--border);
	}
	@media (prefers-reduced-motion: reduce) {
		.twisty :global(svg) {
			transition: none;
		}
	}
</style>
