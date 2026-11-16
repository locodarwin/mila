// Copying text. The clipboard API needs a secure context (https or
// localhost); over plain http, as a server at home may be reached, the
// text is selected and copied the old way instead.

/**
 * Copies text. `field` is an input or text area showing it, which the
 * old way selects (and which stays selected if copying fails, so the
 * person can copy it by hand); without one, a hidden text area stands in.
 * Returns whether it worked.
 */
export async function copyText(
	text: string,
	field?: HTMLInputElement | HTMLTextAreaElement | null
): Promise<boolean> {
	try {
		if (navigator.clipboard && window.isSecureContext) {
			await navigator.clipboard.writeText(text);
			return true;
		}
	} catch {
		// refused (no permission, the page not focused): try the old way
	}
	const own = field ?? hiddenArea(text);
	try {
		own.select();
		return document.execCommand('copy');
	} catch {
		return false;
	} finally {
		if (!field) own.remove();
	}
}

function hiddenArea(text: string): HTMLTextAreaElement {
	const area = document.createElement('textarea');
	area.value = text;
	area.setAttribute('readonly', '');
	area.setAttribute('aria-hidden', 'true');
	area.style.cssText = 'position:fixed;top:0;left:0;width:1px;height:1px;opacity:0;';
	document.body.append(area);
	return area;
}
