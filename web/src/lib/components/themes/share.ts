// Sharing a theme: as a file to download, or as text on the clipboard.
import { copyText } from '../../clipboard';
import { exportFileName, exportTheme, type ThemeDoc } from '../../theme/document';
import { toasts } from '../../toasts.svelte';

/** Downloads a theme as <name>.mila-theme.json, from a Blob URL. */
export function download(theme: ThemeDoc) {
	const url = URL.createObjectURL(new Blob([exportTheme(theme)], { type: 'application/json' }));
	const a = document.createElement('a');
	a.href = url;
	a.download = exportFileName(theme);
	document.body.append(a);
	a.click();
	a.remove();
	setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** Copies a theme as text, and says whether that worked. */
export async function copy(theme: ThemeDoc) {
	if (await copyText(exportTheme(theme))) toasts.show(`Copied ${theme.name} as text`);
	else toasts.error('Copying did not work here. Export the file instead.');
}
