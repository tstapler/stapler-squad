// Config for website/tests/a11y.spec.ts (Epic 6.2) and website/tests/e2e/**
// (the UX-acceptance suite closing validation.md's "UX Acceptance Tests"
// gap). `website/` is a standalone package from `web-app/`, so this is its
// own Playwright config rather than an addition to the root `tests/e2e/`
// suite -- extended in place here rather than duplicated, per
// e2e-test-conventions' page-helper/no-waitForTimeout/@feature conventions
// ported from that root suite.
//
// Deliberately NOT using Playwright's built-in `webServer` option: `astro
// preview` daemonizes itself (its CLI invocation returns once the detached
// server is confirmed listening, printing a "Preview server running"
// status line, rather than blocking in the foreground) -- Playwright reads
// that immediate, successful exit as "Process from config.webServer exited
// early" and refuses to start, regardless of the real server being up.
// `bun run test:a11y` (website/package.json) instead starts the server
// explicitly with `bun run preview`, which blocks until ready and returns,
// then runs this config, then stops it with `astro preview stop` --
// the same explicit start/test/stop sequence deploy-website.yml's a11y step
// uses.
import { defineConfig } from '@playwright/test';

const PORT = 4321;
const BASE_PATH = '/stapler-squad';

export default defineConfig({
	testDir: './tests',
	testMatch: ['a11y.spec.ts', 'e2e/**/*.spec.ts'],
	timeout: 60_000,
	fullyParallel: false,
	retries: process.env.CI ? 1 : 0,
	use: {
		// Trailing slash matters: Playwright resolves a relative goto() path
		// against baseURL via the WHATWG URL algorithm, where a leading "/"
		// is absolute-from-origin and silently discards the "/stapler-squad"
		// base path -- this bit the first version of a11y.spec.ts (it
		// navigated to the bare site root instead of the base-pathed page,
		// scanning a 404 page without erroring). Relative, non-"/"-prefixed
		// paths in a11y.spec.ts resolve correctly against this trailing slash.
		baseURL: `http://localhost:${PORT}${BASE_PATH}/`,
	},
});
