import { defineConfig, devices } from '@playwright/test';

// Mila as the desktop app runs it on this computer, with a server of its own, since
// that's a different mode from the one the other tests use.
const SERVER = 'http://127.0.0.1:4175';

export default defineConfig({
	testDir: 'e2e/local',
	testMatch: '**/*.local.ts',
	fullyParallel: false,
	workers: 1,
	reporter: process.env.CI ? 'github' : 'list',
	use: { baseURL: SERVER, trace: 'retain-on-failure' },
	projects: [{ name: 'local', use: { ...devices['Desktop Chrome'] } }],
	webServer: {
		command: [
			'cd .. && make build && rm -rf tmp/e2e-local && mkdir -p tmp/e2e-local &&',
			'MILA_LOCAL=on MILA_LOCAL_KEY=a-key-for-the-browser-tests-0123456789',
			'./bin/mila serve --listen 127.0.0.1:4175 --base-url ' +
				SERVER +
				' --data-dir tmp/e2e-local/data --log-format text'
		].join(' '),
		url: SERVER + '/healthz',
		timeout: 180_000,
		reuseExistingServer: false
	}
});
