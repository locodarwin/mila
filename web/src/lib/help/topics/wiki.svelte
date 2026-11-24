<script lang="ts">
	import Shot from '../../components/help/Shot.svelte';
	import Keys from '../../components/help/Keys.svelte';
</script>

<h2 id="wikis" tabindex="-1">The workspace's wiki and each project's</h2>
<p>
	A wiki is for what you want to keep that is not a task: how things are done, decisions, lists and
	plans. The workspace has one, under Workspace Wiki in the sidebar<Keys
		id="go.wiki"
		lead=" or with "
		trail=","
		otherwise=","
	/> and each project has one of its own, on the project's Project Wiki tab beside its tasks. A project
	opens on whichever tab you had open last.
</p>
<Shot
	name="wiki"
	alt="The workspace's wiki: the tree of pages on the left, and a page with headings, a table and code on the right."
/>
<p>
	Every wiki starts with a main page, named after its workspace or project. Guests see the wikis of
	the projects shared with them, and never the workspace's.
</p>

<h2 id="pages" tabindex="-1">Pages and subpages</h2>
<p>
	The tree beside the page shows every page in the wiki. The add button at the top of the tree, or
	New page in the page's menu, adds a page under the one you are on; type its title and press Enter
	to start writing. The path above the title shows where the page sits.
</p>
<p>
	Drag a page in the tree to move it before, after or into another page<Keys
		id="wiki.move"
		lead=", or focus it and move it with "
		otherwise=""
	/>. Up and down move it among its siblings, right puts it under the page above it, and left takes
	it out a level.
</p>
<p>
	The tree folds away when you want the room<Keys
		id="wiki.tree"
		lead=", with its button or "
		otherwise=", with its button"
	/>, and comes back the same way. On a phone, Show pages opens the tree as a sheet.
</p>

<h2 id="writing" tabindex="-1">Writing</h2>
<p>
	Pages are written in the same editor as a task's notes, and saved as Markdown. Use the toolbar at
	the top of the page, select text for the formatting bar, or type <code>/</code> at the start of a line
	for lists, checklists, tables, boxes and images. Writing in notes and pages, under Help, has the details.
</p>
<p>
	Several people can write in a page at once, and everything each of them types stays. Their
	pictures show above the page while they have it open, and their cursors show in it as they type.
</p>

<p>
	To give a page more room, drag the grip in its bottom right corner, or anywhere along its right
	edge. The edge of the list of pages drags too. You can also focus an edge and use the arrow keys.
	Double-click the page's grip, or press Enter on it, to go back to where it started. Each device
	remembers its own widths.
</p>
<p>
	When there is room to the right of the page, a panel there shows the page's contents, its
	headings, so you can jump to one. Choose Pages at the top of the panel to see the wiki's pages
	instead. The button beside them hides the panel, and the page then takes all the room there is
	unless you drag it narrower. The same button at the top right of the page brings the panel back.
</p>

<h2 id="comments" tabindex="-1">Comments on a page</h2>
<p>
	Select some words in a page and choose Comment in the formatting bar<Keys
		id="doc.comment"
		lead=", or press "
	/>. Your comment goes in the Comments tab beside the page, and the words are highlighted.
	Selecting a highlight, or moving the cursor into one, picks its thread. Anyone who can read the
	page can comment.
</p>
<p>
	The comments button above the page opens the tab. When there is no room beside the page it opens
	over the page's edge, and on a phone as a sheet.
</p>
<p>
	A highlight follows its words as people edit around them. If the words are deleted, the thread
	stays and shows what it was on. Highlights are never part of the page's text, so they stay out of
	its Markdown and its history.
</p>
<p>
	Resolve a thread when it is settled: its highlight goes and it moves to Resolved, where anyone can
	reopen it. People you mention are told, and so is everyone already in the thread.
</p>

<h2 id="history" tabindex="-1">History</h2>
<p>
	History, in a page's menu, lists a version of the page for each time someone wrote in it: the same
	person's writing counts as one version until they stop for five minutes. Each version shows who
	wrote it and when, with the words it added underlined and those it removed struck through.
</p>
<p>
	Restore this version puts a page back as it was then. The page as it was before stays in the
	history too, so a restore can be undone, or restored over in turn. History needs a connection.
</p>
<p>
	A page keeps its newest 500 versions. On a very long page it keeps fewer, the oldest going first.
</p>

<h2 id="deleting" tabindex="-1">Deleting pages</h2>
<p>
	Delete is in the page's menu. A deleted page takes its subpages with it, and a message offers
	Undo. Deleted pages wait in the trash for 30 days, where Restore brings a page back with its
	subpages. A main page cannot be deleted: it goes with its project or workspace.
</p>

<h2 id="finding" tabindex="-1">Finding a page</h2>
<p>
	The command palette finds pages by their titles and by what they say, beside tasks and projects.
	Copy link, in a page's menu, gives an address that opens the page for anyone who can see it.
</p>

<h2 id="exports" tabindex="-1">Exports</h2>
<p>
	Exports, in Settings, Import and export, hold the wikis you can see. A JSON export keeps every
	page with its history and files, and brings them back when imported, links and images pointing at
	the pages, tasks and files that came in with them. A Markdown export writes a Wiki folder for each
	wiki, a file per page and a folder for each page's subpages, with links between pages and tasks
	pointing at their files. Both hold a page's comments, the Markdown under Comments at the end of
	its file.
</p>
