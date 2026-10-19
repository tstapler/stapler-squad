// Uses Astro's own getViteConfig (not vitest/config's defineConfig) so the
// project's real vite pipeline -- including the Astro/MDX transforms Starlight
// registers -- is available to tests, letting Epic 3.x's component tests
// import .astro files directly and render them via astro/container.
import { getViteConfig } from 'astro/config';

export default getViteConfig({
	test: {
		include: ['tests/unit/**/*.test.ts', 'tests/integration/**/*.test.ts'],
		// Several integration-style tests (build.test.ts, not-found.test.ts,
		// check-internal-links.integration.test.ts, check-asset-health's) each
		// run a real `bun run build` into the one shared website/dist/ -- in
		// parallel test files that races on the same `dist/.prerender` temp
		// output and corrupts the build. Serializing file execution trades some
		// wall-clock time for a dist/ that's never written by two builds at once.
		fileParallelism: false,
	},
});
