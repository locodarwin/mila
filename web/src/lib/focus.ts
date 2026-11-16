// focusOnShow puts focus in a field as it appears, for a field shown by a
// click or key (a new project's name, a new card's title). The autofocus
// attribute does not do it there: it works only as a page loads, so the
// button that was pressed kept focus.
export function focusOnShow(node: HTMLElement) {
	requestAnimationFrame(() => {
		if (node.isConnected) node.focus();
	});
}
