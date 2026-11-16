import { defineConfig, devices } from '@playwright/test';
import { SHOTS_SERVER } from './e2e/shots/shots';

// The documentation's pictures (e2e/shots): the help topics' (help.shots.ts, into
// static/help-images) and the guide's (guide.shots.ts, into docs/guide/images),
// taken on a server of their own from the workspace docs.setup.ts seeds. That is
// scripts/sample-data.py's small business, made around the day it runs, so a
// picture shows a real day's work. Run by `make shots`, in the pinned
// Playwright image, before a release and after a screen in a picture changes.
export default defineConfig({
	testDir: 'e2e/shots',
	testMatch: '**/*.shots.ts',
	reporter: 'list',
	// One at a time: every file sets the same person's theme and panel preferences.
	workers: 1,
	timeout: 300_000,
	use: {
		baseURL: SHOTS_SERVER,
		...devices['Desktop Chrome'],
		// Narrow enough that a picture's words can be read in a topic's column.
		viewport: { width: 1000, height: 720 },
		deviceScaleFactor: 1.5,
		reducedMotion: 'reduce',
		serviceWorkers: 'block',
		timezoneId: 'UTC',
		locale: 'en-US'
	},
	projects: [
		{ name: 'seed', testMatch: /docs\.setup\.ts/ },
		{ name: 'shots', dependencies: ['seed'] }
	],
	webServer: {
		command: [
			'cd .. && test -x bin/mila && rm -rf tmp/shots && mkdir -p tmp/shots/mail',
			'&& python3 -I scripts/sample-data.py tmp/shots/sample --set business --seed 3 > /dev/null',
			'&& ./bin/mila serve --update-check off --listen 127.0.0.1:4177 --base-url ' + SHOTS_SERVER,
			'--data-dir tmp/shots/data --smtp-url file://$PWD/tmp/shots/mail --sign-in-rate 1000 --invite-rate 1000',
			'--log-format text --log-level warn > tmp/shots/server.log 2>&1'
		].join(' '),
		url: SHOTS_SERVER + '/healthz',
		timeout: 60_000,
		reuseExistingServer: false
	}
});
