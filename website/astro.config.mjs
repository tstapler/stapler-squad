// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import mdx from '@astrojs/mdx';

// https://astro.build/config
export default defineConfig({
	site: 'https://tstapler.github.io',
	base: '/stapler-squad',
	integrations: [
		starlight({
			title: 'Stapler Squad',
			// Task 4.4.2a: src/pages/404.astro (built on <StarlightPage>) replaces
			// Starlight's own built-in 404 route, which otherwise collides with it
			// at the same "/404" pattern (Astro build warning, promoted to a hard
			// error in later Astro versions).
			disable404Route: true,
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/tstapler/stapler-squad' },
			],
			// Phase 4 (Epics 4.1-4.3): real content under each group. `slug`-only
			// entries (no `label`) use each page's own frontmatter title, so the
			// sidebar can't drift from what the page itself says it's called.
			sidebar: [
				{
					label: 'Getting Started',
					items: [{ slug: 'help/getting-started' }],
				},
				{
					label: 'Features',
					items: [{ slug: 'help/features' }],
				},
				{
					// Explicit slug list (not `autogenerate: { directory: 'help' }`)
					// because help/ also holds getting-started.md and features.mdx,
					// which belong in the groups above, not here.
					label: 'Troubleshooting',
					items: [
						{ slug: 'help/add-user-themes' },
						{ slug: 'help/debug-with-logs' },
						{ slug: 'help/dispatch-work-to-google-jules' },
						{ slug: 'help/enable-pi-support' },
						{ slug: 'help/expose-github-webhook-endpoint' },
						{ slug: 'help/expose-slack-interactive-endpoint' },
						{ slug: 'help/macos-codesigning' },
						{ slug: 'help/manage-notifications' },
						{ slug: 'help/manage-systemd-service' },
						{ slug: 'help/monitor-external-terminal-sessions' },
						{ slug: 'help/use-up-next-pr-tabs-and-ask-to-fix' },
					],
				},
			],
		}),
		mdx(),
	],
});
