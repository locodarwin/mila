// Who is signed in, and which workspace they are looking at. Loaded by the
// app layout before anything renders.
import { api, ApiError, call, type Role, type User, type Workspace } from './api/client';
import { early, forgetEarly } from './api/early';
import { deviceClock, deviceDateOrder, deviceWeekStart } from './dates';
import { pictures } from './pictures.svelte';
import { preferences } from './preferences.svelte';
import { themes } from './theme/themes.svelte';
import { workspaceThemes } from './theme/workspaceThemes.svelte';

export const WORKSPACE_KEY = 'mila.workspace';
// The last known user and workspaces, so the app opens with no network.
// Forgotten on sign-out along with everything else on the device.
const CACHE_KEY = 'mila.session';

interface Cached {
	user: User;
	passkeys: boolean;
	workspaces: Workspace[];
}

function readCache(): Cached | null {
	try {
		return JSON.parse(localStorage.getItem(CACHE_KEY) ?? 'null');
	} catch {
		return null;
	}
}

function writeCache(c: Cached | null) {
	try {
		if (c) localStorage.setItem(CACHE_KEY, JSON.stringify(c));
		else localStorage.removeItem(CACHE_KEY);
	} catch {
		// the app still works online
	}
}

// What this device last told the server about its time zone and how it
// writes dates, so it is said again only when it changes.
const REPORTED_KEY = 'mila.reported';

function readReported(): string | null {
	try {
		return localStorage.getItem(REPORTED_KEY);
	} catch {
		return null;
	}
}

function writeReported(v: string | null) {
	try {
		if (v) localStorage.setItem(REPORTED_KEY, v);
		else localStorage.removeItem(REPORTED_KEY);
	} catch {
		// said again next time
	}
}

// The people of each workspace last loaded (members, and those who left),
// so an app opened with no network still knows who is who and which
// picture to ask the browser's cache for. Kept apart from mila.session,
// which loadWorkspaces rewrites whole.
const PEOPLE_KEY = 'mila.people';

/** A member of a workspace, as the app keeps them. */
export interface Member {
	userId: string;
	name: string;
	email: string;
	role?: Role;
	avatar: string | null;
}

/** Someone who worked in a workspace and is no longer a member. */
interface Former {
	userId: string;
	name: string;
}

interface People {
	user: string;
	members: Record<string, Member[]>;
	former: Record<string, Former[]>;
	/** By workspace, the projects each guest is given, as far as this person may know. */
	guests?: Record<string, Record<string, string[]>>;
}

function readPeople(userId: string): People | null {
	try {
		const p = JSON.parse(localStorage.getItem(PEOPLE_KEY) ?? 'null') as People | null;
		return p?.user === userId ? p : null;
	} catch {
		return null;
	}
}

function writePeople(p: People | null) {
	try {
		if (p) localStorage.setItem(PEOPLE_KEY, JSON.stringify(p));
		else localStorage.removeItem(PEOPLE_KEY);
	} catch {
		// loaded again next time
	}
}

const offline = (err: unknown) => err instanceof ApiError && err.code === 'network';

function readWorkspace(): string | null {
	try {
		return localStorage.getItem(WORKSPACE_KEY);
	} catch {
		return null;
	}
}

class Session {
	loaded = $state(false);
	setupRequired = $state(false);
	passkeys = $state(true);
	/** The desktop app's mode: one person on this computer, no sign-in, nobody else. */
	local = $state(false);
	user = $state<User | null>(null);
	workspaces = $state<Workspace[]>([]);
	#selected = $state<string | null>(readWorkspace());

	/** The workspace in view: the one last picked, else the personal one. */
	workspace = $derived(
		this.workspaces.find((w) => w.id === this.#selected) ?? this.workspaces[0] ?? null
	);

	async load() {
		try {
			const state = await call(early('/auth/state') ?? api.GET('/auth/state'));
			this.setupRequired = state.setupRequired;
			this.passkeys = state.passkeys;
			this.local = state.local ?? false;
			this.user = state.user ?? null;
			if (!this.user) {
				writeCache(null);
				writePeople(null);
				forgetEarly();
			} else {
				preferences.connect(this.user.id, this.user.preferences);
				themes.connect(this.user.id);
				workspaceThemes.connect(this.user.id);
			}
		} catch (err) {
			const cached = readCache();
			if (!offline(err) || !cached) throw err;
			this.setupRequired = false;
			this.passkeys = cached.passkeys;
			this.user = cached.user;
			this.workspaces = cached.workspaces;
			// This device's copy of the preferences is at least as new as the cache's.
			preferences.connect(cached.user.id, null);
			themes.connect(cached.user.id);
			workspaceThemes.connect(cached.user.id);
		}
		if (this.user) {
			const people = readPeople(this.user.id);
			if (people) {
				this.#members = people.members;
				this.#former = people.former;
				this.#guests = people.guests ?? {};
			}
		}
		this.loaded = true;
	}

	/** Takes the user's record as the server now has it, and keeps it on the device. */
	setUser(u: User) {
		this.user = u;
		writeCache({ user: u, passkeys: this.passkeys, workspaces: this.workspaces });
	}

	async loadWorkspaces() {
		try {
			this.workspaces = await call(early('/workspaces') ?? api.GET('/workspaces'));
		} catch (err) {
			const cached = readCache();
			if (!offline(err) || !cached) throw err;
			this.workspaces = cached.workspaces;
			this.#prunePeople();
			return;
		}
		if (this.user)
			writeCache({ user: this.user, passkeys: this.passkeys, workspaces: this.workspaces });
		this.#prunePeople();
	}

	select(id: string) {
		this.#selected = id;
		try {
			localStorage.setItem(WORKSPACE_KEY, id);
		} catch {
			// remembered for this visit only
		}
	}

	#members = $state<Record<string, Member[]>>({});
	/** By workspace, the projects each guest is given, for who can be mentioned where. */
	#guests = $state<Record<string, Record<string, string[]>>>({});

	/** Workspaces whose people were loaded since the app opened and are not stale. */
	#loaded = new Set<string>();

	/**
	 * Who belongs to a workspace, for assigning and for naming comment
	 * authors. Until loadMembers has run, and for personal workspaces, that
	 * is just the user. A guest gets names and pictures with no addresses,
	 * and only the other guests they share a project with.
	 */
	members(ws: string): Member[] {
		const me = this.user
			? [
					{
						userId: this.user.id,
						name: this.user.name,
						email: this.user.email,
						// A record cached before pictures has none.
						avatar: this.user.avatar ?? null
					}
				]
			: [];
		return this.#members[ws] ?? me;
	}

	/**
	 * Who can be told about a task in a project: everyone but the user who can
	 * reach it, which is the members, and the guests it was shared with. A task
	 * in no project reaches no guest. The server checks again before telling anyone.
	 */
	mentionable(ws: string, projectId: string | null): Member[] {
		return this.reachable(ws, projectId).filter((m) => m.userId !== this.user?.id);
	}

	/** Everyone who can reach a task in a project, the user included: who it can be given to. */
	reachable(ws: string, projectId: string | null): Member[] {
		const guests = this.#guests[ws] ?? {};
		return this.members(ws).filter(
			(m) => m.role !== 'guest' || (!!projectId && !!guests[m.userId]?.includes(projectId))
		);
	}

	/**
	 * Whether someone is known to be a guest who cannot reach a project (or
	 * the inbox, null). Unlike reachable, it says no when the lists have not
	 * come yet: for copying an assignment that is already there.
	 */
	outOfReach(ws: string, projectId: string | null, userId: string): boolean {
		const guests = this.#guests[ws];
		if (!guests || this.members(ws).find((m) => m.userId === userId)?.role !== 'guest')
			return false;
		return !projectId || !guests[userId]?.includes(projectId);
	}

	/** People who worked in a workspace and left it, or deleted their account. */
	#former = $state<Record<string, Former[]>>({});

	/**
	 * Loads a shared workspace's members, and the people who have left it,
	 * so their comments keep a name: once, and again after refreshPeople
	 * marked them stale. The lists are replaced in place, never emptied
	 * first, so names never flash to "Someone". Needs a connection.
	 */
	loadMembers(ws: string) {
		if (!this.#seesPeople(ws)) {
			this.#forgetPeople(ws);
			return;
		}
		if (this.#loaded.has(ws)) return;
		this.#loaded.add(ws);
		const path = { params: { path: { workspaceId: ws } } };
		if (this.#seesMembers(ws)) {
			call(api.GET('/workspaces/{workspaceId}/members', path))
				.then((list) => {
					// Not if they became a guest meanwhile.
					if (!this.#seesMembers(ws)) return;
					this.#members[ws] = list;
					this.#keepPeople();
				})
				.catch(() => this.#loaded.delete(ws));
		}
		call(api.GET('/workspaces/{workspaceId}/people', path))
			.then((list) => {
				if (!this.#seesPeople(ws)) return;
				this.#guests[ws] = Object.fromEntries(
					list.filter((p) => p.guest).map((p) => [p.userId, p.projects])
				);
				if (!this.#seesMembers(ws)) {
					this.#members[ws] = list.map((p) => ({
						userId: p.userId,
						name: p.name,
						email: '',
						role: p.guest ? 'guest' : 'member',
						avatar: p.avatar ?? null
					}));
				}
				this.#keepPeople();
			})
			.catch(() => this.#loaded.delete(ws));
		call(api.GET('/workspaces/{workspaceId}/former-members', path))
			.then((list) => {
				if (!this.#seesPeople(ws)) return;
				this.#former[ws] = list.map((m) => ({ userId: m.userId, name: m.name }));
				this.#keepPeople();
			})
			.catch(() => this.#loaded.delete(ws));
	}

	/** Whether the user may list a workspace's members, addresses included: a shared one they are more than a guest in. */
	#seesMembers(ws: string): boolean {
		const w = this.workspaces.find((x) => x.id === ws);
		return !!w && !w.isPersonal && w.role !== 'guest';
	}

	/** Whether a workspace has other people the user may know of: any shared one they are in. */
	#seesPeople(ws: string): boolean {
		const w = this.workspaces.find((x) => x.id === ws);
		return !!w && !w.isPersonal;
	}

	/** Forgets a workspace's people, here and on the device: they left it. */
	#forgetPeople(ws: string) {
		this.#loaded.delete(ws);
		if (!(ws in this.#members) && !(ws in this.#former) && !(ws in this.#guests)) return;
		delete this.#members[ws];
		delete this.#former[ws];
		delete this.#guests[ws];
		this.#keepPeople();
	}

	/**
	 * Forgets the people of every workspace the user left. Someone who became a
	 * guest keeps the names they may still know, and no addresses or other
	 * guests, until their people load again.
	 */
	#prunePeople() {
		for (const ws of new Set([...Object.keys(this.#members), ...Object.keys(this.#former)])) {
			if (!this.#seesPeople(ws)) {
				this.#forgetPeople(ws);
			} else if (!this.#seesMembers(ws) && this.#members[ws]?.some((m) => m.email)) {
				this.#loaded.delete(ws);
				delete this.#guests[ws];
				this.#members[ws] = this.#members
					[ws]!.filter((m) => m.role !== 'guest')
					.map((m) => ({
						...m,
						email: ''
					}));
				this.#keepPeople();
			}
		}
	}

	#keepPeople() {
		if (this.user)
			writePeople({
				user: this.user.id,
				members: $state.snapshot(this.#members),
				former: $state.snapshot(this.#former),
				guests: $state.snapshot(this.#guests)
			});
	}

	#peopleWait: Promise<void> | null = null;

	/**
	 * Loads people afresh after someone changed their name or picture, or
	 * joined or left: the user's own record, the workspace in view's people
	 * now, and every other workspace's when it is next shown. Calls within
	 * a quarter of a second make one load, so a burst of nudges costs one.
	 * Fails quietly offline.
	 */
	refreshPeople(): Promise<void> {
		this.#peopleWait ??= new Promise((done) =>
			setTimeout(() => {
				this.#peopleWait = null;
				void this.#refreshPeople().finally(done);
			}, 250)
		);
		return this.#peopleWait;
	}

	async #refreshPeople() {
		if (!this.user) return;
		try {
			this.setUser(await call(api.GET('/me')));
		} catch {
			return;
		}
		this.#loaded.clear();
		if (this.workspace) this.loadMembers(this.workspace.id);
		pictures.retry();
	}

	/** A person's name and picture in a workspace, as nameOf names them. */
	personOf(ws: string, userId: string): { name: string; avatar: string | null } {
		if (this.user && userId === this.user.id)
			return { name: this.user.name, avatar: this.user.avatar ?? null };
		const m = this.#members[ws]?.find((x) => x.userId === userId);
		if (m) return { name: m.name, avatar: m.avatar ?? null };
		return { name: this.nameOf(ws, userId), avatar: null };
	}

	/** A name to show: a member's, someone who left ("Deleted user" for a deleted account), else "Someone". */
	nameOf(ws: string, userId: string): string {
		return (
			this.memberName(ws, userId) ??
			this.#former[ws]?.find((m) => m.userId === userId)?.name ??
			'Someone'
		);
	}

	/** A member's name now, or undefined for someone not (or no longer) known. */
	memberName(ws: string, userId: string): string | undefined {
		return this.members(ws).find((m) => m.userId === userId)?.name;
	}

	/**
	 * Tells the server this device's time zone when it differs, so due
	 * notices and the digest arrive at local times, and how the device
	 * writes dates and times and starts its week, which emails follow for
	 * "auto" preferences. Fails quietly offline.
	 */
	syncTimezone() {
		let zone: string;
		try {
			zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
		} catch {
			return;
		}
		const user = this.user;
		if (!zone || !user) return;
		const body = {
			timezone: zone,
			clock: deviceClock(),
			dateOrder: deviceDateOrder(),
			weekStart: deviceWeekStart()
		};
		const said = `${user.id} ${body.timezone} ${body.clock} ${body.dateOrder} ${body.weekStart}`;
		if (user.timezone === zone && readReported() === said) return;
		call(api.PUT('/me/timezone', { body }))
			.then((u) => {
				this.user = u;
				writeReported(said);
			})
			.catch(() => {});
	}

	/** Forgets the user on this device. */
	forget() {
		this.user = null;
		this.workspaces = [];
		this.#members = {};
		this.#former = {};
		this.#guests = {};
		this.#loaded.clear();
		writeCache(null);
		writePeople(null);
		writeReported(null);
		preferences.disconnect();
		themes.disconnect();
	}

	signedIn(user: User) {
		this.user = user;
		this.setupRequired = false;
		this.loaded = true;
		// Answers asked for before signing in say nobody is.
		forgetEarly();
	}

	async signOut() {
		await call(api.POST('/auth/sign-out'));
		this.user = null;
		this.workspaces = [];
		// M2 keeps workspace data on the device; signing out will wipe it here.
	}
}

export const session = new Session();
