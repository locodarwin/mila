<script lang="ts">
	import Tip from '../../components/help/Tip.svelte';
	import Keys from '../../components/help/Keys.svelte';
</script>

<h2 id="formatting" tabindex="-1">Formatting</h2>
<p>
	A task's notes and a wiki page are written in the same editor. Select some text and a small bar
	shows over it, with bold, italic, strikethrough, code, link and heading<Keys
		id="doc.toolbar"
		lead=". From the keyboard, "
		trail=" goes to the bar, the arrow keys move along it, and Escape goes back to the text"
		otherwise=""
	/>.
</p>
<p>
	The bar also changes the selected words' font and size, from the fonts themes use, and highlights
	them in one of five colors. A highlight's colors come from your theme, so words stay readable in
	every one.
</p>
<p>
	Markdown typed as you write works too: <code>**bold**</code>, <code>*italic*</code>,
	<code>#</code> and a space for a heading, <code>-</code> and a space for a list. The usual keys
	work as well: <Keys id="doc.bold" /> for bold, <Keys id="doc.italic" /> for italic,
	<Keys id="doc.strike" /> for strikethrough and <Keys id="doc.code" /> for code.
</p>

<h2 id="blocks" tabindex="-1">Lists, tables and more</h2>
<p>
	Type <code>/</code> at the start of a line for a menu of what can go there: a heading, a bulleted
	or numbered list, a checklist, a table, two or three columns, a foldable section, a saved view, a
	diagram, a code block, a quote, a note, tip or warning box, an image or a divider. Keep typing to
	narrow it, choose with the arrow keys and Enter, or press Escape to keep the <code>/</code> as it is.
</p>
<p>
	A checklist's boxes tick as you click them. They are part of the text: the task's own Checklist,
	further down the task, is still the one counted on rows and cards.
</p>
<p>
	In a table, Tab goes to the next cell, and from the last cell makes a new row. While the cursor is
	in a table, the bar adds rows and columns and deletes them, or the whole table. Code blocks are
	highlighted in the language named after the opening backticks, such as
	<code>```python</code>.
</p>

<h2 id="columns" tabindex="-1">Columns and foldable sections</h2>
<p>
	Columns put text, lists, images, boxes and tables side by side. Where there is no room, on a phone
	or in a task's panel, they show one under another. With the cursor in two columns, the bar makes
	one of them wider, or takes the columns out and keeps what was in them.
</p>
<p>
	A foldable section is a title with anything under it. The arrow beside the title opens and closes
	it for you alone: nobody else's page changes. Whoever writes it chooses how it starts, with Starts
	open in the bar while the cursor is in its title. A link to a heading inside one, or a comment in
	it, opens it.
</p>

<h2 id="views" tabindex="-1">A view in a page</h2>
<p>
	Choose View from the <code>/</code> menu to show a shared saved view's tasks where you are writing.
	They are always as they are now, and work offline. Check one off or open it from there, or open the
	whole view. Everyone sees only the tasks they could see anyway.
</p>
<p>
	Only shared views can go in a page, since someone's own view would show everyone else nothing.
</p>
<p>
	A view shows the way it's laid out: a board view as a board and a table view as a table. Calendar
	and timeline views show as a list. To show one differently in this page only, click it and choose
	Show as in the formatting bar: As the view, List, Board or Table.
</p>
<p>
	Drag a card to another column to move its task, just as on the board. A table's cells can't be
	changed in a page; open the task, or the view, for that. On a phone, a board scrolls sideways and
	a table shows as a list.
</p>
<p>
	Lists and tables show the first ten tasks, with Show all for the rest. A board's columns show five
	cards each, with a button for the rest.
</p>

<h2 id="reports" tabindex="-1">A report in a page</h2>
<p>
	Choose Report from the <code>/</code> menu, then a shared saved report, to show one of its charts where
	you are writing. To show another, click it and choose Chart in the formatting bar: any chart of any
	of its tabs, or the numbers at the top of one.
</p>
<p>
	The chart is worked out on each reader's device, so it is always current and works offline, and it
	counts only the tasks that reader can see. It follows the saved report: change the report's period
	or filter, and every page showing it changes too. Point at it or tap it for the numbers, as on the
	Reports page.
</p>
<p>
	Only shared reports can go in a page. Save one from the Reports page with Save as view, and share
	it.
</p>

<h2 id="diagrams" tabindex="-1">Diagrams</h2>
<p>
	Choose Diagram from the <code>/</code> menu, then a kind: a flowchart, sequence diagram, state
	diagram, class diagram, entity relationship diagram, org chart, architecture diagram, mind map,
	timeline, Gantt chart or pie chart. Each starts as a small example to change. Typing a kind's name
	after the <code>/</code> finds it too.
</p>
<p>
	A diagram is drawn from text in Mermaid's language, described at mermaid.js.org. While the cursor
	is in a diagram, its text shows under the drawing, or beside it on a wide page, and the drawing
	changes as you type. If the text has a mistake, the last drawing stays, dimmed, with the mistake
	and its line under the text. Escape leaves the text, and Edit on the diagram goes back to it.
</p>
<p>
	A line such as <code>accTitle: Request flow</code> names the diagram, for screen readers, full
	screen and downloads, and <code>accDescr:</code> describes it.
</p>
<p>
	The buttons on a diagram show it full screen, where you can zoom and move it, copy its text, or
	download it as an SVG or PNG picture. Its colors come from your theme.
</p>
<p>
	A diagram is kept as a code block in the language <code>mermaid</code>, which GitHub, Obsidian and
	Notion draw too. People can change one together, and a page's history shows what changed in its
	text. Diagrams are drawn on your device, so they work offline: the common kinds always, and the
	others once one has been drawn while connected.
</p>

<h2 id="images" tabindex="-1">Images</h2>
<p>
	Choose Image from the <code>/</code> menu to put a picture in the text. The images already attached
	to the task or page show to pick from, or upload a new one, which is attached for you. Images can be
	PNG, JPEG, GIF, WebP or SVG, and uploading needs a connection.
</p>
<p>
	Click an image and the bar gives it a size (a quarter, half, three quarters or the full width), a
	place (the left or right, with the text beside it, or the middle) and a caption under it. On a
	phone an image takes the full width.
</p>

<h2 id="type" tabindex="-1">A page's font and size</h2>
<p>
	A page shows in your theme's page font and size, which the theme editor's Pages group sets. The
	same group colors headings, links, code, quotes and tables, in pages and task notes alike. To give
	one page a font and size for everyone who reads it, choose Font and size from the page's menu.
	Words picked out with the bar keep their own.
</p>

<h2 id="links" tabindex="-1">Links to pages and tasks</h2>
<p>
	Type <code>[[</code> anywhere in the text for a menu of pages and tasks to link to, narrowed as you
	type their name. Its last choice makes a new page with the name typed and links to it: under the page
	you are on, or from a task's notes in its project's wiki (the workspace's for a task in the Inbox).
</p>
<p>
	A link to a task shows the task's status beside it, and a link to a page or task in the trash says
	so. Click a link to follow it: a task opens beside the page you are on. When a page or task is
	renamed, links that showed its old name show the new one, wherever they are. Renaming a project
	does the same for links to its wiki's main page.
</p>
<p>
	A link in something a guest can read keeps its old name if the guest cannot see what it links to,
	so the new name stays with the people who can.
</p>
<p>
	A page ends with what links to it, and a task lists the pages and notes that link to it, under
	Linked from.
</p>

<h2 id="markdown" tabindex="-1">Markdown</h2>
<Tip>Everything is kept as Markdown, which is what exports and the API get.</Tip>
<p>
	Markdown, beside the notes or above a page, shows the text as Markdown to read or edit; press it
	again to go back to the formatted text. Boxes are written the way GitHub writes them, starting
	with a line such as
	<code>&gt; [!NOTE]</code>.
</p>
<p>
	What Markdown has no way to say is written so that other Markdown apps still show the words. A
	foldable section is <code>&lt;details&gt;</code>, as the web writes one. Highlights, fonts and
	sizes are a little HTML around the words. Columns, a view, an image's size and the page's font are
	short notes such as <code>&lt;!-- mila:columns --&gt;</code>, which other apps leave out, so there
	the columns are one under another and a view is a link to it.
</p>
<p>Other HTML typed into the text stays as text: nothing anyone writes is shown as HTML.</p>

<h2 id="wide" tabindex="-1">Room to write</h2>
<p>
	The button beside a task's notes opens them wide, with the rest of the task put aside for long
	writing. Press it again, or Escape, to bring the rest back.
</p>
