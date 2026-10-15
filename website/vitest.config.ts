// Uses Astro's own getViteConfig (not vitest/config's defineConfig) so the
// project's real vite pipeline -- including the Astro/MDX transforms Starlight
// registers -- is available to tests, letting Epic 3.x's component tests
// import .astro files directly and render them via astro/container.
import { getViteConfig } from 'astro/config';

export default getViteConfig({
	test: {
		include: ['tests/unit/**/*.test.ts', 'tests/integration/**/*.test.ts'],
	},
});
