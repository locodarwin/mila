// What the commands that work anywhere do. The window's key handler and
// the command palette both run them from here. Commands on a focused
// row, card or entry are run by their own handlers.
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { engine } from '../sync/engine.svelte';
import { appearance } from '../theme/appearance.svelte';
import { toasts } from '../toasts.svelte';
import { ui } from '../ui.svelte';
import type { CommandId } from './commands';

export const ACTIONS: Partial<Record<CommandId, () => unknown>> = {
	palette: () => ui.open('palette'),
	search: () => ui.open('palette'),
	'quick-add': () => ui.open('quickadd'),
	help: () => ui.open('help'),
	'go.inbox': () => goto(resolve('/(app)')),
	'go.today': () => goto(resolve('/(app)/today')),
	'go.upcoming': () => goto(resolve('/(app)/upcoming')),
	'go.projects': () => goto(resolve('/(app)/projects')),
	'go.wiki': () => goto(resolve('/(app)/wiki')),
	'go.time': () => goto(resolve('/(app)/time')),
	'go.reports': () => goto(resolve('/(app)/reports')),
	'go.help': () => goto(resolve('/(app)/help')),
	'go.trash': () => goto(resolve('/(app)/trash')),
	'go.settings': () => goto(resolve('/(app)/settings')),
	'go.preferences': () => goto(resolve('/(app)/settings/preferences')),
	'go.keyboard': () => goto(resolve('/(app)/settings/keyboard')),
	'scheme.light': () => appearance.set({ scheme: 'light' }),
	'scheme.dark': () => appearance.set({ scheme: 'dark' }),
	'scheme.system': () => appearance.set({ scheme: 'system' }),
	undo: () => engine.undo().then((done) => done && toasts.show('Undone'))
};
