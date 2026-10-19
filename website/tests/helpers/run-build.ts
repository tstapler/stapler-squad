// Shared by every integration-style test that spawns a real `bun run build`.
//
// Astro's `getViteConfig` (used by vitest.config.ts so component tests can
// render .astro files) sets process.env.BASE_URL="/" and SITE globally for
// the test process, so `import.meta.env.BASE_URL` resolves inside unit
// tests. `execFileSync` inherits `process.env` by default, so a real build
// spawned from inside a test would otherwise silently bake in that
// test-mode BASE_URL instead of astro.config.mjs's real `/stapler-squad`
// base -- confirmed by diffing a build run this way against one run from a
// plain shell: every import.meta.env.BASE_URL-derived URL (e.g. Starlight's
// sidebar nav hrefs, via @astrojs/starlight's base.js) came out base-less,
// while Astro.site-derived canonical URLs (set directly from astro.config.mjs's
// `site`, not inherited env) stayed correct. Stripping these two vars before
// spawning lets the child process compute its own values from
// astro.config.mjs, matching a real `bun run build` run from a shell.
import { execFileSync } from 'node:child_process';

export function runRealBuild(cwd: string): void {
	const { BASE_URL, SITE, ...cleanEnv } = process.env;
	execFileSync('bun', ['run', 'build'], { cwd, stdio: 'inherit', env: cleanEnv });
}
