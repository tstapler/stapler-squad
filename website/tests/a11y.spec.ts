// Phase 6, Epic 6.2 (Story 6.2.1): axe-core accessibility gate for the
// marketing/help site. Mirrors tests/e2e/accessibility.spec.ts's EXACT
// severity threshold and pattern (same @axe-core/playwright library, same
// `impact === 'critical' || impact === 'serious'` filter, same
// `expect(...).toHaveLength(0)` assertion) per research/ux.md §3's explicit
// "reuse the same library" instruction -- this is a second site, not a
// second a11y-tooling choice.
import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

// Relative, non-"/"-prefixed paths: see playwright.config.ts's baseURL
// comment for why a leading "/" here would silently drop the
// "/stapler-squad" base path and scan the wrong (404) page.
const PAGES = {
	landing: '',
	gettingStarted: 'help/getting-started/',
} as const;

/** Same format tests/e2e/accessibility.spec.ts uses for its console.error dump. */
function formatViolation(v: import('axe-core').Result): string {
	const affected = v.nodes
		.slice(0, 2)
		.map((n) => n.target.join(', '))
		.join('; ');
	return `\n  [${v.impact?.toUpperCase()}] ${v.id}: ${v.description}\n    Affected: ${affected}`;
}

function logViolations(path: string, violations: import('axe-core').Result[]): void {
	if (violations.length === 0) return;
	console.error(`Accessibility violations found on ${path}:${violations.map(formatViolation).join('')}`);
}

test.describe('Accessibility (WCAG 2.1 AA) — stapler-squad-website', () => {
	// design/ux.md §5 criterion 15: every <img> has descriptive alt text or
	// alt="" (purely decorative) -- axe's `image-alt` rule, run across both
	// pages above.
	test('images_should_HaveDescriptiveAltOrEmptyAlt_When_AxeScanRun', async ({ page }) => {
		for (const [label, path] of Object.entries(PAGES)) {
			await page.goto(path, { waitUntil: 'load' });

			const results = await new AxeBuilder({ page }).withRules(['image-alt']).analyze();
			logViolations(`${path} (${label})`, results.violations);

			expect(results.violations).toHaveLength(0);
		}
	});

	// design/ux.md §5 criterion 15: every icon-only button (search trigger,
	// copy button) has an aria-label describing its action, not just a
	// visual glyph -- axe's `button-name` rule.
	test('iconButtons_should_HaveAriaLabel_When_AxeScanRun', async ({ page }) => {
		for (const [label, path] of Object.entries(PAGES)) {
			await page.goto(path, { waitUntil: 'load' });

			const results = await new AxeBuilder({ page }).withRules(['button-name']).analyze();
			logViolations(`${path} (${label})`, results.violations);

			expect(results.violations).toHaveLength(0);
		}
	});

	// design/ux.md §5 criterion 16: color contrast >=4.5:1 normal /
	// >=3:1 large text, verified before merge rather than fixed post-hoc (the
	// two theme.css.ts precedents this test warns against repeating), with no
	// severity filter -- catches a moderate/minor contrast issue the
	// critical/serious-only scan below wouldn't flag. Scoped to `.hero` on the
	// landing page (research/ux.md §3's CTA-button check) and the whole page
	// on docs (its sidebar link text has no equivalent container to scope to).
	const CONTRAST_SCANS = {
		landing: { path: PAGES.landing, include: '.hero' },
		gettingStarted: { path: PAGES.gettingStarted, include: undefined },
	} as const;
	for (const [label, { path, include }] of Object.entries(CONTRAST_SCANS)) {
		test(`ctaAndLinkText_should_PassWcagAAContrast_When_AxeScanRunBeforeMerge (${label})`, async ({ page }) => {
			await page.goto(path, { waitUntil: 'load' });

			let builder = new AxeBuilder({ page }).withRules(['color-contrast']);
			if (include) builder = builder.include(include);
			const results = await builder.analyze();
			logViolations(`${path} (${label})`, results.violations);

			expect(results.violations).toHaveLength(0);
		});
	}

	// design/ux.md §5 criterion 19: the combined CI gate -- Lighthouse
	// accessibility/performance/best-practices >=0.90 (enforced separately
	// by lighthouserc.json + lighthouserc.mobile.json via `pnpm run
	// lhci:desktop`/`lhci:mobile`, website/tests/unit/lighthouserc.test.ts,
	// and website/tests/integration/lighthouse.integration.test.ts -- not
	// re-implemented here) plus zero critical/serious axe-core violations on
	// the landing page and at least one docs page template. This test
	// enforces the axe half of that combined gate across both pages in one
	// assertion, matching the set of pages Lighthouse's own config scans.
	test('lighthouseAndAxeGate_should_PassOnLandingAndOneDocsPage_When_CiRuns', async ({ page }) => {
		for (const [label, path] of Object.entries(PAGES)) {
			await page.goto(path, { waitUntil: 'load' });

			const results = await new AxeBuilder({ page }).analyze();
			const criticalViolations = results.violations.filter(
				(v) => v.impact === 'critical' || v.impact === 'serious',
			);
			logViolations(`${path} (${label})`, criticalViolations);

			expect(criticalViolations).toHaveLength(0);
		}
	});
});
