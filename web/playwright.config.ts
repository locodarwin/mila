import { defineConfig, devices } from '@playwright/test';

// Browser tests run against the real binary, so the embedded build, the
// routing, the security headers and sign-in are all part of what is
// tested. The server sends email to a directory the tests read, and its
// log (where the setup link appears) goes to a file.
// MILA_E2E_PORT runs a second server beside one already under test.
const PORT = process.env.MILA_E2E_PORT || '4173';
export const SERVER = `http://localhost:${PORT}`;
export const DATA = '../tmp/e2e';

export default defineConfig({
	testDir: 'e2e',
	fullyParallel: true,
	// GitHub's runners have few cores, and Playwright's default of half of
	// them leaves one worker; two halve the run.
	// One server for every test: left to itself Playwright takes half the cores,
	// and at 16 workers ordinary waits run out on pages that have nothing wrong.
	workers: process.env.CI ? 2 : 6,
	// Those runners are slower and vary from run to run, so CI waits longer.
	timeout: process.env.CI ? 60_000 : 30_000,
	expect: { timeout: process.env.CI ? 10_000 : 5_000 },
	reporter: process.env.CI ? 'github' : 'list',
	use: { baseURL: SERVER },
	projects: [
		{ name: 'setup', testMatch: /global\.setup\.ts/ },
		{
			name: 'desktop',
			testMatch: '**/*.e2e.ts',
			dependencies: ['setup'],
			use: { ...devices['Desktop Chrome'] }
		},
		{
			name: 'phone',
			testMatch: '**/*.e2e.ts',
			dependencies: ['setup'],
			use: { ...devices['Pixel 7'] }
		}
	],
	webServer: {
		command: [
			'cd .. && make build && rm -rf tmp/e2e && mkdir -p tmp/e2e/inbound',
			`&& ./bin/mila serve --update-check off --listen 127.0.0.1:${PORT} --base-url ` + SERVER,
			'--data-dir tmp/e2e/data --smtp-url file://$PWD/tmp/e2e/mail --sign-in-rate 1000 --invite-rate 10000 --oauth on',
			"--imap-url file://$PWD/tmp/e2e/inbound --inbound-address 'mila+{token}@example.test' --imap-poll 1",
			'--log-format text 2>&1 | tee tmp/e2e/server.log'
		].join(' '),
		url: SERVER + '/healthz',
		timeout: 180_000,
		reuseExistingServer: false
	}
});
