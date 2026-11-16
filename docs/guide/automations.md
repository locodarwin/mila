# Automations

An automation is a rule that changes tasks when something happens to them. When a task moves to Done, it can unassign everyone. When a task arrives by email, it can add a label and set a due date. Mila makes the changes for you, on the server, whichever device made the first change.

Automations are in Settings, Automations, for the workspace picked under Workspace in Settings.

## Making a rule

1. Choose New automation.
2. Give it a name.
3. Under In, choose one project or The whole workspace.
4. Under When, choose what starts it.
5. Under Only if, choose which tasks it works on, or leave it at Any task.
6. Under Then, choose what it does. Add a change adds another.
7. Choose Save.

The sentence above the builder says how the rule reads. It is how the rule shows in the list.

Only if uses the same conditions as a saved view's [filter](filters.md). With none, the rule works on any task where it applies.

Where a rule works cannot change once it is saved. To move a rule to another project, make a new one there.

### Testing a rule

Under the builder, Test it on your tasks shows what the rule would do to a few open tasks if its When happened to them now. Each says what it would do, or "Doesn't fit, so nothing." The test changes nothing.

### Turning a rule off

The switch beside a rule in the list turns it off without losing it. Turn it on again when you want it back. Edit and Delete are beside it too.

## What can start a rule

- A task is created, completed or reopened.
- A task moves to a status.
- Its priority changes to one you choose.
- Someone is assigned, a person you choose or anyone.
- A label you choose is added.
- A custom field you choose changes.
- A comment is added.
- Its last blocker finishes. See [Dependencies](dependencies.md).
- It arrives by email. See [Email to task](email-to-task.md).
- Its due date is near: it is due today, in 1 day, in 3 days or in a week, or it is overdue. Mila checks every few minutes, and a rule runs once for each due date.

Each project has its own statuses, so a rule for the whole workspace chooses a kind of status instead, such as a done status.

## What a rule can do

- Set the status, the priority or the due date. The due date can be today, tomorrow, in 3 days, in a week or next Monday.
- Complete the task.
- Assign someone, or unassign everyone. Besides a person, you can choose whoever made the task, or whoever did this.
- Add or remove a label.
- Move the task to another project, with its subtasks.
- Add a comment or a subtask.
- Notify someone.

A rule makes up to 10 changes, in order. If one cannot be done, such as assigning someone who cannot reach the task, the others still are, and Recent runs says what happened.

A rule never deletes anything.

A comment a rule adds is plain text, so it mentions nobody. Use Notify to tell someone.

## Whose rule it is

Members make rules for a project. Owners and admins also make rules for the whole workspace. A workspace holds up to 100.

Guests do not see rules, though what they do can start one. A guest is told what a rule did, in a task's history and in a notification, but not its name. See [If you are a guest](guests.md).

A rule runs as whoever last saved it or turned it on, and can only do what that person can. The list says who each rule runs as. Turning on someone else's rule makes it yours.

If that person can no longer run it, because they left the workspace or became a guest, the rule stops and the list says why. Saving it makes it yours and starts it again.

## What to expect

A change made offline starts its rules when it reaches the server.

What a rule does shows in the task's history under the rule's name. Undo takes back your own edit. What the rule did then stays until you change it.

Rules act from now on. Turning one on changes nothing already there.

A rule's own changes start no other rule, so put changes that belong together in one rule. Imports, making things from a template and a repeating task coming back start no rules either.

A rule that runs 200 times in an hour stops, and whoever saved it is told. Changing many [selected tasks](selecting-tasks.md) at once counts as one run toward that.

## Recent runs

Recent runs, at the end of the page, lists what each rule did in the last 30 days: when, which rule, which task, and what happened. A run that could not finish also tells whoever saved the rule. The list needs a connection.

Notifications from automations, a rule's Notify and a rule of yours that stops, come under Automations in Settings, Notifications. See [Notifications](notifications.md).

## Templates and imports

A project template keeps its project's rules. In each project made from it, they start switched off.

A workspace brought in from a JSON [export](exports-and-imports.md) keeps its rules, switched off.

## Next

- [Email to task](email-to-task.md)
- [Templates](templates.md)
