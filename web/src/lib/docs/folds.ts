// Opening the foldable sections around something in a page, so that what a link,
// a comment or the caret goes to is there to see (docs/PLAN.md section 3). Each
// fold's node (nodeviews.ts) listens for the event. Small, so the wiki loads it
// without the editor.

/** The event a fold's node opens on. */
export const OPEN_FOLD = 'mila-open-fold';

/** Opens every closed fold that holds an element. */
export function revealInFolds(el: Element | null) {
	for (let at = el?.parentElement?.closest('.fold'); at; at = at.parentElement?.closest('.fold'))
		if (!at.classList.contains('open')) at.dispatchEvent(new CustomEvent(OPEN_FOLD));
}
