# Writing in notes and pages

A task's notes and a wiki page are written in the same editor.

## Formatting

Select some text and a small bar shows over it, with bold, italic, strikethrough, code, link and heading. From the keyboard, `Alt+F10` goes to the bar, the arrow keys move along it, and `Escape` goes back to the text.

The bar also has Font, Size and Highlight for the selected words:

- Font offers the fonts themes use: Inter, Nunito, Source Serif, JetBrains Mono, and this device's own sans, serif and mono. Theme's goes back to your theme's font.
- Size makes words Small, Large or Huge, or Normal again.
- Highlight colors them yellow, green, blue, pink or gray. No highlight takes it off.

A highlight's colors come from your theme, so words stay readable in every one.

In a wiki page, the bar also has Comment. See [Comments on a page](wiki.md#comments-on-a-page).

Markdown typed as you write works too: `**bold**`, `*italic*`, `#` and a space for a heading, `-` and a space for a list.

These keys are always available, and cannot be changed:

| Key | Does |
|---|---|
| `Ctrl+B` | Bold |
| `Ctrl+I` | Italic |
| `Ctrl+Shift+S` | Strikethrough |
| `Ctrl+E` | Code |

## Lists, tables and more

Type `/` at the start of a line for a menu of what can go there:

- a heading
- a bulleted or numbered list
- a checklist
- a table
- a code block
- a quote
- two or three columns
- a foldable section
- a saved view
- a note, tip or warning box
- an image
- a divider

Keep typing to narrow it, choose with the arrow keys and `Enter`, or press `Escape` to keep the `/` as it is. The list scrolls to keep your choice in view.

A checklist's boxes tick as you click them. They are part of the text. A task's own [Checklist section](tasks.md#checklists) is a separate thing, and it is the one counted on rows and cards.

In a table, `Tab` goes to the next cell, and from the last cell makes a new row. While the cursor is in a table, the bar adds rows and columns and deletes them, or the whole table.

Code blocks are highlighted in the language named after the opening backticks, such as ` ```python `.

## Columns and foldable sections

Columns put text, lists, images, boxes and tables side by side. Columns do not go inside columns. Where there is no room, on a phone or in a task's panel, they show one under another.

With the cursor in two columns, Column widths in the bar makes them Equal, Left wider or Right wider. Take out the columns, in the bar for two or three, takes the columns away and keeps what was in them.

A foldable section is a title with anything under it. The arrow beside the title opens and closes it for you alone: nobody else's page changes. `Enter` in the title goes to the text under it.

Whoever writes a foldable section chooses how it starts, with Starts open in the bar while the cursor is in its title. Open or close it here, beside it in the bar, opens or closes it from the keyboard. A link to a heading inside one, or a comment in it, opens it.

## A view in a page

Choose View from the `/` menu, then pick a saved view under Show a view here, to show its tasks where you are writing. They are always as they are now, and work offline. Check one off or open it from there, or choose Open the view.

Only shared views can go in a page, since someone's own view would show everyone else nothing. Everyone sees only the tasks they could see anyway. A view that has been deleted, or that you cannot open, says so.

A view shows the way it is laid out: a board view as a board, and a table view as a table. Calendar and timeline views show as a list.

To show one differently in this page only, click it and choose Show as in the formatting bar: As the view, List, Board or Table.

Drag a card to another column to move its task, just as on the board. A table's cells cannot be changed in a page; open the task, or the view, for that. On a phone, a board scrolls sideways and a table shows as a list.

Lists and tables show the first ten tasks, with Show all for the rest. A board's columns show five cards each, with a button for the rest.

## A report in a page

Choose Report from the `/` menu, then a shared report view under Show a report here, to show one of its charts where you are writing. To show another, click it and choose Chart in the formatting bar: any chart of any of its tabs, or the numbers at the top of one.

The chart is worked out on each reader's device, so it is always current and works offline, and it counts only the tasks that reader can see. It follows the report view: change its period or filter, and every page showing it changes too. Point at it or tap it for the numbers, as on the Reports page.

Only shared report views can go in a page. Make one on the Reports page with Save as view, then share it.

## Images

Choose Image from the `/` menu to put a picture in the text. The images already attached to the task or page show to pick from, or upload a new one, which is attached for you.

Images can be PNG, JPEG, GIF or WebP, up to 25 MB. Uploading needs a connection.

Click an image and the bar has three more choices:

- Image size: a quarter, half, three quarters or the full width, or As it is.
- Where it sits: on the left or the right, with the text beside it, or in the middle.
- Caption: a line under the picture.

On a phone an image takes the full width.

## A page's font and size

A page shows in your theme's page font and size, which the Pages group in the [theme editor](themes.md) sets. The same group colors headings, links, code, quotes and tables, in pages and in task notes alike.

To give one page a font and size for everyone who reads it, choose Font and size from the page's menu. The size can be Small, Normal or Large. Words picked out with the bar keep their own font and size.

## Links to pages and tasks

Type `[[` anywhere in the text for a menu of pages and tasks to link to, narrowed as you type their name. As with the `/` menu, the list scrolls to keep your choice in view.

Its last choice makes a new page with the name typed, and links to it. The new page goes under the page you are on. From a task's notes it goes in its project's wiki, or the workspace's for a task in the Inbox.

A link to a task shows the task's status beside it, and a link to a page or task in the trash says so. A link to something you cannot see says Not available here.

Click a link to follow it. A task opens beside the page you are on.

Only a link to this Mila's own address counts as a link to a page or task. A link to another site stays an ordinary web link, even if its address looks like one of Mila's, and it opens in a new tab.

When a page or task is renamed, links that showed its old name show the new one, wherever they are. Renaming a project does the same for links to its wiki's main page. A link worded some other way keeps its words.

A link in something a guest can read keeps its old name if the guest cannot see what it links to, so the new name stays with the people who can.

A page ends with what links to it, and a task lists the pages and notes that link to it, under Linked from.

## Markdown

Everything is kept as Markdown, which is what exports and the API get.

The Markdown button, beside the notes or above a page, shows the text as Markdown to read or edit. Press it again to go back to the formatted text.

Boxes are written the way GitHub writes them, starting with a line such as `> [!NOTE]`.

What Markdown has no way to say is written so that other Markdown apps still show the words. A foldable section is `<details>`, as the web writes one. Highlights, fonts and sizes are a little HTML around the words. Columns, a view, an image's size and the page's font are short notes such as `<!-- mila:columns -->`, which other apps leave out. There the columns are one under another, and a view is a link to it.

Other HTML typed into the text stays as text. Nothing anyone writes is shown as HTML.

## Room to write

The button beside a task's notes opens them wide, with the rest of the task put aside for long writing. Press it again, or `Escape`, to bring the rest back.

For a wiki page, see [Giving a page more room](wiki.md#giving-a-page-more-room).

## Writing together and offline

Two people can write in the same text at once, and everything each of them types stays. You can also write offline, and your text merges when you reconnect. See [Offline and sync](offline-and-sync.md#when-two-people-edit).

Undo while you type takes back your own typing, never anyone else's.

## Next

- [The wiki](wiki.md)
- [People and roles](people-and-roles.md)
- [Tasks](tasks.md)
