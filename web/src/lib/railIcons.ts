// The icons beside Settings' pages in the rail, as lucide draws them (ISC
// license), each made one SVG path for one small component, RailIcon.svelte:
// circles, rectangles and lines become path commands, and each shape starts
// with an absolute move. A component per icon cost Import from another app
// its place under the size budget. To change one, take its shapes from
// node_modules/@lucide/svelte/dist/icons/<name>.svelte.

export const RAIL_ICONS = {
	general:
		'M10 12h4M10 8h4M14 21v-3a2 2 0 0 0-4 0v3M6 10H4a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-2M6 21V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v16',
	people:
		'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M16 3.128a4 4 0 0 1 0 7.744M22 21v-2a4 4 0 0 0-3-3.87M5 7a4 4 0 1 0 8 0a4 4 0 1 0 -8 0',
	labels:
		'M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42zM7 7.5a0.5 0.5 0 1 0 1 0a0.5 0.5 0 1 0 -1 0',
	templates:
		'M4 3h16a1 1 0 0 1 1 1v5a1 1 0 0 1 -1 1h-16a1 1 0 0 1 -1 -1v-5a1 1 0 0 1 1 -1zM4 14h7a1 1 0 0 1 1 1v5a1 1 0 0 1 -1 1h-7a1 1 0 0 1 -1 -1v-5a1 1 0 0 1 1 -1zM17 14h3a1 1 0 0 1 1 1v5a1 1 0 0 1 -1 1h-3a1 1 0 0 1 -1 -1v-5a1 1 0 0 1 1 -1z',
	automations:
		'M15.914 4a1.5 1.5 0 00-2.474-1.561l-9 9A1.5 1.5 0 005.5 14h4.002a.5.5 0 01.471.666L8.086 20a1.5 1.5 0 002.475 1.56l9-9A1.5 1.5 0 0018.5 10h-3.997a.5.5 0 01-.472-.667z',
	themes:
		'M11 17a4 4 0 0 1-8 0V5a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2ZM16.7 13H19a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2H7M 7 17h.01M11 8l2.3-2.3a2.4 2.4 0 0 1 3.404.004L18.6 7.6a2.4 2.4 0 0 1 .026 3.434L9.9 19.8',
	webhooks:
		'M18 16.98h-5.99c-1.1 0-1.95.94-2.48 1.9A4 4 0 0 1 2 17c.01-.7.2-1.4.57-2M6 17l3.13-5.78c.53-.97.1-2.18-.5-3.1a4 4 0 1 1 6.89-4.06M12 6l3.13 5.73C15.66 12.7 16.9 13 18 13a4 4 0 0 1 0 8',
	calendar:
		'M8 2v3M16 2v3M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1 -2 2h-14a2 2 0 0 1 -2 -2v-14a2 2 0 0 1 2 -2zM3 9h18',
	email:
		'M22 7l-8.991 5.727a2 2 0 0 1-2.009 0L2 7M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-16a2 2 0 0 1 -2 -2v-12a2 2 0 0 1 2 -2z',
	data: 'M3 16l4 4 4-4M7 20V4M21 8l-4-4-4 4M17 4v16',
	profile: 'M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2M8 7a4 4 0 1 0 8 0a4 4 0 1 0 -8 0',
	appearance:
		'M12 22a1 1 0 0 1 0-20 10 9 0 0 1 10 9 5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8zM13 6.5a0.5 0.5 0 1 0 1 0a0.5 0.5 0 1 0 -1 0M17 10.5a0.5 0.5 0 1 0 1 0a0.5 0.5 0 1 0 -1 0M6 12.5a0.5 0.5 0 1 0 1 0a0.5 0.5 0 1 0 -1 0M8 7.5a0.5 0.5 0 1 0 1 0a0.5 0.5 0 1 0 -1 0',
	preferences: 'M10 5H3M12 19H3M14 3v4M16 17v4M21 12h-9M21 19h-5M21 5h-7M8 10v4M8 12H3',
	keyboard:
		'M10 8h.01M12 12h.01M14 8h.01M16 12h.01M18 8h.01M6 8h.01M7 16h10M8 12h.01M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-16a2 2 0 0 1 -2 -2v-12a2 2 0 0 1 2 -2z',
	notifications:
		'M10.268 21a2 2 0 0 0 3.464 0M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326',
	security:
		'M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z',
	tokens:
		'M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4zM16 7.5a0.5 0.5 0 1 0 1 0a0.5 0.5 0 1 0 -1 0',
	admin:
		'M4 2h16a2 2 0 0 1 2 2v4a2 2 0 0 1 -2 2h-16a2 2 0 0 1 -2 -2v-4a2 2 0 0 1 2 -2zM4 14h16a2 2 0 0 1 2 2v4a2 2 0 0 1 -2 2h-16a2 2 0 0 1 -2 -2v-4a2 2 0 0 1 2 -2zM6 6L6.01 6M6 18L6.01 18'
} as const;

export type RailIconName = keyof typeof RAIL_ICONS;
