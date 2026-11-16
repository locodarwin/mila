<script lang="ts">
	// The style guide's sections for Settings, Import and export and Admin, the
	// security log and sharing, from made-up data. They load apart, which
	// keeps the style guide's first load within its budget.
	import type { Schemas } from '#lib/api/client.js';
	import Button from '#lib/components/Button.svelte';
	import AsanaSteps from '#lib/components/data/AsanaSteps.svelte';
	import ClickupSteps from '#lib/components/data/ClickupSteps.svelte';
	import CsvMapping from '#lib/components/data/CsvMapping.svelte';
	import GithubSteps from '#lib/components/data/GithubSteps.svelte';
	import TodoistSteps from '#lib/components/data/TodoistSteps.svelte';
	import ImportDone from '#lib/components/data/ImportDone.svelte';
	import ImportPlan from '#lib/components/data/ImportPlan.svelte';
	import DeleteAccount from '#lib/components/data/DeleteAccount.svelte';
	import ExportList from '#lib/components/data/ExportList.svelte';
	import ImportList from '#lib/components/data/ImportList.svelte';
	import AdminStatus from '#lib/components/admin/AdminStatus.svelte';
	import MicrosoftMailbox from '#lib/components/admin/MicrosoftMailbox.svelte';
	import InviteList from '#lib/components/admin/InviteList.svelte';
	import UserList from '#lib/components/admin/UserList.svelte';
	import SecurityLog from '#lib/components/SecurityLog.svelte';
	import EmailAddressCard from '#lib/components/email/EmailAddressCard.svelte';
	import RecentMail from '#lib/components/email/RecentMail.svelte';
	import ShownOnce from '#lib/components/api/ShownOnce.svelte';
	import TokenList from '#lib/components/api/TokenList.svelte';
	import WebhookItem from '#lib/components/api/WebhookItem.svelte';
	import ShareProject from '#lib/components/ShareProject.svelte';
	import { parseCsv } from '#lib/csv.js';
	import { ASANA } from '#lib/asanaimport.js';
	import { CLICKUP } from '#lib/clickupimport.js';
	import { TODOIST, readTodoist } from '#lib/todoistimport.js';
	import { guessTargets, plan as csvPlan, summaryLines, type Target } from '#lib/csvimport.js';
	import { planIssues, readIssues } from '#lib/githubimport.js';
	import type { CustomField } from '#lib/sync/entities.js';
	import sampleAvatar from './sample-avatar.jpg';

	const ago = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString();

	// Import and export: lists as they look with an export or import in each state,
	// and a CSV file's columns and dry run.
	const inAWeek = new Date(Date.now() + 7 * 864e5).toISOString();
	// Admin, the security log and sharing, from made-up data.
	const hoursAgo = (h: number) => new Date(Date.now() - h * 3600_000).toISOString();
	const demoBackups: Schemas['Backup'][] = [
		{ name: 'b3', at: hoursAgo(5), kind: 'nightly', schema: 14, size: 2_400_000 },
		{ name: 'b2', at: hoursAgo(29), kind: 'manual', schema: 14, size: 2_300_000 },
		{ name: 'b1', at: hoursAgo(53), kind: 'premigrate', schema: 13, size: 2_200_000 }
	];
	const demoStatus: Schemas['AdminStatus'] = {
		version: '0.6.0',
		commit: 'fb0afa0',
		dialect: 'sqlite',
		schema: 14,
		latestSchema: 14,
		migratedAt: hoursAgo(53),
		emailMode: 'smtp',
		passkeys: true,
		baseUrl: 'https://mila.example.com',
		users: 3,
		backup: {
			nightly: true,
			hour: 3,
			dir: '/var/lib/mila/backups',
			lastOkAt: hoursAgo(5),
			recent: demoBackups
		}
	};
	const demoMailboxes: Schemas['MicrosoftMailbox'][] = [
		{ state: 'not-connected', mailbox: 'tasks@contoso.com' },
		{
			state: 'signing-in',
			mailbox: 'tasks@contoso.com',
			userCode: 'K7PQ2XMF',
			verificationUri: 'https://microsoft.com/devicelogin',
			expiresAt: '2026-11-03T18:15:00Z'
		},
		{
			state: 'connected',
			mailbox: 'tasks@contoso.com',
			account: 'shawn@contoso.com',
			connectedAt: '2026-11-01T18:02:00Z',
			connectedBy: 'Shawn'
		},
		{
			state: 'lost',
			mailbox: 'tasks@contoso.com',
			account: 'shawn@contoso.com',
			lostAt: '2026-11-02T09:00:00Z'
		},
		{
			state: 'lost',
			mailbox: 'tasks@contoso.com',
			account: 'shawn@contoso.com',
			lostAt: '2026-11-02T09:00:00Z',
			keyMissing: true
		}
	];
	const demoFailing: Schemas['AdminStatus'] = {
		...demoStatus,
		dialect: 'postgres',
		emailMode: 'log',
		update: {
			version: 'v0.7.0',
			url: 'https://github.com/locodarwin/mila/releases/tag/v0.7.0',
			publishedAt: new Date(2026, 9, 30).toISOString()
		},
		backup: {
			...demoStatus.backup!,
			lastErrorAt: hoursAgo(1),
			lastError: 'write /var/lib/mila/backups: no space left on device'
		}
	};
	let demoBackingUp = $state(false);
	let demoUsers = $state<Schemas['AdminUser'][]>([
		{
			id: 'u1',
			name: 'Shawn',
			email: 'shawn@example.com',
			avatar: null,
			isInstanceAdmin: true,
			createdAt: hoursAgo(2000),
			devices: 2,
			lastActiveAt: hoursAgo(0.1)
		},
		{
			id: 'u2',
			name: 'Dana',
			email: 'dana@example.com',
			avatar: sampleAvatar,
			isInstanceAdmin: false,
			createdAt: hoursAgo(400),
			devices: 1,
			lastActiveAt: hoursAgo(30)
		},
		{
			id: 'u3',
			name: 'Gus',
			email: 'gus@example.com',
			avatar: null,
			isInstanceAdmin: false,
			createdAt: hoursAgo(90),
			devices: 0
		}
	]);
	let demoInvites = $state<Schemas['AdminInvite'][]>([
		{
			id: 'i1',
			workspaceId: 'w',
			workspaceName: 'Garden club',
			email: 'eli@example.com',
			role: 'guest',
			invitedByName: 'Dana',
			createdAt: hoursAgo(20),
			expiresAt: hoursAgo(-148)
		}
	]);
	const chrome =
		'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0 Safari/537.36';
	const firefox = 'Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0';
	let demoTokens = $state<Schemas['ApiToken'][]>([
		{
			id: 't1',
			name: 'Backup script',
			canWrite: false,
			createdAt: '2026-10-01T10:00:00Z',
			lastUsedAt: '2026-11-03T12:00:00Z',
			expiresAt: '2027-01-30T10:00:00Z'
		},
		{
			id: 't2',
			name: 'Zapier',
			canWrite: true,
			workspaceId: 'w1',
			createdAt: '2026-09-01T10:00:00Z'
		}
	]);
	const demoHook: Schemas['Webhook'] = {
		id: 'h1',
		url: 'https://hooks.example.com/mila',
		events: ['task', 'comment'],
		projectId: 'p1',
		createdAt: '2026-10-01T10:00:00Z',
		state: 'failing'
	};
	const demoDeliveries: Schemas['WebhookDelivery'][] = [
		{
			id: 'd1',
			event: 'task.completed',
			summary: 'Plant the garlic',
			status: 'delivered',
			attempts: 1,
			responseCode: 200,
			responseBody: 'ok',
			createdAt: '2026-11-03T14:00:00Z'
		},
		{
			id: 'd2',
			event: 'comment.added',
			summary: '',
			status: 'failed',
			attempts: 6,
			responseCode: 503,
			responseBody: 'Service unavailable',
			createdAt: '2026-11-02T14:00:00Z'
		}
	];
	const demoEvents: Schemas['SecurityEvent'][] = [
		['session_revoked', 'self', false, chrome, 0.2],
		['sign_in', 'magic_link', true, '', 1],
		['passkey_added', '', false, 'Firefox on Linux', 30],
		['invite_accepted', '', false, 'Garden club', 50],
		['admin_granted', 'admin', false, 'Dana', 70],
		['sign_in', 'passkey', false, '', 90]
	].map(([kind, method, newDevice, detail, h], i) => ({
		id: `s${i}`,
		kind: kind as Schemas['SecurityEventKind'],
		method: method as string,
		newDevice: newDevice as boolean,
		detail: detail as string,
		ip: i % 2 ? '198.51.100.7' : '203.0.113.5',
		userAgent: i === 1 ? chrome : firefox,
		createdAt: hoursAgo(h as number)
	}));
	let demoSharing = $state(false);
	let demoGuests = $state<Schemas['ProjectGuest'][]>([
		{ userId: 'g1', name: 'Gus', email: 'gus@example.com', avatar: sampleAvatar, shared: true },
		{ userId: 'g2', name: 'Eli', email: 'eli@example.com', avatar: null, shared: false }
	]);

	let demoExports = $state<Schemas['Export'][]>([
		{
			id: 'e1',
			workspaceId: 'w',
			workspaceName: 'Home',
			format: 'json',
			status: 'ready',
			fileName: 'mila-home-json-2026-10-29.zip',
			size: 2_400_000,
			createdAt: ago(30),
			finishedAt: ago(29),
			expiresAt: inAWeek
		},
		{
			id: 'e2',
			workspaceId: 'w',
			workspaceName: 'Home',
			format: 'markdown',
			status: 'running',
			fileName: 'mila-home-markdown-2026-10-29.zip',
			size: 0,
			createdAt: ago(1),
			expiresAt: inAWeek
		},
		{
			id: 'e3',
			workspaceId: 'w2',
			workspaceName: 'Garden club',
			format: 'csv',
			status: 'failed',
			fileName: 'mila-garden-club-csv-2026-10-28.zip',
			size: 0,
			error: 'No such workspace.',
			createdAt: ago(900),
			finishedAt: ago(899),
			expiresAt: inAWeek
		}
	]);
	const demoImports: Schemas['Import'][] = [
		{
			id: 'i1',
			fileName: 'mila-home-json-2026-10-01.zip',
			keepIds: false,
			status: 'ready',
			workspaceId: 'w3',
			workspaceName: 'Home',
			createdAt: ago(60),
			finishedAt: ago(58),
			report: {
				restored: { task: 132, project: 6, comment: 40, status: 30, label: 8 },
				dropped: { task_assignee: 3 },
				credited: 12,
				activity: 410,
				files: 9,
				missingFiles: 0
			}
		},
		{
			id: 'i2',
			fileName: 'notes.zip',
			keepIds: true,
			status: 'failed',
			error: 'This zip is not a Mila JSON export (it has no Mila manifest.json).',
			createdAt: ago(2000),
			finishedAt: ago(2000)
		}
	];
	const demoFields: CustomField[] = [
		{
			id: 'f-size',
			projectId: null,
			name: 'Size',
			type: 'select',
			options: [{ id: 'o-s', name: 'Small', color: 'green' }],
			sortKey: 'a0'
		}
	];
	const demoCsv = parseCsv(
		'Task,Due date,Tags,Size,Owner\nRepaint the porch,2026-11-08,"house, weekend",Large,Dana\nCall the plumber,next week,house,Small,\n,2026-11-02,,,\n'
	);
	let demoTargets = $state<Target[]>(guessTargets(demoCsv[0], demoFields));
	const demoPlan = $derived(
		csvPlan(
			demoCsv,
			demoTargets,
			{ kind: 'new', name: 'Imported' },
			{
				projects: [],
				statuses: () => [],
				labels: [{ id: 'l-house', name: 'house' }],
				fields: demoFields,
				members: [{ userId: 'u-dana', name: 'Dana', email: 'dana@example.com' }],
				reaches: () => true
			}
		)
	);
	// A file of issues as gh writes it: one open, one closed with a comment,
	// assigned to someone here and someone who isn't.
	const demoIssues = readIssues(
		JSON.stringify([
			{
				number: 12,
				title: 'Gate sticks in wet weather',
				body: 'Plane the bottom edge.',
				state: 'OPEN',
				closedAt: null,
				createdAt: '2026-10-20T09:00:00Z',
				labels: [{ name: 'house' }, { name: 'bug' }],
				assignees: [{ login: 'dana' }],
				milestone: { title: 'Autumn' },
				comments: [],
				url: 'https://github.com/garden-club/shed/issues/12'
			},
			{
				number: 9,
				title: 'Order compost bins',
				body: '',
				state: 'CLOSED',
				closedAt: '2026-10-18T16:00:00Z',
				createdAt: '2026-10-02T09:00:00Z',
				labels: [],
				assignees: [{ login: 'sam-gh' }],
				milestone: null,
				comments: [
					{ author: { login: 'sam-gh' }, createdAt: '2026-10-18T15:00:00Z', body: 'Ordered two.' }
				],
				url: 'https://github.com/garden-club/shed/issues/9'
			},
			'not an issue'
		])
	);
	const demoIssuePlan = planIssues(
		demoIssues,
		{ kind: 'new', name: 'shed' },
		{
			projects: [],
			statuses: () => [],
			labels: [{ id: 'l-house', name: 'house' }],
			fields: [],
			members: [{ userId: 'u-dana', name: 'Dana', email: 'dana@example.com' }],
			reaches: () => true
		}
	);
	// A file as ClickUp writes it: a task with a subtask, comments as JSON,
	// a Lead (left out) and someone who isn't here.
	const demoClickup = parseCsv(
		[
			'Task ID,Task Link,Task Type,Task Name,Task Content,Status,Date Created,Due Date,Parent ID,Assignees,Priority,List Name,Space Name,Comments',
			'86d0001,https://app.clickup.com/t/86d0001,Task,Paint the shed,"Two coats.\\nGreen.",in progress,1790000000000,1790500000000,null,[Dana],2,Garden,Home,"[{""text"":""Bought the paint."",""by"":""dana@example.com"",""date"":""10/2/2026, 9:00:00 AM PDT""}]"',
			'86d0002,https://app.clickup.com/t/86d0002,Task,Sand the boards,,complete,1790000000000,,86d0001,[Sam Reyes],3,Garden,Home,[]',
			'86d0003,https://app.clickup.com/t/86d0003,Lead,Hardware store,,upcoming,1790000000000,,null,[],null,Garden,Home,[]'
		].join('\n')
	);
	let demoClickupTargets = $state<Target[]>(guessTargets(demoClickup[0], [], CLICKUP));
	const demoClickupPlan = $derived(
		csvPlan(
			demoClickup,
			demoClickupTargets,
			{ kind: 'new', name: 'ClickUp' },
			{
				projects: [],
				statuses: () => [],
				labels: [],
				fields: [],
				members: [{ userId: 'u-dana', name: 'Dana', email: 'dana@example.com' }],
				reaches: () => true
			},
			{ dialect: CLICKUP }
		)
	);
	// A file as Asana writes it: sections, a subtask named by its parent's
	// title, a dependency, and someone who isn't here.
	const demoAsana = parseCsv(
		[
			'Task ID,Created At,Completed At,Name,Section/Column,Assignee Email,Due Date,Tags,Projects,Parent task,Blocked By (Dependencies),Priority',
			'9001,2026-10-01,,Paint the shed,Doing,dana@example.com,2026-11-20,house,Garden,,,High',
			'9002,2026-10-01,2026-10-09,Sand the boards,Doing,sam@example.com,,,,Paint the shed,,Low',
			'9003,2026-10-02,,Hang the lights,Later,,,,Garden,,9001,'
		].join('\n')
	);
	const demoAsanaTargets = guessTargets(demoAsana[0], [], ASANA);
	const demoAsanaPlan = csvPlan(
		demoAsana,
		demoAsanaTargets,
		{ kind: 'new', name: 'Asana' },
		{
			projects: [],
			statuses: () => [],
			labels: [{ id: 'l-house', name: 'house' }],
			fields: [],
			members: [{ userId: 'u-dana', name: 'Dana', email: 'dana@example.com' }],
			reaches: () => true
		},
		{ dialect: ASANA }
	);
	// Two files as Todoist writes them, a project each: a section, a subtask
	// by its indent, a label in a task, a repeat, a comment, a date in words
	// from the day of the export, and one that cannot be read.
	const todoistHead =
		'TYPE,CONTENT,DESCRIPTION,PRIORITY,INDENT,AUTHOR,RESPONSIBLE,DATE,DATE_LANG,DEADLINE';
	const demoTodoist = readTodoist([
		{
			name: 'Garden.csv',
			rows: parseCsv(
				[
					todoistHead,
					'meta,view_style=board,,,,,,,,',
					'section,Doing,,,,,,,,',
					'task,Paint the shed @house,Two coats.,1,1,Dana (1),Dana (1),tomorrow at 9am,en,',
					'task,Sand the boards,,4,2,Dana (1),Sam (2),,,',
					'note,Fine paper first.,,,,Sam (2),,2026-10-02T10:30:00Z,,',
					'section,Later,,,,,,,,',
					'task,Water the beds,,3,1,Dana (1),,every! 2 days,en,'
				].join('\n')
			)
		},
		{
			name: 'Errands.csv',
			rows: parseCsv(
				[todoistHead, 'task,Post the parcel,,2,1,Dana (1),,when it stops raining,en,'].join('\n')
			)
		}
	])!;
	const demoTodoistTargets = guessTargets(demoTodoist.rows[0], [], TODOIST);
	const demoTodoistPlan = csvPlan(
		demoTodoist.rows,
		demoTodoistTargets,
		{ kind: 'new', name: 'Todoist' },
		{
			projects: [],
			statuses: () => [],
			labels: [{ id: 'l-house', name: 'house' }],
			fields: [],
			members: [{ userId: 'u-dana', name: 'Dana', email: 'dana@example.com' }],
			reaches: () => true
		},
		{ dialect: TODOIST, today: new Date(2026, 10, 8), date: () => 'Oct 2, 2026' }
	);
	let demoImported = $state(true);
</script>

<section>
	<h2>Import and export</h2>
	<p class="note">
		Settings, Import and export: exports in each state (a finished one downloads; delete asks twice)
		and imports with their report; then Settings, Profile's Delete your account, which asks for the
		email address.
	</p>
	<ExportList
		items={demoExports}
		onremove={(id) => (demoExports = demoExports.filter((e) => e.id !== id))}
	/>
	<ImportList items={demoImports} />
	<DeleteAccount email="shawn@example.com" ondelete={() => {}} />
	<h3>CSV import</h3>
	<p class="note">
		What each column goes to, guessed from its heading, and the dry run, which follows the choices.
	</p>
	<CsvMapping
		headers={demoCsv[0]}
		samples={demoCsv.slice(1)}
		fields={demoFields}
		bind:targets={demoTargets}
	/>
	<ImportPlan plan={demoPlan} destination="a new project, Imported" />
	<h3>Import from another app</h3>
	<p class="note">
		The steps for getting GitHub's file (the command copies), and the same dry run for a file of
		issues, read by its own reader: an entry that is not an issue, someone who is not here, and what
		the reader adds to the summary. Then what an import says when it is done.
	</p>
	<GithubSteps />
	<ImportPlan
		plan={demoIssuePlan}
		destination="a new project, shed"
		item={(line) => demoIssues.names.get(line) ?? `Entry ${line}`}
		unit={['entry', 'entries']}
	>
		<ul>
			<li>1 open and 1 closed; closed ones come in done</li>
			<li>1 comment, each starting with who wrote it and when</li>
		</ul>
	</ImportPlan>
	<p class="note">
		ClickUp's steps, then the CSV import's column screen with ClickUp's columns matched, and its dry
		run: a Lead left out, a subtask, someone who is not here, and the summary its dialect adds.
	</p>
	<ClickupSteps />
	<CsvMapping
		headers={demoClickup[0]}
		samples={demoClickup.slice(1)}
		fields={[]}
		bind:targets={demoClickupTargets}
	/>
	<ImportPlan plan={demoClickupPlan} destination="a new project, Garden">
		<ul>
			{#each summaryLines(demoClickupPlan, demoClickupTargets, CLICKUP) as line, i (i)}
				<li>{line}</li>
			{/each}
		</ul>
	</ImportPlan>
	<p class="note">
		Asana's steps and its dry run: sections as statuses, a subtask named by its parent's title, a
		dependency, and someone who is not here. Its column screen is the same as ClickUp's.
	</p>
	<AsanaSteps />
	<ImportPlan plan={demoAsanaPlan} destination="a new project, Garden">
		<ul>
			{#each summaryLines(demoAsanaPlan, demoAsanaTargets, ASANA) as line, i (i)}
				<li>{line}</li>
			{/each}
		</ul>
	</ImportPlan>
	<p class="note">
		Todoist's steps and its dry run for two files, a project each, with each row named by its file:
		a section as a status, a subtask by its indent, a repeat, a date in words counted from today,
		one that cannot be read, and someone who is not here.
	</p>
	<TodoistSteps />
	<ImportPlan
		plan={demoTodoistPlan}
		destination="a new project, Garden"
		item={(line) => demoTodoist.where[line] ?? `Row ${line}`}
	>
		<ul>
			{#each summaryLines(demoTodoistPlan, demoTodoistTargets, TODOIST) as line, i (i)}
				<li>{line}</li>
			{/each}
		</ul>
	</ImportPlan>
	{#if demoImported}
		<ImportDone
			ws="style-guide"
			ids={['a', 'b']}
			project={null}
			focus={false}
			trash={async () => {}}
			ontrash={() => (demoImported = false)}
		/>
	{:else}
		<Button onclick={() => (demoImported = true)}>Show the import's end again</Button>
	{/if}
</section>

<section>
	<h2>Admin</h2>
	<p class="note">
		Settings, Admin, for the instance's admins: the instance at a glance with its backups (the
		second as when the last backup failed), every account with its admin switch and sign-out, and
		every open invitation. Signing out and canceling ask twice; the last admin's switch is off.
	</p>
	<AdminStatus
		status={demoStatus}
		busy={demoBackingUp}
		onbackup={() => {
			demoBackingUp = true;
			setTimeout(() => (demoBackingUp = false), 1500);
		}}
	/>
	<AdminStatus status={demoFailing} />
	<p class="note">
		A Microsoft mailbox for email to task, in each of its states: not connected, a code to enter at
		Microsoft, connected, needing to be signed in again, and a sign-in this server can't open. Its
		buttons ask this page's server, which has none.
	</p>
	{#each demoMailboxes as mailbox (`${mailbox.state}${mailbox.keyMissing ? '-key' : ''}`)}
		<MicrosoftMailbox {mailbox} refresh={async () => {}} />
	{/each}
	<UserList
		users={demoUsers}
		me="u1"
		onadmin={(u, on) =>
			(demoUsers = demoUsers.map((x) => (x.id === u.id ? { ...x, isInstanceAdmin: on } : x)))}
		onsignout={(u) =>
			(demoUsers = demoUsers.map((x) => (x.id === u.id ? { ...x, devices: 0 } : x)))}
		onremovepicture={(u) => {
			demoUsers = demoUsers.map((x) => (x.id === u.id ? { ...x, avatar: null } : x));
		}}
	/>
	<InviteList
		invites={demoInvites}
		oncancel={(inv) => (demoInvites = demoInvites.filter((x) => x.id !== inv.id))}
	/>
</section>

<section>
	<h2>Security log</h2>
	<p class="note">
		Settings, Security, Recent activity: sign-ins (a browser new to the account is marked),
		passkeys, devices signed out, invitations accepted and admin rights, with where each came from.
	</p>
	<SecurityLog events={demoEvents} more onmore={() => {}} />
</section>

<section>
	<h2>Email to task</h2>
	<p class="note">
		Settings, Email to task: an address, where its tasks go and who it accepts mail from, and what
		became of the mail lately.
	</p>
	<EmailAddressCard
		address={{
			id: 'addr-1',
			address: 'mila+7fk2mq9xr4vzt3hb8cn6wd5jpa@example.com',
			assignToMe: true,
			senders: ['sam@work.example.org'],
			createdAt: '2026-11-01T09:00:00Z'
		}}
		projects={[{ id: 'p1', name: 'Garden' }]}
		own="sam@example.com"
		onsave={() => {}}
		onreplace={() => {}}
		ondelete={() => {}}
	/>
	<RecentMail
		items={[
			{
				id: 'm1',
				addressId: 'addr-1',
				from: 'sam@work.example.org',
				subject: 'Fwd: Invoice 4521',
				outcome: 'made',
				taskId: 't-beds',
				receivedAt: new Date(Date.now() - 6 * 3_600_000).toISOString()
			},
			{
				id: 'm2',
				addressId: 'addr-1',
				from: 'stranger@example.net',
				subject: 'A great offer',
				outcome: 'refused_sender',
				receivedAt: new Date(Date.now() - 3 * 86_400_000).toISOString()
			}
		]}
		onopen={() => {}}
	/>
	<h2>API tokens and webhooks</h2>
	<p class="note">
		Settings, API tokens and Settings, Webhooks: what each token can do and where, a secret shown
		once, and a webhook with its log.
	</p>
	<TokenList
		tokens={demoTokens}
		workspaceName={() => 'Garden club'}
		onrevoke={(t) => (demoTokens = demoTokens.filter((x) => x.id !== t.id))}
	/>
	<ShownOnce
		title="Your new token, Backup script"
		note="This is the only time Mila shows it, so copy it now."
		value="mila_example-only-not-a-real-token"
		ondone={() => {}}
	/>
	<WebhookItem
		hook={demoHook}
		projectName={() => 'Garden'}
		deliveries={demoDeliveries}
		open
		onopen={() => {}}
		ontest={() => {}}
		onpause={() => {}}
		onrotate={() => {}}
		ondelete={() => {}}
		onresend={() => {}}
	/>
</section>

<section>
	<h2>Sharing</h2>
	<p class="note">
		A project's Share button, for a shared workspace's admins: each guest of the workspace with
		whether they see the project. Members see every project, so only guests are listed.
	</p>
	<div><Button onclick={() => (demoSharing = true)}>Share Garden</Button></div>
	<ShareProject
		bind:open={demoSharing}
		projectName="Garden"
		guests={demoGuests}
		onchange={(g, on) =>
			(demoGuests = demoGuests.map((x) => (x.userId === g.userId ? { ...x, shared: on } : x)))}
	/>
</section>

<style>
	/* As the style guide's own sections. */
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
	.note {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
</style>
