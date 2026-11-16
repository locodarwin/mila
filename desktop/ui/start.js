// The app's own page: the Places screen, which lists every place with how it is, and
// adds, opens, renames and removes them (docs/PLAN.md section 3, the desktop app).
const $ = (id) => document.getElementById(id);
const invoke = (command, args) => window.__TAURI_INTERNALS__.invoke(command, args);
const sections = ['home', 'first', 'server', 'local', 'waiting', 'starting'];
let places = [];

function show(which) {
	for (const s of sections) $(s).hidden = s !== which;
}

function fail(message) {
	$('error').textContent = message;
	$('error').hidden = !message;
}

const STATUS = {
	checking: ['checking', 'Checking…'],
	ready: ['ready', 'Ready'],
	stopped: ['stopped', 'Stopped'],
	offline: ['offline', 'Offline'],
	signin: ['signin', 'Sign in again']
};

/** When a place was last opened, in a few words. */
function opened(seconds) {
	if (!seconds) return 'not opened yet';
	const then = new Date(seconds * 1000);
	const day = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
	const days = Math.round((day(new Date()) - day(then)) / 86_400_000);
	if (days <= 0) return 'opened today';
	if (days === 1) return 'opened yesterday';
	const sameYear = then.getFullYear() === new Date().getFullYear();
	return `opened ${then.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: sameYear ? undefined : 'numeric' })}`;
}

function el(tag, props = {}, ...children) {
	const node = document.createElement(tag);
	for (const [k, v] of Object.entries(props)) {
		if (k === 'class') node.className = v;
		else if (k.startsWith('on')) node.addEventListener(k.slice(2), v);
		else if (v !== undefined && v !== null) node.setAttribute(k, v);
	}
	node.append(...children.filter((c) => c !== null && c !== undefined));
	return node;
}

async function start() {
	const view = await invoke('places_list');
	places = view.places;
	$('version').textContent = `Version ${view.version}`;
	$('open-last').checked = view.open_last;
	if (location.hash === '#server') return toServer();
	if (location.hash === '#local') return toLocal();
	if (!places.length) return show('first');
	render();
	show('home');
	// The last place first in focus, so Enter opens it.
	const last = document.querySelector('.place.last .open') || document.querySelector('.place .open');
	last?.focus();
	for (const p of places) check(p);
}

function render() {
	const list = $('list');
	list.replaceChildren(...places.map(row));
}

function row(p) {
	const status = el('span', { class: 'status checking' }, 'Checking…');
	const meta = el('span', { class: 'meta' }, status, el('span', { class: 'when' }, opened(p.opened)));
	const open = el('button', { type: 'button', class: p.last ? 'open primary' : 'open', onclick: () => openPlace(p) }, 'Open');
	const more = el('button', {
		type: 'button',
		class: 'more',
		'aria-label': `More for ${p.title}`,
		title: `More for ${p.title}`,
		'aria-haspopup': 'menu',
		'aria-expanded': 'false',
		onclick: (e) => toggleMenu(e.currentTarget, p)
	}, '⋯');
	const title = el('span', { class: 'title' }, p.title);
	const item = el(
		'li',
		{ class: p.last ? 'place last' : 'place', 'data-id': p.id },
		el('span', { class: p.local ? 'icon local' : 'icon server', 'aria-hidden': 'true' }),
		el(
			'div',
			{ class: 'body' },
			el('span', { class: 'name' }, title, p.address !== p.title ? el('small', {}, p.address) : null, p.last ? el('span', { class: 'tag' }, 'Last opened') : null),
			meta
		),
		el('span', { class: 'acts' }, open, el('span', { class: 'pop' }, more))
	);
	item.status = status;
	item.meta = meta;
	item.openButton = open;
	return item;
}

function rowOf(id) {
	return [...document.querySelectorAll('.place')].find((r) => r.dataset.id === id);
}

async function check(p) {
	let s;
	try {
		s = await invoke('place_status', { id: p.id });
	} catch {
		return;
	}
	p.status = s.status;
	const r = rowOf(p.id);
	if (!r) return;
	const [kind, words] = STATUS[s.status] ?? STATUS.checking;
	r.status.className = `status ${kind}`;
	r.status.textContent = words;
	r.meta.querySelectorAll('.extra').forEach((n) => n.remove());
	const extra = [];
	if (s.who) extra.push(s.who);
	if (s.workspaces != null) extra.push(`${s.workspaces} ${s.workspaces === 1 ? 'workspace' : 'workspaces'}`);
	if (s.status === 'offline') extra.push('works from its saved copy');
	if (s.newer) extra.push('the server is newer than this app');
	r.status.after(...extra.map((t) => el('span', { class: 'extra' }, t)));
	if (s.status === 'signin') r.openButton.textContent = 'Sign in…';
}

async function openPlace(p) {
	fail('');
	closeMenus();
	if (p.status === 'signin') return connect(p.id);
	if (p.local) show('starting');
	try {
		await invoke('open_place', { id: p.id });
	} catch (e) {
		show('home');
		fail(String(e));
	}
}

// A place's ⋯ menu.
function closeMenus() {
	document.querySelectorAll('.pop ul').forEach((m) => m.remove());
	document.querySelectorAll('.more[aria-expanded="true"]').forEach((b) => b.setAttribute('aria-expanded', 'false'));
}

function toggleMenu(button, p) {
	const open = button.getAttribute('aria-expanded') === 'true';
	closeMenus();
	if (open) return;
	const item = (label, act, note, cls) =>
		el('li', { role: 'none' }, el('button', { type: 'button', role: 'menuitem', class: cls, onclick: () => { closeMenus(); act(); } }, label, note ? el('small', {}, note) : null));
	const menu = el(
		'ul',
		{ role: 'menu', 'aria-label': `More for ${p.title}` },
		item('Rename…', () => rename(p)),
		p.local ? item('Show its folder', () => invoke('show_folder', { id: p.id }).catch((e) => fail(String(e))), p.address) : null,
		!p.local ? item('Sign out', () => signOut(p), 'Keeps the place; your work stays on the server') : null,
		item('Remove from this app…', () => askRemove(p), p.local ? 'You choose whether its work stays' : 'Your work stays on the server', 'danger')
	);
	button.after(menu);
	button.setAttribute('aria-expanded', 'true');
	menu.querySelector('[role="menuitem"]').focus();
	menu.addEventListener('keydown', (e) => {
		const items = [...menu.querySelectorAll('[role="menuitem"]')];
		const at = items.indexOf(document.activeElement);
		if (e.key === 'ArrowDown') items[(at + 1) % items.length].focus();
		else if (e.key === 'ArrowUp') items[(at - 1 + items.length) % items.length].focus();
		else if (e.key === 'Escape') {
			closeMenus();
			button.focus();
		} else return;
		e.preventDefault();
	});
}

document.addEventListener('click', (e) => {
	if (!e.target.closest('.pop')) closeMenus();
});

function rename(p) {
	const r = rowOf(p.id);
	const title = r.querySelector('.title');
	const input = el('input', { class: 'rename', value: p.title, maxlength: '60', 'aria-label': `Name for ${p.title}` });
	title.replaceWith(input);
	input.select();
	let done = false;
	const finish = async (save) => {
		if (done) return;
		done = true;
		if (save) {
			try {
				p.title = await invoke('rename_place', { id: p.id, name: input.value });
			} catch (e) {
				fail(String(e));
			}
		}
		title.textContent = p.title;
		input.replaceWith(title);
		r.querySelector('.more').setAttribute('aria-label', `More for ${p.title}`);
		r.querySelector('.more').focus();
	};
	input.addEventListener('keydown', (e) => {
		if (e.key !== 'Enter' && e.key !== 'Escape') return;
		// Or the same key press would go on to press the button focus moves to.
		e.preventDefault();
		finish(e.key === 'Enter');
	});
	input.addEventListener('blur', () => finish(true));
}

async function signOut(p) {
	fail('');
	try {
		await invoke('sign_out_place', { id: p.id });
		check(p);
	} catch (e) {
		fail(String(e));
	}
}

// Removing a place, and for one on this computer, perhaps its work.
let removing = null;

function askRemove(p) {
	removing = p;
	$('remove-title').textContent = `Remove ${p.title} from this app?`;
	$('remove-says').textContent = p.local
		? 'It stops, and leaves the list. Its work stays in its folder unless you choose to delete it.'
		: 'It leaves the list. Your work stays on the server, and you can connect to it again any time.';
	$('remove-local').hidden = !p.local;
	$('remove-delete').checked = false;
	$('remove-confirm').hidden = true;
	$('remove-typed').value = '';
	$('remove-typed-label').textContent = `Type ${p.title} to delete its work for good`;
	$('remove-error').hidden = true;
	$('remove-go').textContent = 'Remove';
	$('remove').showModal();
}

$('remove-delete').addEventListener('change', () => {
	const deleting = $('remove-delete').checked;
	$('remove-confirm').hidden = !deleting;
	$('remove-go').textContent = deleting ? 'Remove and delete its work' : 'Remove';
	if (deleting) $('remove-typed').focus();
});
$('remove-cancel').addEventListener('click', () => $('remove').close());
$('remove-form').addEventListener('submit', async (e) => {
	e.preventDefault();
	const p = removing;
	const remove = $('remove-delete').checked;
	if (remove && $('remove-typed').value.trim() !== p.title) {
		$('remove-error').textContent = `Type ${p.title} exactly, or untick Also delete its work.`;
		$('remove-error').hidden = false;
		$('remove-typed').focus();
		return;
	}
	try {
		await invoke('forget_place', { id: p.id, delete: remove, typed: remove ? $('remove-typed').value : null });
	} catch (err) {
		$('remove-error').textContent = String(err);
		$('remove-error').hidden = false;
		return;
	}
	$('remove').close();
	places = places.filter((x) => x.id !== p.id);
	if (!places.length) show('first');
	else rowOf(p.id)?.remove();
	document.querySelector('.place .open')?.focus();
});

// Adding places.
function toServer() {
	fail('');
	show('server');
	$('address').focus();
}

function toLocal() {
	fail('');
	show('local');
	$('local-name').value = '';
	$('local-name').placeholder = places.some((p) => p.local) ? 'Side project' : 'On this computer';
	$('local-name').focus();
}

function back() {
	fail('');
	history.replaceState(null, '', location.pathname);
	if (places.length) {
		render();
		show('home');
		for (const p of places) check(p);
	} else show('first');
}

async function connect(address) {
	fail('');
	show('waiting');
	try {
		await invoke('connect_server', { address });
	} catch (err) {
		if (String(err) === 'canceled') back();
		else {
			show('server');
			$('address').value = address;
			fail(String(err));
		}
	}
}

async function addLocal(name) {
	fail('');
	show('starting');
	try {
		await invoke('add_local', { name: name || null });
	} catch (e) {
		show(places.length ? 'local' : 'first');
		fail(String(e));
	}
}

$('add-server').addEventListener('click', toServer);
$('first-server').addEventListener('click', toServer);
$('add-local').addEventListener('click', toLocal);
$('first-local').addEventListener('click', () => addLocal(''));
document.querySelectorAll('.back').forEach((b) => b.addEventListener('click', back));
$('server-form').addEventListener('submit', (e) => {
	e.preventDefault();
	connect($('address').value);
});
$('local-form').addEventListener('submit', (e) => {
	e.preventDefault();
	addLocal($('local-name').value.trim());
});
$('cancel').addEventListener('click', () => invoke('cancel_connect'));
// No webview menu (Back, Reload, Inspect) but in fields, for cut, copy and paste,
// as in Mila's own pages (onlyMilaMenus in web/src/lib/desktop.ts).
document.addEventListener('contextmenu', (e) => {
	if (!(e.target instanceof Element) || !e.target.closest('input, textarea')) e.preventDefault();
});
$('open-last').addEventListener('change', () => invoke('set_open_last', { on: $('open-last').checked }).catch((e) => fail(String(e))));

start().catch((e) => fail(String(e)));
