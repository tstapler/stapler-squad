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
			// Phase 6 (Epic 6.2): fixes two axe-core `serious` violations found by
			// a real scan against Starlight's own default markup (not something
			// this project's own components introduced) -- see
			// website/tests/a11y.spec.ts. Site-wide via `head` since both apply
			// to every page: the search-trigger button (every page's header) and
			// code blocks (most docs pages, this project's InstallCommand.astro).
			head: [
				{
					tag: 'script',
					content: `
						function fixSearchButtonLabel() {
							// Root cause, confirmed by reading axe-core's own
							// label-content-name-mismatch check source (not guessed): it
							// compares raw ON-SCREEN text (ignoring aria-hidden entirely --
							// it calls visibleVirtual() with screenReader=false) against the
							// accessible name from aria-label="Search". Once JS reveals the
							// "Ctrl"/"K" <kbd> shortcut hint for non-touch devices, the
							// button's on-screen text is "Search" immediately followed by
							// "Ctrl" immediately followed by "K", with NO whitespace text
							// node between any of them in Starlight's markup -- confirmed
							// with axe-core's own visibleVirtual() called directly against
							// this exact DOM, which returned the literal string
							// "Search CtrlK" (the outer span/kbd boundary already had a gap;
							// the two *inner* "Ctrl"/"K" <kbd> siblings did not). axe's
							// word-tokenizer reads "CtrlK" as one unbroken token that can
							// never match the two separate "ctrl"/"k" words in a
							// multi-word accessible name. Fix both gaps: insert a real space
							// at each missing boundary so every word tokenizes separately,
							// and extend aria-label to include them (unconditionally --
							// harmless when the hint is hidden on mobile/touch, since the
							// check only requires visible text to be a subsequence of the
							// name, not an exact match).
							document.querySelectorAll('button[data-open-modal]').forEach((btn) => {
								const kbdWrapper = btn.querySelector(':scope > kbd');
								if (!kbdWrapper) return;
								if (btn.dataset.a11yOriginalLabel === undefined) {
									btn.dataset.a11yOriginalLabel = btn.getAttribute('aria-label') || 'Search';
								}
								const prev = kbdWrapper.previousSibling;
								if (!prev || prev.nodeType !== Node.TEXT_NODE) {
									btn.insertBefore(document.createTextNode(' '), kbdWrapper);
								}
								const innerKbds = Array.from(kbdWrapper.querySelectorAll(':scope > kbd'));
								for (let i = 1; i < innerKbds.length; i++) {
									const prevInner = innerKbds[i].previousSibling;
									if (!prevInner || prevInner.nodeType !== Node.TEXT_NODE) {
										kbdWrapper.insertBefore(document.createTextNode(' '), innerKbds[i]);
									}
								}
								const shortcutWords = innerKbds.map((el) => el.textContent.trim()).filter(Boolean);
								btn.setAttribute('aria-label', [btn.dataset.a11yOriginalLabel, ...shortcutWords].join(' '));
							});
						}
						function fixScrollableCodeBlocks() {
							// A <pre> wider than its box scrolls horizontally but isn't
							// natively keyboard-focusable in Safari (axe's
							// scrollable-region-focusable) -- only tag the ones that are
							// actually overflowing, not every <pre>, so this doesn't add
							// a pointless extra Tab stop on short code blocks.
							document.querySelectorAll('pre').forEach((el) => {
								if (!el.hasAttribute('tabindex') && el.scrollWidth > el.clientWidth) {
									el.setAttribute('tabindex', '0');
								}
							});
						}
						function runA11yFixups() {
							fixSearchButtonLabel();
							fixScrollableCodeBlocks();
						}
						document.addEventListener('DOMContentLoaded', runA11yFixups);
						window.addEventListener('load', runA11yFixups);
						window.addEventListener('resize', runA11yFixups);
					`,
				},
			],
		}),
		mdx(),
	],
});
