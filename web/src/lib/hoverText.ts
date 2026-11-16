// Everything that can be clicked says what it is when hovered. A control with
// its own title keeps it, and that is the place for words that explain more
// than its name. Any other gets its name, put there as the pointer reaches it,
// so a screen reader never hears the name twice.

const CLICKABLE = [
	'a[href]',
	'button',
	'summary',
	'select',
	'label',
	'input',
	'[data-task-id]',
	...['button', 'tab', 'menuitem', 'menuitemradio', 'menuitemcheckbox', 'option', 'treeitem'].map(
		(role) => `[role="${role}"]`
	)
].join(',');

// Fields are for typing in, and say so themselves.
const TYPED = ['text', 'search', 'email', 'url', 'password', 'tel'];

const LONGEST = 200;

function words(text: string | null | undefined): string {
	const said = (text ?? '').replace(/\s+/g, ' ').trim();
	return said.length > LONGEST ? `${said.slice(0, LONGEST - 1)}…` : said;
}

/** What a control is called: as a screen reader would have it, or its words. */
export function nameOf(el: Element): string {
	const labelled = el.getAttribute('aria-labelledby');
	const named =
		el.getAttribute('aria-label') ??
		(labelled &&
			labelled
				.split(/\s+/)
				.map((id) => document.getElementById(id)?.textContent)
				.join(' '));
	if (named) return words(named);
	const labels = (el as HTMLInputElement).labels;
	if (labels?.length) return words(labels[0]!.textContent);
	return words(el.textContent);
}

function hovered(e: PointerEvent) {
	if (e.pointerType === 'touch' || !(e.target instanceof Element)) return;
	// The editor redraws what changes inside it, even under a pointer about to click.
	if (e.target.closest('[contenteditable]')) return;
	const el = e.target.closest(CLICKABLE);
	if (!el || el.closest('[inert]')) return;
	if (el instanceof HTMLInputElement && TYPED.includes(el.type)) return;
	// Something inside it may say more about itself.
	const titled = e.target.closest('[title]');
	if (titled && el.contains(titled) && titled !== el) return;
	// Ours can be redone, as the name changes; a title of its own stays.
	const ours = el.getAttribute('data-hover-text');
	const title = el.getAttribute('title');
	if (title && title !== ours) return;
	const name = nameOf(el);
	if (name) {
		el.setAttribute('title', name);
		el.setAttribute('data-hover-text', name);
	}
}

/** Starts it for the whole page, returning how to stop. */
export function watchHover(): () => void {
	document.addEventListener('pointerover', hovered, { passive: true });
	return () => document.removeEventListener('pointerover', hovered);
}
