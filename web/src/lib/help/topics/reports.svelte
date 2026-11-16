<script lang="ts">
	import Keys from '../../components/help/Keys.svelte';
</script>

<h2 id="page" tabindex="-1">The Reports page</h2>
<p>
	Reports in the sidebar<Keys id="go.reports" lead=", or " trail="," /> shows how work is going, in three
	tabs: Project status, Workload, and Throughput and cycle time. Your device works them out from the tasks
	it holds, so they work offline, and they count only the tasks you can see. A guest's reports cover the
	projects shared with them.
</p>
<p>
	Reports count every task, subtasks too, in the inbox and the projects in use. Tasks in the trash,
	tasks in archived projects and canceled tasks are left out. A task you reopen counts as open until
	you finish it again, and then it counts as done on that day.
</p>
<p>Time logged has a page of its own: Time in the sidebar, which Reports links to.</p>
<p>
	A summary of each week can come by email too: turn on the weekly report in Settings,
	Notifications. The server works it out by the same rules.
</p>

<h2 id="project" tabindex="-1">One project</h2>
<p>
	Project, at the top, shows every tab for one project alone, or for all of them. With one project,
	Tasks by status uses that project's own statuses by name, in its order. A project's menu has Open
	its reports, which opens the page with that project chosen, and the page's address names it, so a
	link to one project's reports can be shared.
</p>

<h2 id="filter" tabindex="-1">The filter and the period</h2>
<p>
	Filter narrows every tab to the tasks you choose, the same way a saved view's filter does: your
	own work, a label or anything else the filter offers, within the project chosen. Finished tasks
	always count in reports, so the filter has no Show finished tasks.
</p>
<p>
	The period sets the weeks the charts cover: the last 6 weeks, the last 3 months, this year, or
	dates you pick, from 2000 on and five years at most. Weeks start on the first day of the week set
	in your preferences. Workload is open work as it stands today, so it has no period.
</p>
<p>
	The tab, the project, the filter and the period are kept on this device for each workspace, and
	come back the next time you open Reports.
</p>

<h2 id="status" tabindex="-1">Project status</h2>
<p>
	The tiles count what is open now, what is done out of all the tasks, what is overdue, and what is
	due between today and the end of next week.
</p>
<p>
	The burnup has two lines: all the tasks there were by the end of each week, and how many of them
	were done. When the lines meet, everything there was is done. A task counts from the day it was
	made.
</p>
<p>
	Where the open work is splits the open tasks by the kind of their status: backlog, to do and in
	progress. A task in the inbox with no status counts as to do.
</p>
<p>
	Needs attention lists open tasks that are overdue, waiting on tasks that are not done yet, or past
	their estimate by the time logged on them. Choose one to open it.
</p>
<p>
	Tasks by status splits every task, done ones too, by its status: by kind across all projects,
	since each project names its statuses its own way, or by the project's own statuses when one is
	chosen. Open work by priority splits the open tasks from urgent to no priority. Each shows up to
	six parts, the smallest of any more folded into Other.
</p>
<p>
	How overdue counts the overdue tasks by how late they are: under a week, 1 to 4 weeks, 1 to 3
	months and longer. Open work by label ranks the 8 labels with the most open tasks; a task with
	several labels counts under each, so these are not parts of a whole. With all projects, Open work
	by project ranks the 8 projects with the most open tasks, the inbox among them.
</p>

<h2 id="workload" tabindex="-1">Workload</h2>
<p>
	Open work by person adds up each person's estimated hours and counts their open tasks. A task with
	several assignees counts for each of them. A task with no estimate counts as a task but adds no
	hours, and the page says how many there are. Tasks nobody is assigned come last.
</p>
<p>
	There is nothing to set about anyone's hours or capacity: the chart shows the load, and you judge
	it.
</p>
<p>
	Due by week counts the open tasks by the week they are due: overdue first, then this week, next
	week, the two weeks after and anything later. Open tasks with no due date are left out, and the
	page says how many.
</p>

<h2 id="flow" tabindex="-1">Throughput and cycle time</h2>
<p>
	Made and done each week shows how many tasks were made and how many were finished in each week of
	the period. When made stays above done, work is piling up.
</p>
<p>
	Cycle time is how long a task took from the first time it went to a status of kind in progress to
	when it was done. The tiles give the typical time (the median: half took less) and the time within
	which 8 in 10 were done; the chart shows how many took under a day, 1 to 2 days and so on.
</p>
<p>
	Tasks done without ever going in progress have no cycle time, so they are left out of it and the
	page says how many. To count a task's time, move it to an in progress status when work on it
	starts.
</p>
<p>
	Cumulative flow stacks, for each day of the period, how many tasks were to do (the backlog among
	them), in progress and done. Done grows at the bottom; a middle band that widens is work piling up
	in progress. A task counts as in progress from the first time it went there, even if it went back
	later.
</p>
<p>
	Cycle time, task by task, shows each task done in the period as a dot: along the bottom when it
	was done, up the side how many days it took from start to done, with dashed lines at the median
	and at 8 in 10. Dots far above the lines are the tasks that took much longer than most.
</p>

<h2 id="saved" tabindex="-1">Saved reports</h2>
<p>
	Save as view keeps the report as it is set up: the tab, the period, the project and the filter. It
	becomes a view of its own kind, a report, in Views in the sidebar, named for what it shows, such
	as Website launch report. Only you see it until you share it, like any view you save.
</p>
<p>
	A saved report opens as this page does, under its own name. Changing its tab, period, project or
	filter there changes the report, for everyone who sees it. A saved report of one project always
	reports on that project. A guest's changes stay on their own screen.
</p>
<p>
	A shared saved report can go in a wiki page or a task's notes, one chart at a time. See Writing.
</p>

<h2 id="charts" tabindex="-1">Charts, tables and CSV</h2>
<p>
	Point at a chart, or tap it, to see the numbers at that spot. From the keyboard, move to a chart,
	and Left and Right (or Up and Down) step through its weeks, bars or parts, Home and End go to the
	first and last, and each one is read out as you reach it.
</p>
<p>
	Show as table puts a chart's numbers in a table in its place. Download CSV saves the tables of the
	tab you are on as one file, with dates written as year, month and day, for a spreadsheet.
</p>
