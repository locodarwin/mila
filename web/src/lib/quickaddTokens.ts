// What quick add calls each kind of token on its chips, for quick add and
// for help, which shows the same chips without loading the parser.
import type { TokenKind } from './quickadd';

export const TOKEN_LABELS: Record<TokenKind, string> = {
	date: 'Due',
	time: 'At',
	project: 'Project',
	priority: 'Priority',
	person: 'For',
	label: 'Label',
	repeat: 'Repeats',
	reminder: 'Reminder'
};
