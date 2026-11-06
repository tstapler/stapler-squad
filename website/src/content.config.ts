import { defineCollection } from 'astro:content';
import { docsLoader, i18nLoader } from '@astrojs/starlight/loaders';
import { docsSchema, i18nSchema } from '@astrojs/starlight/schema';

export const collections = {
	docs: defineCollection({ loader: docsLoader(), schema: docsSchema() }),
	// Task 4.4.1a: overrides Pagefind's default zero-results string (plain
	// "No results for [SEARCH_TERM]") with one that names an exit path, since
	// Pagefind's default UI renders that string as a text node (verified by
	// reading @pagefind/default-ui's source) and can't embed a clickable link.
	i18n: defineCollection({ loader: i18nLoader(), schema: i18nSchema() }),
};
