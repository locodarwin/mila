<script lang="ts">
	import Tip from '../../components/help/Tip.svelte';
	import { ALL_DAY_AT, formatTime } from '../../dates';

	const nine = formatTime(ALL_DAY_AT);
</script>

<h2 id="reminders" tabindex="-1">Reminders</h2>
<p>
	Open a task and choose Add a reminder. A task with a due time offers the due time itself, 15
	minutes, an hour or a day before it, and {nine} on the due date when it is due later that day. A task
	with only a date offers {nine} on the day and {nine} the day before. Every task offers tomorrow at {nine},
	or pick any date and time. Choices already in the past are left out.
</p>
<p>
	A reminder tied to the due date (at the due time, before it, or on the morning of the due date)
	moves when the task's date or time changes, and comes along when a repeating task comes back.
	Tomorrow at {nine}, and a date and time you pick, stay where they are. Reminders are yours: nobody
	else sees the ones you set.
</p>

<h2 id="channels" tabindex="-1">Where notifications go</h2>
<p>
	In Settings, Notifications you choose where each kind goes: in the app (the bell), as a push, or
	by email. The kinds are mentions, tasks assigned to you, comments on tasks you made or are
	assigned to, comments in threads you wrote in on wiki pages, a task of yours that was waiting
	being ready to start, your tasks reaching their due time, reminders, someone joining through your
	invite, and mail you sent to one of your addresses becoming a task.
</p>
<p>
	When a workspace, project or task is deleted, its notifications go too, read or not. If it is
	restored from the trash, they come back.
</p>
<p>
	The X on a notification clears it, and Clear all clears the whole list. Any email or push still
	waiting to go out for what you clear goes with it.
</p>
<Tip
	>Clearing is for good, on all your devices, and it takes away only the notification, not what it
	was about.</Tip
>
<p>
	Mentions and reminders always show in the app, where the list is their record. You are never told
	about your own changes.
</p>

<h2 id="push" tabindex="-1">Push notifications</h2>
<p>
	A push reaches a phone or computer even when Mila is not open. Turn push on in each browser you
	want it in, from Settings, Notifications, and send a test there to check it arrives. Some servers
	are not set up to send pushes; the page says so.
</p>
<p>
	Push on a device ends with your sign-in there, after 30 days without opening Mila on it or 90 days
	in any case. Three days before, the device gets one push saying what keeps it going: open Mila, or
	sign in again.
</p>

<h2 id="digest" tabindex="-1">The daily digest</h2>
<p>
	The digest is one email a day, at the hour you choose in Settings, Notifications, with your
	overdue tasks, the ones due that day, and the notifications you have not read. On a day with
	nothing to say, no email comes.
</p>
<p>
	Tasks in archived projects stay quiet: no reminders, no due notices and no place in the digest's
	lists of what is due. Unarchive the project and its dates count again.
</p>

<h2 id="weekly" tabindex="-1">The weekly report</h2>
<p>
	The weekly report is an email on the first day of your week, at the same hour as the digest, for
	each workspace you are in. It tells you about the week before, against the week before that: the
	tasks done and made, how many were overdue as it ended, and the median cycle time. Then come a
	line for each project, the time you logged (and everyone, if you are a member), and your tasks
	that are overdue, waiting on others or past their estimate, each with a link.
</p>
<p>
	It counts only what you can see, as Reports does. A week with nothing done, made or logged in a
	workspace sends nothing for it. Turn it on in Settings, Notifications, under Summaries by email;
	your first day of the week is the one in Settings, Preferences.
</p>

<h2 id="mentions" tabindex="-1">Mentions</h2>
<p>
	In a comment, type @ and choose a person from the list. They get a notification that links to the
	task.
</p>

<h2 id="calendar" tabindex="-1">Calendar feeds</h2>
<p>
	Settings, Calendar feeds makes a feed you can subscribe to in Google Calendar, Apple Calendar or
	Outlook: your own tasks (assigned to you, and My Day), one project, or a saved view. Tasks with a
	due date show as events that link back to the task. A feed only reads; it never changes anything
	in Mila.
</p>
