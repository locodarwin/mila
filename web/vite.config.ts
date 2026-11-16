import { defineConfig } from 'vitest/config';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';

export default defineConfig({
	server: {
		// make dev: the Go server runs on 8790 and Vite proxies the API (and
		// calendar feeds) to it.
		proxy: {
			// The API reference at /api/docs is a page of the app.
			'^/api/(?!docs)': 'http://127.0.0.1:8790',
			'/ical': 'http://127.0.0.1:8790',
			'/healthz': 'http://127.0.0.1:8790'
		}
	},
	build: {
		// Never inline assets as data: URLs; the CSP only allows files from 'self'.
		assetsInlineLimit: 0
	},
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// A single-page app: every unknown path gets index.html, which the Go
			// server embeds and serves for client-side routes. Every file also
			// gets .br and .gz copies, which the server sends to browsers that
			// take them (server/internal/httpapi/spa.go).
			adapter: adapter({ fallback: 'index.html', precompress: true })
		})
	],
	test: {
		expect: { requireAssertions: true },
		projects: [
			{
				extends: './vite.config.ts',
				test: {
					name: 'server',
					environment: 'node',
					include: ['src/**/*.{test,spec}.{js,ts}'],
					exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
				}
			}
		]
	}
});
