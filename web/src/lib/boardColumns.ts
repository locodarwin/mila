// A board's columns: a project's statuses, the
// priorities, due dates or a field's choices, each with what moving a card
// into it writes. Apart from viewDisplay.ts, so a page loads it only once a
// board shows.
import { addDays } from './filter';
import type { Columns, Task } from './sync/entities';
import {
	PRIORITY_ORDER,
	peopleOrder,
	type BoardColumn,
	type ColumnContext,
	type ColumnIcon
} from './viewDisplay';

/**
 * A board's columns. Each task belongs to exactly one: a task whose status
 * is gone is in the first status column, and one whose value names no
 * column is under None.
 */
export function boardColumns(columns: Columns, ctx: ColumnContext): BoardColumn[] {
	if (columns.by === 'due') return dueColumns(ctx);
	if (columns.by === 'priority')
		return PRIORITY_ORDER.map((p) => ({
			id: `prio:${p.value}`,
			title: p.label,
			icon: { kind: 'priority', priority: p.value },
			has: (t) => t.priority === p.value,
			write: { priority: p.value }
		}));
	if (columns.by === 'field') {
		const field = ctx.fields.find((f) => f.id === columns.field);
		if (field && (field.type === 'select' || field.type === 'user')) {
			const values = ctx.values.get(field.id);
			const people =
				field.type === 'user'
					? peopleOrder(
							ctx.members.map((m) => m.userId),
							ctx
						)
					: [];
			const choices =
				field.type === 'user'
					? people.map((p) => ({
							id: p.id,
							title: p.title,
							icon: { kind: 'person', name: ctx.nameOf(p.id) } as ColumnIcon
						}))
					: field.options.map((o) => ({
							id: o.id,
							title: o.name,
							icon: { kind: 'color', color: o.color } as ColumnIcon
						}));
			const ids = new Set(choices.map((c) => c.id));
			const valueOf = (t: Task) => {
				const v = values?.get(t.id);
				return typeof v === 'string' && ids.has(v) ? v : null;
			};
			return [
				...choices.map((c) => ({
					id: `f:${c.id}`,
					title: c.title,
					icon: c.icon,
					has: (t: Task) => valueOf(t) === c.id,
					write: { field: field.id, value: c.id }
				})),
				{
					id: 'f:none',
					title: 'None',
					has: (t: Task) => valueOf(t) === null,
					write: { field: field.id, value: null }
				}
			];
		}
	}
	const known = new Set(ctx.statuses.map((s) => s.id));
	return ctx.statuses.map((s, i) => ({
		id: `s:${s.id}`,
		title: s.name,
		icon: { kind: 'status', category: s.category },
		has: (t: Task) => t.statusId === s.id || (i === 0 && !known.has(t.statusId ?? '')),
		write: { status: s.id }
	}));
}

/**
 * A board by due date, as Upcoming lists it: what is past, today, tomorrow,
 * each of the five days after by name, then later, and (where tasks can have
 * none) no date. Nothing moves into the past; moving a card to Later gives it
 * the first day after the week.
 */
function dueColumns(ctx: ColumnContext): BoardColumn[] {
	const today = ctx.today ?? isoToday();
	const name = ctx.dayName ?? ((day: string) => day);
	const week = Array.from({ length: 7 }, (_, i) => addDays(today, i));
	const out: BoardColumn[] = [
		{
			id: 'due:past',
			title: ctx.dated ? 'Overdue' : 'Earlier',
			has: (t) => !!t.dueDate && t.dueDate < today,
			write: null
		},
		...week.map((day, i) => ({
			id: `due:${day}`,
			title: i === 0 ? 'Today' : i === 1 ? 'Tomorrow' : name(day),
			has: (t: Task) => t.dueDate === day,
			write: { due: day }
		})),
		{
			id: 'due:later',
			title: 'Later',
			has: (t) => !!t.dueDate && t.dueDate > week[6],
			write: { due: addDays(today, 7) }
		}
	];
	if (!ctx.dated)
		out.push({ id: 'due:none', title: 'No date', has: (t) => !t.dueDate, write: { due: null } });
	return out;
}

function isoToday(): string {
	const d = new Date();
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
