<script lang="ts">
	import Tip from '../../components/help/Tip.svelte';
</script>

<h2 id="saving" tabindex="-1">Saving a template</h2>
<p>
	A template is a copy of a project or a task to start new ones from. Open a project, or a task's
	panel, and choose Save as a template. Name it, and choose whether it is a new template or replaces
	one you saved before.
</p>
<p>
	A project template keeps the project's statuses, fields, saved views, wiki pages, description,
	icon and color, and its open tasks with their subtasks, checklists, notes, priorities, labels,
	field values, repeats and dependencies. A task template keeps the task and its open subtasks the
	same way. Comments, files, history and finished tasks stay behind.
</p>
<p>
	Assignees stay behind too, unless you choose Keep assignees. Someone who has left the workspace by
	the time the template is used is left off, and Mila says so.
</p>
<Tip
	>A template is a copy taken when you save it: changing the project or task afterwards does not
	change the template.</Tip
>
<p>Templates belong to the workspace, and every member can use them; guests do not see them.</p>

<h2 id="projects" tabindex="-1">A project from a template</h2>
<p>
	On the Projects page, choose From a template beside the new project's name, or search the command
	palette for "from a template". Pick the template, name the project and choose its start date. The
	task with the earliest date lands on the start date, and every other date moves with it.
</p>
<p>
	The new project's saved views are yours alone at first; share them as you would any view. Labels
	the template uses that the workspace no longer has are made again.
</p>

<h2 id="tasks" tabindex="-1">A task from a template</h2>
<p>
	The button beside New task in the sidebar lists the task templates, and the command palette finds
	them too. The task is made in the project you are looking at, or in the Inbox, due as many days
	after today as the template's task was due after it was saved. The message that follows opens the
	task, or takes it back.
</p>

<h2 id="pages" tabindex="-1">A wiki page from a template</h2>
<p>
	To save a page, choose Save as a template in its menu. A template holds the page's title and text
	and, if you keep them, its subpages. Comments, history and files stay behind, and images with
	them.
</p>
<p>
	A new, empty page lists the workspace's page templates under its text until you start typing. You
	can also use the button beside New page in the list of pages, or the command palette. A template
	only ever fills an empty page. It works in any wiki in the workspace, and links between its pages
	lead to the new copies.
</p>
<p>
	Write <code>{'{{today}}'}</code> in a page's title or text, and using the template puts in the date
	it is used on, the way you show dates.
</p>

<h2 id="managing" tabindex="-1">Managing templates</h2>
<p>
	Settings, Templates lists the workspace's templates and shows what each one holds, with its dates
	counted in days from its first. Rename a template there, start something from it, or delete it. A
	deleted template stays in the trash for 30 days.
</p>
<p>
	To change a template, start a project or a task from it, change that, and save it as a template
	again, replacing the old one.
</p>
<p>
	A workspace keeps up to 200 templates, each with up to 500 tasks. Templates go into the
	workspace's JSON export and come back with an import.
</p>
