<script lang="ts">
	import Tip from '../../components/help/Tip.svelte';
</script>

<h2 id="dry-run" tabindex="-1">Checking first</h2>
<p>
	Settings, Import and export, has two ways to bring tasks in from elsewhere: a CSV file, and a file
	from another app. Both run in your browser and show you a dry run before anything changes: how
	many tasks would come in and where, what would be made for them, what is left out and why.
</p>
<p>
	An import takes up to 5,000 tasks at a time. Once you have chosen the file, it works offline, like
	any other edit. Names are cut to the lengths Mila takes, so nothing in the file is refused for
	being too long.
</p>
<p>
	Importing the same file twice makes two copies of each task, and the dry run can't tell you that.
	When an import is done, you can open what it made, or move every task it made to the trash at
	once.
</p>

<h2 id="csv" tabindex="-1">A CSV file</h2>
<p>
	Any spreadsheet or app that saves CSV works. After you choose the file, Mila guesses what each
	column means from its heading, and you can change any of them. One column must be the title. Dates
	are read as 2026-11-03.
</p>
<p>
	With a Project column, each row can go in the project it names, made if there is none by that
	name, and rows that name none go to a new project. Or put every row in one place: a new project,
	one you have, or the inbox. Statuses, labels and field choices are matched by name, and made where
	missing.
</p>
<p>
	A column can also hold estimates, checklist items, comments, or links to files, which are listed
	at the end of the notes. To bring in subtasks, choose the column of each row's ID, and the column
	that names its parent's ID or title; a subtask whose parent isn't in the file comes in on its own.
</p>
<p>
	Columns of the tasks each one is blocked by or blocking bring in dependencies, naming each task by
	its ID or title. A dependency on a task that isn't coming in, or one that would make a loop, is
	left out, and the dry run says so.
</p>

<h2 id="github" tabindex="-1">GitHub Issues</h2>
<p>
	GitHub's site has no export for issues, so the file comes from GitHub's gh tool. Install it and
	sign in, then run the command shown under Import from another app in the repository's folder. It
	writes issues.json, which you choose on the same page.
</p>
<p>
	Each issue becomes a task, in a new project named after the repository unless you choose another
	project or the inbox. Open issues come in to do, and closed ones done, as of the day they closed.
	The notes start with a link back to the issue, and its text follows.
</p>
<p>
	Labels come in as labels, matched by name where you have them already, and a milestone becomes a
	label too. Comments come in as comments of yours, each starting with who wrote it on GitHub and
	when.
</p>
<p>
	Assignees are matched to people in the workspace whose name or email address is their GitHub
	username. Anyone else is left off, and the dry run lists them, with the issues that come in
	unassigned because of it.
</p>
<Tip>Pull requests, reactions and project boards aren't in the file, so they stay on GitHub.</Tip>

<h2 id="clickup" tabindex="-1">ClickUp</h2>
<p>
	In ClickUp, open your Workspace's settings, then Import and export, and export your task data as
	CSV. Only the Workspace owner can. For one List with its custom fields, export that List's view as
	CSV instead.
</p>
<p>
	Choose the file under Import from another app, ClickUp. It opens the same column screen as a CSV
	file, with ClickUp's columns already matched, so a heading ClickUp renames only means choosing
	that column by hand.
</p>
<p>
	Each List becomes a project named after it, and where two Spaces each have a List of the same
	name, the Space's name follows in parentheses. You can put everything in one project or the inbox
	instead.
</p>
<p>
	Statuses keep ClickUp's names, each counted as to do, in progress or done by its name. Done tasks
	are done as of the day they closed where the file says, and otherwise as of the day you import.
</p>
<p>
	Only items of the type Task come in. Leads, Accounts and ClickUp's other types are left out, and
	the dry run lists them. A subtask comes in under its parent, or on its own when its parent is left
	out or missing.
</p>
<p>
	Due and start dates fall on the day they are in your time zone, with a due time only where ClickUp
	had one. Priorities 1 to 4 become urgent, high, medium and low; tags become labels, checklists
	your checklist, and the time estimated your estimate.
</p>
<p>
	Assignees are matched to people in the workspace by name or email address; anyone else is left
	off, and the dry run lists them. Comments come in as comments of yours, each starting with who
	wrote it in ClickUp and when.
</p>
<p>
	The notes start with a link back to the task in ClickUp. Attachments can't be fetched from the
	file, so the notes end with a list of links to them instead.
</p>
<Tip>Time tracked in ClickUp stays behind, and so do the attachments themselves.</Tip>

<h2 id="asana" tabindex="-1">Asana</h2>
<p>
	In Asana, open the project, then the menu beside its name, and choose Export/Print, then CSV. The
	file holds one project, so export each one you want.
</p>
<p>
	Choose the file under Import from another app, Asana. It opens the same column screen as a CSV
	file, with Asana's columns already matched, so a heading Asana renames only means choosing that
	column by hand.
</p>
<p>
	The tasks go in a project named after Asana's, or one you have of that name. A task in several
	projects goes in the one the rest of the file is in. You can put everything in one project or the
	inbox instead.
</p>
<p>
	Each section becomes a status, counted as to do, in progress or done by its name, so a board shows
	the same columns. Completed tasks come in done, as of the day Asana says.
</p>
<p>
	Subtasks come in under their parent, and dependencies between tasks in the file come in too. Tags
	become labels, and a Priority field of high, medium or low becomes the task's priority.
</p>
<p>
	Assignees are matched to people in the workspace by email address; anyone else is left off, and
	the dry run lists them. The notes start with a link back to the task in Asana.
</p>
<Tip>Asana's file has no comments or attachments, so they stay in Asana.</Tip>

<h2 id="todoist" tabindex="-1">Todoist</h2>
<p>
	In Todoist, open the project, then the menu of three dots at its top right, and choose Export as a
	template, then Export as a file. The file holds one project, so export each one you want.
</p>
<p>
	Choose the files under Import from another app, Todoist, all at once. Each becomes a project named
	after its file, or goes in one you have of that name. You can put everything in one project or the
	inbox instead.
</p>
<p>
	Each section becomes a status, counted as to do, in progress or done by its name, so a board shows
	the same columns. A task indented under another comes in as its subtask. Labels written in a task,
	such as @errands, become labels, and p1 to p3 become urgent, high and medium.
</p>
<p>
	Todoist writes a due date the way it was typed. Dates such as Nov 20 and times such as 9am come in
	as they are, and repeats such as every monday or every! 3 days keep repeating from the next day
	they fall on. A date counted from the day of the export, such as tomorrow or in 6 days, is counted
	from the day you import instead, and the dry run says how many there are. A task with both a date
	and a deadline keeps the deadline at the top of its notes.
</p>
<p>
	Comments come in starting with who wrote them and when, with their files as links. The person a
	task is assigned to is matched to someone in the workspace by name; anyone else is left off, and
	the dry run lists them.
</p>
<Tip>Todoist's file has no completed tasks or reminders, so they stay in Todoist.</Tip>

<h2 id="monday" tabindex="-1">monday.com</h2>
<p>
	In monday.com, open the board, then the menu of three dots beside its name, and choose to export
	the board to Excel. Tick updates and subitems, so they are in the file. The file holds one board,
	so export each one you want.
</p>
<p>
	Choose the files under Import from another app, monday.com, all at once. Each board becomes a
	project of its name, or goes in one you have of that name. You can put everything in one project
	or the inbox instead.
</p>
<p>
	Each item becomes a task, and its subitems its subtasks. Each group and tag becomes a label, so
	the grouping is kept. The Status column's values become statuses, counted as to do, in progress or
	done by their names, so Working on it and Stuck are in progress. Owner or Person is matched to
	people in the workspace by name; anyone else is left off, and the dry run lists them. Due date,
	the timeline's start, Priority, Notes and Files come in too, the files as links. You can match any
	other column to a custom field you have.
</p>
<p>An item or subitem with a timeline but no date of its own is due when its timeline ends.</p>
<p>
	Updates and their replies come in as comments, starting with who wrote them and when. If the file
	has no updates, or names subitems it leaves out, the dry run says so, and exporting again with
	them ticked brings them.
</p>
<Tip>monday.com's file has no updates on subitems, so they stay in monday.com.</Tip>

<h2 id="notion" tabindex="-1">Notion</h2>
<p>
	In Notion, open Settings, then General, and choose Export all workspace content, or export one
	page and what is under it from the page's menu of three dots. Choose Markdown &amp; CSV, and
	include subpages and files. With or without folders for subpages, the pages keep their tree.
</p>
<p>
	Choose the zip under Import from another app, Notion, as it downloaded. It can be up to 500 MB,
	and a zip that unpacks into far more than its own size is refused. Its pages go in the wiki as the
	same tree, under a new page called Notion by default, or at the top of the workspace wiki or in a
	project's wiki. Links between them point at the new pages, and a link to a page that is not in the
	export becomes its text. Images and files are uploaded as the page's own, so the import needs a
	connection; one over 25 MB is left out, and the dry run names it.
</p>
<p>
	Each database comes in as tasks, in a project of its name, or as pages under the page it was on.
	As tasks, its columns are matched by their names, and Match columns lets you change them; a
	Priority of Critical becomes urgent. A row's page becomes the task's notes, and properties with
	nowhere to go become a table at the top of them. As pages, each row is a page with its properties
	in a table at the top.
</p>
<p>
	Callouts become note boxes, without the icon Notion gives them, and toggles their heading with
	what was inside below it. People are matched to someone in the workspace by name; anyone else is
	left off, and the dry run lists them.
</p>
<Tip>Notion's export has no comments or page history, so they stay in Notion.</Tip>

<h2 id="confluence" tabindex="-1">Confluence</h2>
<p>
	In Confluence, open a page, then its menu of three dots, Export, Export to Markdown. Each file
	holds one page, without the pages under it, so export each page you want.
</p>
<p>
	Choose all the files at once under Import from another app, Confluence. Each becomes a page,
	titled with its first heading, under a new page called Confluence by default, or at the top of the
	workspace wiki or in a project's wiki. They go side by side, in title order, since the files don't
	say which page was under which: drag them into a tree afterwards. The dry run says when a page of
	that title is already there; importing it again makes another.
</p>
<p>
	Text, headings and tables come in. Pictures and attached files are not in Confluence's Markdown,
	so add them in Mila afterwards. Links keep going where they went, links to Confluence pages
	included.
</p>
