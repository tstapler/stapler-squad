// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	site: 'https://tstapler.github.io',
	base: '/stapler-squad',
	integrations: [
		starlight({
			title: 'Stapler Squad',
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/tstapler/stapler-squad' },
			],
			// Placeholder groups for Epic 1.1 — later Phase 4 tasks (4.1.1a, 4.2.1a, 4.3.1a)
			// add real content under each without touching this structure.
			sidebar: [
				{
					label: 'Getting Started',
					items: [],
				},
				{
					label: 'Features',
					items: [],
				},
				{
					label: 'Troubleshooting',
					items: [],
				},
			],
		}),
	],
});
