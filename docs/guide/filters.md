# Filters

Filter, at the top of a [saved view](saved-views.md), narrows what the view shows. The filter saves with the view as you change it, and a number on the button says how many parts are set. What the view shows changes as you set each part, in every layout.

Inbox, My Day and Upcoming have the same Filter, kept on this device. There it narrows what the list holds and never adds to it. See [Other ways to see them](daily-lists.md#other-ways-to-see-them).

A project's own page always shows all its tasks. To filter a project, save it as a view with the bookmark button.

## What you can filter by

| Part | Finds |
|---|---|
| Words | Words in the title or notes, matched as one phrase |
| Status | The kind of status (Backlog, To do, In progress, Done or Canceled), each covering every project's statuses of that kind, then the projects' own statuses by name, such as Waiting on review. A status several projects have is one choice. A task matches a kind or a status picked |
| Priority | The priorities you pick |
| Label | Tasks with the labels you pick |
| Assignee | Tasks assigned to the people you pick |
| Due | Any time, Overdue, Due today or before, Due within a week, Has a due date, or No due date |
| Dependencies | Is blocked: tasks waiting for one not done. Is blocking: unfinished tasks that hold others up |
| Time | Tasks with an estimate or none, over their estimate, with time logged lately, or with time logged by you |
| Project | In a view across every project, the projects you pick. Inbox stands for the tasks with no project |
| In my My Day | The tasks you picked for [My Day](daily-lists.md#my-day), today or on an earlier day. Each person sees their own picks |

## How the parts combine

Where you pick from chips, a task matches if it has any of the chips you picked. Time is the exception: a task must have every one you picked.

A task must match every part you set. So "Priority: High or Urgent" and "Label: Client" finds high or urgent tasks that also carry that label.

## Finished tasks

Finished tasks stay out unless you turn on Show finished tasks, or pick the Done or Canceled status.

## Custom fields

Add a field condition to filter on a custom field. What you can ask depends on its type:

| Type | Conditions |
|---|---|
| Text, Link | Contains words, or does not |
| Number | Equal to, less than or more than a number |
| Date | On, before, after or between days, given as a date or as today, tomorrow or a week away |
| Select, Person | Any or none of those you pick |
| Multi-select | Any, none, or all of those you pick |
| Checkbox | Checked or not |

Any field except a checkbox can also be empty or not empty.

A condition on a field that was deleted, or that you cannot see, treats every task as having no value there. It shows as A deleted field, with a Remove button.

A choice that was deleted, or a person who left, stays in the condition as its own chip until you take it out.

## Next

- [Search and the command palette](search-and-palette.md)
- [Calendar feeds](calendar-feeds.md)
