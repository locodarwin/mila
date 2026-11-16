// The pages this tab has just made to fill from a template (newPageForTemplate),
// so only those fill as their address opens: an address with a template in it
// that someone sent would otherwise write that template as whoever opens it.
const made = new Map<string, string>();

export function madeForTemplate(pageId: string, templateId: string) {
	made.set(pageId, templateId);
}

/** Whether this tab made the page to fill from this template, forgetting it. */
export function takeTemplateFill(pageId: string, templateId: string): boolean {
	const wanted = made.get(pageId);
	made.delete(pageId);
	return wanted === templateId;
}
