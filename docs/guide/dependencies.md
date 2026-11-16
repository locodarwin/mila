# Dependencies

Some work cannot start until other work is done. Walls are painted after the drywall, and the tile goes on after the countertops. You mark that by making one task wait for another.

## Making a task wait

Open the task that waits. Under Dependencies, Blocked by, choose Add a task it waits for. Type part of a title and pick the task.

Blocking, just below, lists the tasks that wait for this one. Remove either kind with the button at the end of its line.

You can also draw a dependency on a [timeline](layouts.md#timeline).

A task can wait for tasks in other projects of the workspace. Mila refuses a dependency that would go in a circle, such as a task waiting for one that already waits for it, and says which tasks are involved.

Guests see a dependency only when they can see both of its tasks.

## Blocked tasks

A task waiting for one that is not done shows a Blocked mark in lists, on boards, in tables and on the timeline. Point at the mark to see what it waits for. Once every task it waits for is done, the mark goes.

To find these tasks, [filter](filters.md) a saved view, or Inbox, My Day or Upcoming, by Dependencies: Is blocked shows tasks that are waiting, and Is blocking shows unfinished tasks that hold others up.

## Dates that clash

A task should start after the tasks it waits for are due. When it starts on or before that day, its panel says so in red, and on a timeline the arrow between the two turns red. A task with only a due date counts that date as its start and its end.

Mila never moves dates by itself. On the timeline, when you move a task later than a task waiting for it starts, Mila offers to move every task down the chain by the same number of days. Undo puts everything back.

## When a task can start

When a task is done, the people assigned to each task that waited for it are told: that it can start, or what it still waits for. You are not told about a task you finished yourself.

Choose where these go under Ready to start in [Settings, Notifications](notifications.md).

## Next

- [Time tracking](time-tracking.md)
- [Layouts: Timeline](layouts.md#timeline)
