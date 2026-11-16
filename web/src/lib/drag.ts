// Native drag and drop's picture of what is dragged: by default the
// browser shows a faded copy. This gives it the lifted look instead, with a
// shadow and a slight tilt (the .drag-lift rule in base.css), while the
// card left behind shows where it came from.

/** Sets the drag's picture to a lifted copy of the element, held where the pointer took it. */
export function liftDragImage(e: DragEvent, el: HTMLElement) {
	if (!e.dataTransfer?.setDragImage) return;
	const rect = el.getBoundingClientRect();
	const holder = document.createElement('div');
	holder.className = 'drag-lift';
	const copy = el.cloneNode(true) as HTMLElement;
	copy.removeAttribute('id');
	copy.style.width = `${rect.width}px`;
	holder.append(copy);
	document.body.append(holder);
	// The holder's padding leaves room for the shadow around the copy.
	const pad = parseFloat(getComputedStyle(holder).paddingLeft) || 0;
	e.dataTransfer.setDragImage(holder, e.clientX - rect.left + pad, e.clientY - rect.top + pad);
	// The browser takes its picture now; the copy is not needed after.
	requestAnimationFrame(() => holder.remove());
}
