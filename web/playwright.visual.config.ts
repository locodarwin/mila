import { defineConfig, devices } from '@playwright/test';

// Visual regression of the style guide (/dev/ui) and the main screens in
// every theme: four characters, light and dark. Screenshots differ between machines (fonts,
// rasterizing), so the baselines are made and checked in one place only,
// the Playwright image pinned in the Makefile (PW_IMAGE) and in CI, which
// must match @playwright/test's exact version: `make visual` checks,
// `make visual-update` makes new baselines after an intended change.
// It runs the binary `make build` made, on a server of its own.
export const VISUAL_SERVER = 'http://localhost:4176';

export default defineConfig({
	testDir: 'e2e/visual',
	testMatch: '**/*.visual.ts',
	fullyParallel: true,
	reporter: process.env.CI ? 'github' : 'list',
	snapshotPathTemplate: 'e2e/visual/__screenshots__/{arg}{ext}',
	expect: {
		toHaveScreenshot: {
			animations: 'disabled',
			caret: 'hide',
			scale: 'css',
			stylePath: 'e2e/visual/screenshot.css',
			// Same image, same browser: only antialiasing may stray.
			maxDiffPixelRatio: 0.002
		}
	},
	use: {
		baseURL: VISUAL_SERVER,
		...devices['Desktop Chrome'],
		viewport: { width: 1100, height: 900 },
		reducedMotion: 'reduce',
		timezoneId: 'UTC',
		locale: 'en-US'
	},
	projects: [
		{ name: 'seed', testMatch: /screens\.setup\.ts/ },
		{ name: 'visual', dependencies: ['seed'] }
	],
	webServer: {
		command: [
			'cd .. && test -x bin/mila && rm -rf tmp/visual && mkdir -p tmp/visual',
			'&& ./bin/mila serve --update-check off --listen 127.0.0.1:4176 --base-url ' + VISUAL_SERVER,
			'--data-dir tmp/visual/data --log-format text --log-level warn > tmp/visual/server.log 2>&1'
		].join(' '),
		url: VISUAL_SERVER + '/healthz',
		timeout: 60_000,
		reuseExistingServer: false
	}
});
