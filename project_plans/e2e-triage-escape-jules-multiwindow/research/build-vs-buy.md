# Build vs. buy

Test-only fixes inside an existing Playwright suite. No library or SaaS applies.
- Adopt: Playwright's built-in `page.routeWebSocket` (**Recommended**, no new dependency).
- A shared "non-announcer alerts" locator helper could dedupe the two locators (**Viable**, but the two sites are in different files and the jscpd gate does not flag them; keep the fix minimal).
- Fork/SaaS/custom library: **Not recommended**.
