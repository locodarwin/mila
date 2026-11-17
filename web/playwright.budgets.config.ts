import { defineConfig, devices } from '@playwright/test';

// The browser's performance budgets, measured on
// a server of their own holding a generated 5,000-task workspace, so its
// data never meets the functional tests'. It runs the binary `make build`
// made; `make budgets` builds it first. One test at a time, so nothing
// else competes for the machine while it is timed.
export const BUDGET_SERVER = 'http://localhost:4174';
export const BUDGET_DATA = '../tmp/budgets';

export default defineConfig({
	testDir: 'e2e/budgets',
	testMatch: '**/*.budget.ts',
	workers: 1,
	fullyParallel: false,
	// Shared runners are noisy: one more try there, none here.
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? 'github' : 'list',
	use: { baseURL: BUDGET_SERVER },
	projects: [{ name: 'budgets', use: { ...devices['Desktop Chrome'] } }],
	webServer: {
		command: [
			'cd .. && test -x bin/mila && rm -rf tmp/budgets && mkdir -p tmp/budgets',
			'&& ./bin/mila dev generate --email perf@example.com --tasks 5000 --board-cards 500 --seed 1',
			'--data-dir tmp/budgets/data --base-url ' + BUDGET_SERVER + ' > tmp/budgets/generated.txt',
			'&& ./bin/mila serve --update-check off --listen 127.0.0.1:4174 --base-url ' + BUDGET_SERVER,
			'--data-dir tmp/budgets/data --log-format text --log-level warn 2>&1 | tee tmp/budgets/server.log'
		].join(' '),
		url: BUDGET_SERVER + '/healthz',
		timeout: 120_000,
		reuseExistingServer: false
	}
});
