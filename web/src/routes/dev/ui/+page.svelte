<script lang="ts">
	import AppearanceControls from '#lib/components/AppearanceControls.svelte';
	import Button from '#lib/components/Button.svelte';
	import { announce, announcer } from '#lib/announce.svelte.js';
	import Checkbox from '#lib/components/Checkbox.svelte';
	import Kbd from '#lib/components/Kbd.svelte';
	import LabelChip from '#lib/components/LabelChip.svelte';
	import PriorityIcon from '#lib/components/PriorityIcon.svelte';
	import ReminderIcon from '#lib/components/ReminderIcon.svelte';
	import ReminderList from '#lib/components/task/ReminderList.svelte';
	import { remindAtFor } from '#lib/reminders.js';
	import type { Reminder } from '#lib/sync/entities.js';
	import RepeatIcon from '#lib/components/RepeatIcon.svelte';
	import RepeatPicker from '#lib/components/task/RepeatPicker.svelte';
	import CalendarFeedLink from '#lib/components/CalendarFeedLink.svelte';
	import type { RepeatMode } from '#lib/recurrence.js';
	import StatusIcon from '#lib/components/StatusIcon.svelte';
	import SyncDot from '#lib/components/SyncDot.svelte';
	import TaskCard from '#lib/components/TaskCard.svelte';
	import TaskRow from '#lib/components/TaskRow.svelte';
	import TextInput from '#lib/components/TextInput.svelte';
	import Toast from '#lib/components/Toast.svelte';
	import { UPDATE_MESSAGES } from '#lib/update.svelte.js';
	import { PALETTE, PRIORITIES, STATUS_CATEGORIES } from '#lib/theme/palette.js';
	import { collapse } from '#lib/motion.js';
	import FieldInput from '#lib/components/task/FieldInput.svelte';
	import HelpExample from '#lib/components/help/Example.svelte';
	import HelpLink from '#lib/components/help/HelpLink.svelte';
	import HelpShot from '#lib/components/help/Shot.svelte';
	import HelpTip from '#lib/components/help/Tip.svelte';
	import HelpKeys from '#lib/components/help/Keys.svelte';
	import Token from '#lib/components/help/Token.svelte';
	import { keymap } from '#lib/keys/keymap.svelte.js';
	import ProjectIcon from '#lib/components/ProjectIcon.svelte';
	import ProjectStylePicker from '#lib/components/ProjectStylePicker.svelte';
	import SidebarEditor from '#lib/components/SidebarEditor.svelte';
	import DatePreferences from '#lib/components/DatePreferences.svelte';
	import { formatDate, formatTime } from '#lib/dates.js';
	import type { Preferences, SidebarItem } from '#lib/preferences.svelte.js';
	import { engine } from '#lib/sync/engine.svelte.js';
	import BlockedMark from '#lib/components/BlockedMark.svelte';
	import { ui } from '#lib/ui.svelte.js';
	import TabBar from '#lib/components/TabBar.svelte';
	import TaskItem from '#lib/components/TaskItem.svelte';
	import SchedulePicker from '#lib/components/SchedulePicker.svelte';
	import Splitter from '#lib/components/Splitter.svelte';
	import { scheduler } from '#lib/schedule.svelte.js';
	import { SAMPLE_ME, SAMPLE_PROJECT, SAMPLE_WS, sampleRows } from './sample';

	const sizes = ['3xl', '2xl', 'xl', 'lg', 'md', 'sm', 'xs'];

	const sampleTasks = [
		{
			id: 1,
			title: 'Draft the onboarding email',
			status: 'in-progress',
			priority: 'high',
			labels: [{ name: 'Writing', color: 'purple' }],
			due: 'Nov 3'
		},
		{
			id: 2,
			title: 'Renew the domain',
			status: 'todo',
			priority: 'urgent',
			labels: [{ name: 'Admin', color: 'red' }],
			due: 'Today'
		},
		{
			id: 3,
			title: 'Sketch the calendar view',
			status: 'backlog',
			priority: 'low',
			labels: [{ name: 'Design', color: 'teal' }]
		}
	] as const;

	let tasks = $state([...sampleTasks]);
	let lastCompleted = $state<(typeof sampleTasks)[number] | null>(null);
	let demoChecked = $state(true);
	let demoText = $state('');
	let demoRule = $state<string | null>('FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH');
	let demoMode = $state<RepeatMode>('schedule');
	// A task due the day after tomorrow at 14:30, so its presets are all to come.
	const inTwoDays = new Date(Date.now() + 2 * 864e5);
	const demoDue = {
		dueDate: `${inTwoDays.getFullYear()}-${String(inTwoDays.getMonth() + 1).padStart(2, '0')}-${String(inTwoDays.getDate()).padStart(2, '0')}`,
		dueTime: '14:30'
	};
	let demoReminders = $state<Reminder[]>([
		{ id: 'r1', taskId: 't', offsetMinutes: 60, remindAt: remindAtFor(demoDue, 60)! },
		{
			id: 'r2',
			taskId: 't',
			offsetMinutes: null,
			remindAt: new Date(Date.now() + 864e5).toISOString()
		}
	]);

	// The sample workspace the views, fields and dialogs below work on.
	const sample = engine.sample(SAMPLE_WS, sampleRows());
	$effect(() => () => engine.dropSample(SAMPLE_WS));
	const sampleTasks2 = $derived(sample.allTasks());
	const sampleFields = $derived(sample.fields());
	const swipeRows = $derived(
		sample
			.tasks(SAMPLE_PROJECT)
			.filter((t) => !t.completedAt)
			.slice(0, 3)
	);
	let demoSidebar = $state(200);
	let demoExpanded = $state(false);
	let demoProject = $state({ icon: 'lucide:sprout', color: 'green' });
	let demoOrder = $state<SidebarItem[]>(['inbox', 'today', 'upcoming', 'views', 'projects']);
	let demoHidden = $state<SidebarItem[]>(['upcoming']);
	let demoDates = $state<Pick<Preferences, 'weekStart' | 'dateFormat' | 'clock'>>({
		weekStart: 'auto',
		dateFormat: 'auto',
		clock: 'auto'
	});
	const dateStyles = [
		{
			name: 'Month first, 12-hour',
			style: { weekStart: 'auto', dateFormat: 'month-day', clock: '12' }
		},
		{
			name: 'Day first, 24-hour',
			style: { weekStart: 'auto', dateFormat: 'day-month', clock: '24' }
		},
		{ name: 'ISO, 24-hour', style: { weekStart: 'auto', dateFormat: 'iso', clock: '24' } }
	] as const;

	function complete(id: number) {
		lastCompleted = tasks.find((t) => t.id === id) ?? null;
		tasks = tasks.filter((t) => t.id !== id);
	}

	function undo() {
		if (lastCompleted) {
			const restored = lastCompleted;
			tasks = sampleTasks.filter((t) => t.id === restored.id || tasks.some((x) => x.id === t.id));
			lastCompleted = null;
		}
	}
</script>

<svelte:head><title>Style guide · Mila</title></svelte:head>

<div class="guide">
	<header class="controls">
		<h1>Style guide</h1>
		<AppearanceControls preview />
	</header>

	<section>
		<h2>Color</h2>
		<!-- Every color token, from the registry (ColorDemo.svelte, loaded apart). -->
		{#await import('./ColorDemo.svelte') then { default: ColorDemo }}
			<ColorDemo />
		{/await}
		<h3>Palette</h3>
		<div class="wrap">
			{#each PALETTE as color (color)}
				<LabelChip {color} name={color} />
			{/each}
		</div>
	</section>

	<section>
		<h2>Type</h2>
		<div class="type">
			{#each sizes as size (size)}
				<p style:font-size={`var(--text-${size})`}>
					<span class="token">{size}</span> Plan the week, then do the work.
				</p>
			{/each}
			<h3 class="heading-sample">Headings use the theme's heading face</h3>
			<p class="tabular">Tabular numbers: 1,234.50 · 09:41 · Nov 28</p>
			<p><code>code uses JetBrains Mono: const due = new Date()</code></p>
		</div>
	</section>

	<section>
		<h2>Controls</h2>
		<div class="wrap">
			<Button variant="primary">Create task</Button>
			<Button>Secondary</Button>
			<Button variant="ghost">Ghost</Button>
			<Button variant="danger">Delete</Button>
			<Button variant="primary" size="sm">Small</Button>
			<Button disabled>Disabled</Button>
		</div>
		<div class="wrap">
			<div class="field">
				<TextInput label="Task title" placeholder="What needs doing?" bind:value={demoText} />
			</div>
			<Checkbox label="Show completed tasks" bind:checked={demoChecked} />
			<Checkbox label="Always on" checked disabled title="This one cannot be turned off." />
			<span class="wrap"><Kbd binding="Mod+k" /> <Kbd binding="q" /></span>
		</div>
	</section>

	<section>
		<h2>Status and priority</h2>
		<div class="wrap">
			{#each STATUS_CATEGORIES as category (category)}
				<span class="pair"><StatusIcon {category} />{category}</span>
			{/each}
		</div>
		<div class="wrap">
			{#each PRIORITIES as priority (priority)}
				<span class="pair"><PriorityIcon {priority} />{priority}</span>
			{/each}
		</div>
	</section>

	<section>
		<h2>Task rows</h2>
		<p class="note">Check a task: it strikes through, holds for a second, then collapses.</p>
		<div class="list">
			{#each tasks as task (task.id)}
				<div out:collapse>
					<TaskRow {...task} labels={[...task.labels]} oncomplete={() => complete(task.id)} />
				</div>
			{/each}
		</div>
		{#if lastCompleted}
			<div class="toast-slot"><Toast message="Task completed" onundo={undo} /></div>
		{/if}
	</section>

	<section>
		<h2>Cards</h2>
		<div class="cards">
			<TaskCard {...sampleTasks[0]} labels={[...sampleTasks[0].labels]} />
			<TaskCard {...sampleTasks[1]} labels={[...sampleTasks[1].labels]} lifted />
		</div>
	</section>

	<section>
		<h2>Repeat</h2>
		<p class="note">
			The task panel's repeat row: presets around the due date (here Thursday, October 29), a custom
			rule, and counting from completion. Rows, cards and calendar entries of a repeating task carry
			the icon.
		</p>
		<div class="wrap">
			<RepeatPicker
				rule={demoRule}
				mode={demoMode}
				anchor="2026-10-29"
				onchange={(rule, mode) => {
					demoRule = rule;
					demoMode = mode;
				}}
			/>
		</div>
		<div class="wrap">
			<span class="pair"><RepeatIcon size={16} label="Repeats every day" />Repeats</span>
		</div>
	</section>

	<section>
		<h2>Reminders</h2>
		<p class="note">
			The task panel's reminders row, for a task due in two days at 14:30: presets relative to the
			due time follow the task when it moves; the others are fixed times. Rows and cards of a task
			with a reminder of yours carry the bell.
		</p>
		<div class="wrap">
			<ReminderList
				task={demoDue}
				reminders={demoReminders}
				onadd={(when) =>
					demoReminders.push({
						id: `r${demoReminders.length + 1}-${Date.now()}`,
						taskId: 't',
						offsetMinutes: 'offset' in when ? when.offset : null,
						remindAt: 'offset' in when ? remindAtFor(demoDue, when.offset)! : when.at
					})}
				onremove={(id) => (demoReminders = demoReminders.filter((r) => r.id !== id))}
			/>
		</div>
		<div class="wrap">
			<span class="pair"><ReminderIcon size={16} />Has a reminder</span>
		</div>
	</section>

	<section>
		<h2>Notifications</h2>
		<p class="note">
			The bell sits in the sidebar (a row) and on the phone's top bar (an icon); it opens the panel,
			shown here in the page. Picking one marks it read and opens its task.
		</p>
		<!-- The bell, the panel, the settings matrix and push (NotificationsDemo.svelte, loaded apart). -->
		{#await import('./NotificationsDemo.svelte') then { default: NotificationsDemo }}
			<NotificationsDemo />
		{/await}
	</section>

	<section>
		<h2>Mentions</h2>
		<p class="note">
			Type @ in a comment to pick a person (arrows, Enter or Tab, Escape), and /task or /wiki to
			refer to a task or page. Comments show mentions as chips with the person's current name,
			references as links with the current title, and web addresses as links.
		</p>
		<!-- Loaded apart, for the script budget (MentionsDemo.svelte). -->
		{#await import('./MentionsDemo.svelte') then { default: MentionsDemo }}
			<MentionsDemo />
		{/await}
	</section>

	<section>
		<h2>Calendar feeds</h2>
		<p class="note">
			A new feed's link, shown once on Settings, Calendar feeds: copy it, or open it as webcal: in a
			calendar app.
		</p>
		<CalendarFeedLink
			url="https://mila.example.com/ical/q8Jr2vT0bYk9XwZp3LmN7cHs5dFgA1eRuWy4iOtK6xE.ics"
			name="My tasks"
		/>
	</section>

	<section>
		<h2>Projects</h2>
		<p class="note">
			A project shows its icon (a Lucide icon or an emoji) in its color, or a dot without one. The
			project page's button picks both; <HelpKeys
				id="sidebar.move"
				trail=" moves"
				otherwise="Its arrows move"
			/>
			a project in the sidebar.
		</p>
		<div class="wrap">
			<span class="pair"><ProjectIcon icon="" color="blue" />No icon</span>
			<span class="pair"><ProjectIcon icon="lucide:sprout" color="green" />Lucide icon</span>
			<span class="pair"><ProjectIcon icon="🍳" color="orange" />Emoji</span>
			<span class="pair"><ProjectIcon icon="lucide:rocket" color="purple" size={24} />Larger</span>
		</div>
		<div class="wrap">
			<ProjectStylePicker
				name="Sample project"
				icon={demoProject.icon}
				color={demoProject.color}
				onchange={(f) => (demoProject = { ...demoProject, ...f })}
			/>
			<span class="note">The picker, for a sample project</span>
		</div>
		<h3>SidebarLink</h3>
		<p class="note">
			Inbox, My Day and Upcoming in the sidebar: the current one marked, and in a collapsed sidebar
			only the icon, with the name as a tooltip and for screen readers.
		</p>
		{#await import('./SidebarDemo.svelte') then { default: SidebarDemo }}
			<SidebarDemo />
		{/await}
	</section>

	<section>
		<h2>Preferences</h2>
		<p class="note">
			Settings, Preferences: the sidebar's items in order (arrows, or Alt with up and down), and how
			dates and times read. Here they change only these samples.
		</p>
		<div class="field-wide">
			<SidebarEditor
				order={demoOrder}
				hidden={demoHidden}
				onchange={(c) => {
					if (c.order) demoOrder = c.order;
					if (c.hidden) demoHidden = c.hidden;
				}}
			/>
		</div>
		<DatePreferences value={demoDates} onchange={(c) => (demoDates = { ...demoDates, ...c })} />
		<table class="dates">
			<caption>One date and two times in each style</caption>
			<thead
				><tr><th scope="col">Style</th><th scope="col">Date</th><th scope="col">Times</th></tr
				></thead
			>
			<tbody>
				{#each dateStyles as d (d.name)}
					<tr>
						<th scope="row">{d.name}</th>
						<td>{formatDate('2026-11-03', { weekday: 'short', year: false }, d.style)}</td>
						<td>{formatTime('09:05', d.style)} · {formatTime('14:30', d.style)}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</section>

	<section>
		<h2>Board</h2>
		<p class="note">
			A project's statuses as columns. Drag a card, or focus it and use Alt with the arrow keys.
		</p>
		<!-- Loaded apart, as the app's own project page loads it, to keep this page in budget. -->
		<div class="board-demo">
			{#await import('#lib/components/views/Board.svelte') then { default: Board }}
				<Board ws={SAMPLE_WS} projectId={SAMPLE_PROJECT} />
			{/await}
		</div>
	</section>

	<section>
		<h2>Calendar</h2>
		<p class="note">
			Tasks on their due dates, from the first day of the user's week; repeating ones show where
			they fall next. Phones get the agenda below the grid instead.
		</p>
		<div class="calendar-demo">
			{#await import('#lib/components/views/Calendar.svelte') then { default: Calendar }}
				<Calendar ws={SAMPLE_WS} tasks={sampleTasks2} />
			{/await}
		</div>
	</section>

	<!-- Saved views' filter and display parts, loaded apart so the page
	     stays in its script budget. -->
	{#await import('./ViewsDemo.svelte') then { default: ViewsDemo }}
		<ViewsDemo />
	{/await}

	{#await import('./WikiDemo.svelte') then { default: WikiDemo }}
		<WikiDemo />
	{/await}

	{#await import('./FormattingDemo.svelte') then { default: FormattingDemo }}
		<FormattingDemo />
	{/await}

	{#await import('./DiagramsDemo.svelte') then { default: DiagramsDemo }}
		<DiagramsDemo />
	{/await}

	{#await import('./DrawingsDemo.svelte') then { default: DrawingsDemo }}
		<DrawingsDemo />
	{/await}

	{#await import('./MenusDemo.svelte') then { default: MenusDemo }}
		<MenusDemo />
	{/await}

	<section>
		<h2>Custom fields</h2>
		<p class="note">How each type of field is filled in on a task.</p>
		<dl class="fields-demo">
			{#each sampleFields as f (f.id)}
				<dt>{f.name}</dt>
				<dd>
					<FieldInput
						ws={SAMPLE_WS}
						taskId="t-beds"
						field={f}
						value={sample.fieldValues('t-beds')[f.id]}
					/>
				</dd>
			{/each}
		</dl>
	</section>

	<section>
		<h2>Quick add, palette and shortcuts</h2>
		<p class="note">
			In the app, {keymap.say('quick-add') ?? 'the palette'}, {keymap.say('palette')} and {keymap.say(
				'help'
			) ?? 'the palette'} open these. Quick add reads dates, times, #projects, @people, !priority and
			repeats as you type; the palette goes anywhere and acts on the focused task.
		</p>
		<div class="wrap">
			<Button onclick={() => ui.open('quickadd')}>Open quick add</Button>
			<Button onclick={() => ui.open('palette')}>Open the palette</Button>
			<Button onclick={() => ui.open('help')}>Show the shortcuts</Button>
		</div>
		<!-- Quick add, the palette and the shortcut sheet load apart, as in the app. -->
		{#await import('#lib/components/QuickAdd.svelte') then { default: QuickAdd }}
			<QuickAdd ws={SAMPLE_WS} />
		{/await}
		{#await import('#lib/components/CommandPalette.svelte') then { default: CommandPalette }}
			<CommandPalette ws={SAMPLE_WS} />
		{/await}
		{#await import('#lib/components/ShortcutHelp.svelte') then { default: ShortcutHelp }}
			<ShortcutHelp />
		{/await}
	</section>

	<section>
		<h2>Phone</h2>
		<p class="note">
			The tab bar a phone shows instead of the sidebar; Search opens the palette and Settings is
			behind the gear at the top. On a touch screen a row swipes: right completes it (with an undo),
			left schedules it. The due date, or the calendar button beside the delete one, schedules
			without a gesture.
		</p>
		<div class="phone-demo"><TabBar inline /></div>
		<div class="list" aria-label="Swipe rows" role="list">
			{#each swipeRows as task (task.id)}
				<div role="listitem" out:collapse>
					<TaskItem ws={SAMPLE_WS} {task} status={sample.status(task.statusId)} />
				</div>
			{/each}
		</div>
	</section>

	<section>
		<h2>Schedule</h2>
		<p class="note">
			Today, tomorrow, the start of next week, any date, or none; and My Day. Beside its button on a
			wide screen, a sheet from the bottom on a phone.
		</p>
		<div class="wrap">
			<Button
				onclick={(e: MouseEvent) =>
					scheduler.open(SAMPLE_WS, 't-seeds', e.currentTarget as HTMLElement)}
				>Schedule "Order seeds"</Button
			>
		</div>
		<SchedulePicker me={SAMPLE_ME} />
	</section>

	<section>
		<h2>Dependencies</h2>
		<p class="note">
			A task that waits for one not done carries the Blocked mark, which names what it waits for.
			The task panel lists what a task waits for and what waits for it, and adds one with a search.
		</p>
		<div class="wrap">
			<BlockedMark names={['Order seeds']} />
			<BlockedMark names={['Order seeds', 'Test the soil']} />
		</div>
		{#await import('#lib/components/task/Dependencies.svelte') then { default: Dependencies }}
			{@const beds = sample.task('t-beds')}
			{#if beds}<div class="deps-demo">
					<Dependencies ws={SAMPLE_WS} replica={sample} task={beds} />
				</div>{/if}
		{/await}
	</section>

	<section>
		<h2>Templates</h2>
		<p class="note">
			Settings, Templates: the workspace's templates, and what the chosen one holds, its dates in
			days from its first. Save one from a project's or a task's menu.
		</p>
		{#await import('#lib/components/templates/TemplateManager.svelte') then { default: TemplateManager }}
			<TemplateManager ws={SAMPLE_WS} />
		{/await}
	</section>

	<section>
		<h2>Time</h2>
		<p class="note">
			A task's Time: its estimate, the time logged and what is left, the timer, Log time and every
			entry. Rows and cards show the time logged against the estimate, red past it. The Time page
			sums it by person, project or label, and sets estimates against what finished tasks took; its
			Log time opens the form under it, which logs to the project chosen. Below, a project's Time
			tab: this month's time, its own timer and Log time, which go to the project itself, and every
			entry of both kinds.
		</p>
		{#await import('./TimeDemo.svelte') then { default: TimeDemo }}
			<TimeDemo replica={sample} />
		{/await}
	</section>

	{#await import('./ReportsDemo.svelte') then { default: ReportsDemo }}
		<ReportsDemo />
	{/await}

	<section>
		<h2>Task panel</h2>
		<p class="note">
			Beside the list, resized by its edge (drag it, or focus it and use the arrow keys), or
			expanded to the full page. On a phone it is a sheet that slides up and pulls down to close.
			Its notes take the wiki's editor: select text for the formatting bar, or type / at the start
			of a line for lists, checklists, tables, boxes and images.
		</p>
		<div class="panel-demo" style:--demo-left={`${demoSidebar}px`}>
			<div class="panel-demo-left" id="demo-left">List</div>
			<div class="panel-demo-edge">
				<Splitter
					label="Resize the demo list"
					controls="demo-left"
					value={demoSidebar}
					min={120}
					max={320}
					grows="right"
					onresize={(px) => (demoSidebar = Math.min(320, Math.max(120, px)))}
				/>
			</div>
			<div class="panel-demo-detail">
				{#await import('#lib/components/task/TaskDetail.svelte') then { default: TaskDetail }}
					<TaskDetail
						ws={SAMPLE_WS}
						taskId="t-beds"
						expanded={demoExpanded}
						onexpand={() => (demoExpanded = !demoExpanded)}
						onclose={() => {}}
						onopen={() => {}}
					/>
				{/await}
			</div>
		</div>
	</section>

	<section>
		<h2>Task panel, summary lines</h2>
		<p class="note">
			The taskPanel preference's other layout: the same sections, each a line that says what it
			holds and opens in place. Which are open is remembered on the device.
		</p>
		<div class="panel-summary-demo">
			{#await import('#lib/components/task/TaskDetail.svelte') then { default: TaskDetail }}
				<TaskDetail
					ws={SAMPLE_WS}
					taskId="t-beds"
					layoutAs="summary"
					onclose={() => {}}
					onopen={() => {}}
				/>
			{/await}
		</div>
	</section>

	<!-- Import and export, Admin, the security log and sharing load apart (AccountDemo.svelte). -->
	{#await import('./AccountDemo.svelte') then { default: AccountDemo }}
		<AccountDemo />
	{/await}

	<section>
		<h2>Feedback</h2>
		<div class="wrap">
			<SyncDot state="synced" />
			<SyncDot state="syncing" />
			<SyncDot state="offline" pending={4} />
			<SyncDot state="offline" pending={4} compact />
			<SyncDot state="paused" pending={3} />
			<SyncDot state="paused" pending={3} compact />
		</div>
		<div class="wrap"><Toast message="Moved 3 tasks to Done" onundo={() => {}} /></div>
		<div class="wrap"><Toast message="Could not reach the server." tone="error" /></div>
		<h3>Updates</h3>
		<p class="note">
			Stays until acted on. A new build can be put off; once the server asks for a newer app, sync
			waits with every edit kept, and only Reload is offered.
		</p>
		<div class="wrap">
			<Toast
				message={UPDATE_MESSAGES.deployed}
				action={{ label: 'Reload', run: () => {} }}
				ondismiss={() => {}}
			/>
		</div>
		<div class="wrap">
			<Toast message={UPDATE_MESSAGES.required} action={{ label: 'Reload', run: () => {} }} />
		</div>
		<h3>Announcements</h3>
		<p class="note">
			Moves, filter counts and the connection are said aloud through one polite live region, out of
			sight. The last one said:
			<span class="said">{announcer.message || 'nothing yet'}</span>
		</p>
		<div class="wrap">
			<Button onclick={() => announce('Moved Plan the week to In progress, 1 of 3')}
				>Announce a move</Button
			>
		</div>
	</section>

	<section>
		<h2>Keyboard shortcuts</h2>
		<p class="note">
			Keys come from the command registry (keys/commands.ts) with the person's changes on top, so
			every key shown follows what they chose. A binding shows as on a Mac or not; a sequence is two
			keys in turn.
		</p>
		<div class="wrap">
			<Kbd binding="Mod+k" apple={false} />
			<Kbd binding="Mod+k" apple />
			<Kbd binding="Alt+Shift+ArrowUp" apple={false} />
			<Kbd binding="Alt+Shift+ArrowUp" apple />
			<Kbd binding="g i" />
			<Kbd command="quick-add" />
		</div>
		<div class="wrap">
			<!-- Loaded apart, for the script budget, as Settings, Keyboard loads it. -->
			{#await import('#lib/components/keys/KeyRecorder.svelte') then { default: KeyRecorder }}
				<KeyRecorder
					name="Add a task"
					anywhere
					focus={false}
					onrecord={() => {}}
					oncancel={() => {}}
				/>
			{/await}
		</div>
		{#await import('#lib/components/keys/ShortcutTable.svelte') then { default: ShortcutTable }}
			<ShortcutTable scopes={['anywhere', 'list']} />
		{/await}
	</section>

	<section>
		<h2>Help</h2>
		<p class="note">
			Help topics show keys through Keys, which follows the person's keys and says when one is off
			or missing; quick add's tokens as Token chips; and HelpLink beside a control that needs
			explaining, carrying an open task along.
		</p>
		<div class="wrap">
			<HelpLink topic="recurrence">How repeats work</HelpLink>
			<HelpLink topic="quick-add" section="dates">Dates in quick add</HelpLink>
		</div>
		<div class="wrap">
			<Token kind="project" text="#work" />
			<Token kind="priority" text="!high" />
			<Token kind="person" text="@dana" />
			<Token kind="label" text="+phone" />
			<Token kind="date" text="tomorrow" />
			<Token kind="time" text="3pm" />
			<Token kind="repeat" text="every mon" />
			<Token kind="reminder" text="remind 1h before" />
		</div>
		<HelpExample
			typed="Call Dana tomorrow 3pm #work !high"
			tokens={[
				{ kind: 'project', text: '#work' },
				{ kind: 'date', text: 'tomorrow' },
				{ kind: 'time', text: '3pm' },
				{ kind: 'priority', text: '!high' }
			]}
			result="What comes out, in words."
		/>
		<HelpTip>Something worth remembering, set apart from the text.</HelpTip>
		<HelpShot name="quick-add" alt="A picture of the app, in the scheme showing." eager />
		<div class="wrap">
			<span class="pair">Complete: <HelpKeys id="task.complete" /></span>
			<span class="pair">Next task: <HelpKeys id="task.next" /></span>
			<span class="pair">Open the trash: <HelpKeys id="go.trash" /></span>
		</div>
		<p>
			In a sentence, the keys come with their words, which go when no key works: open quick add with
			New task<HelpKeys id="quick-add" lead=", or press " />; open the trash from the sidebar<HelpKeys
				id="go.trash"
				lead=", or press "
			/>.
		</p>
		{#await import('#lib/components/keys/ShortcutTable.svelte') then { default: ShortcutTable }}
			<ShortcutTable scopes={['board']} query="move" />
		{/await}
	</section>

	<section>
		<h2>API reference</h2>
		<p class="note">
			The parts of the API reference at /api/docs. SchemaView shows a schema: each field with its
			type, whether it is required or read-only, its limits and default, and the objects it holds
			one level down. OperationList is the page's list of endpoints by tag, folded on a phone;
			Operation is one endpoint, its details drawn once opened; TryIt sends its request as the
			person signed in.
		</p>
		{#await import('./ApiDocsDemo.svelte') then { default: ApiDocsDemo }}
			<ApiDocsDemo />
		{/await}
	</section>

	<section>
		<h2>People</h2>
		<p class="note">
			A person's picture where Mila has one, else their initials on a color of their own. Lee's
			picture is not there, so Lee keeps the initials. Settings, Profile takes a picture from a
			file, a drop or a paste, and crops it to a circle; here nothing is sent.
		</p>
		{#await import('./PeopleDemo.svelte') then { default: PeopleDemo }}
			<PeopleDemo />
		{/await}
	</section>

	<section>
		<h2>Automations</h2>
		<p class="note">
			Settings, Automations, on the sample workspace. Nothing here is saved or sent.
		</p>
		{#await import('#lib/components/automations/Automations.svelte') then { default: AutomationsPage }}
			<AutomationsPage ws={SAMPLE_WS} role="owner" me={SAMPLE_ME} sample />
		{/await}
	</section>

	<section>
		<h2>Themes</h2>
		<p class="note">
			A theme of one's own: the picker, each state of a token in the editor (from its base with its
			contrast, changed and failing with Fix, worked out from the accent, the same as another, a
			font and a shadow), the swatch card, the notices of a workspace theme, and the preview of a
			theme kept on this page only.
		</p>
		{#await import('./ThemesDemo.svelte') then { default: ThemesDemo }}
			<ThemesDemo />
		{/await}
	</section>

	<section>
		<h2>Selecting several tasks</h2>
		<p class="note">
			The bar that changes the selected tasks, for two of the sample's: on a computer, floating at
			the bottom of the page; on a phone, the strip at the top and the bar in place of the tabs.
		</p>
		{#await import('#lib/components/SelectionBar.svelte') then { default: SelectionBar }}
			<div class="selection-demo">
				<SelectionBar ws={SAMPLE_WS} phone={false} inline sample={sampleTasks2.slice(0, 2)} />
				<div class="selection-phone">
					<SelectionBar ws={SAMPLE_WS} phone inline sample={sampleTasks2.slice(0, 2)} />
				</div>
			</div>
		{/await}
	</section>

	{#await import('./ListViewsDemo.svelte') then { default: ListViewsDemo }}
		<ListViewsDemo />
	{/await}
</div>

<style>
	.selection-demo {
		display: grid;
		gap: var(--space-5);
	}
	.selection-phone {
		display: grid;
		max-width: 360px;
		border: var(--border-width) solid var(--border);
	}
	.guide {
		max-width: 960px;
		margin: 0 auto;
		padding: 0 var(--space-5) var(--space-7);
	}
	/* The theme cards (the built-ins and Omarchy's) make the controls
	   taller than a screen, so they scroll away with the page rather than
	   cover it. */
	.controls {
		display: grid;
		gap: var(--space-3);
		padding: var(--space-4) 0;
		background: var(--surface-0);
		border-bottom: var(--border-width) solid var(--border);
	}
	h1 {
		font-size: var(--text-2xl);
	}
	section {
		display: grid;
		gap: var(--space-4);
		padding: var(--space-6) 0;
		border-bottom: var(--border-width) solid var(--border);
	}
	h2 {
		font-size: var(--text-xl);
	}
	h3 {
		font-size: var(--text-md);
	}
	.panel-summary-demo {
		max-width: 460px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-md);
		overflow: hidden;
	}
	.deps-demo {
		display: grid;
		gap: var(--space-4);
		max-width: 26rem;
	}
	.panel-demo {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(280px, 380px));
		gap: var(--space-4);
		align-items: start;
	}
	.wrap {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3);
		align-items: center;
	}
	.type {
		display: grid;
		gap: var(--space-2);
	}
	.token {
		display: inline-block;
		width: 3em;
		font-family: var(--font-mono);
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
	.heading-sample {
		font-family: var(--font-heading);
		font-size: var(--text-xl);
	}
	.field {
		width: 260px;
	}
	.pair {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.note {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.said {
		font-weight: 600;
		color: var(--text-primary);
	}
	.list {
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	.toast-slot {
		justify-self: start;
	}
	.field-wide {
		max-width: 420px;
	}
	.dates {
		border-collapse: collapse;
		font-size: var(--text-sm);
	}
	.dates caption {
		margin-bottom: var(--space-2);
		text-align: left;
		color: var(--text-secondary);
	}
	.dates th,
	.dates td {
		padding: var(--space-1) var(--space-3) var(--space-1) 0;
		text-align: left;
	}
	.board-demo {
		height: 360px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	.phone-demo {
		max-width: 412px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	.panel-demo {
		position: relative;
		display: grid;
		grid-template-columns: var(--demo-left) 1fr;
		height: 480px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: hidden;
	}
	.panel-demo-left {
		padding: var(--space-3);
		background: var(--surface-1);
		color: var(--text-secondary);
	}
	.panel-demo-edge {
		position: absolute;
		top: 0;
		bottom: 0;
		left: var(--demo-left);
	}
	.panel-demo-detail {
		min-width: 0;
		min-height: 0;
		border-left: var(--border-width) solid var(--border);
	}
	.calendar-demo {
		height: 560px;
		border: var(--border-width) solid var(--border);
		border-radius: var(--radius-lg);
		overflow: auto;
	}
	.fields-demo {
		display: grid;
		grid-template-columns: max-content minmax(0, 260px);
		gap: var(--space-2) var(--space-4);
		align-items: center;
		margin: 0;
	}
	.fields-demo dt {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.fields-demo dd {
		margin: 0;
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(240px, 280px));
		gap: var(--space-4);
		padding: var(--space-4);
		border-radius: var(--radius-lg);
		background: var(--surface-1);
	}
</style>
