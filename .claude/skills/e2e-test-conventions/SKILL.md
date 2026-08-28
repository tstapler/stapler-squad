---
name: e2e-test-conventions
description: Use when writing or reviewing Playwright e2e specs in stapler-squad's tests/e2e/ — enforces the four hard, CI-checked conventions — feature annotation headers, no waitForTimeout, data-testid/ARIA-only locators, and page helpers under tests/e2e/pages/.
globs: ["tests/e2e/**/*.ts"]
---

# E2E Test Conventions

Four hard conventions for all Playwright specs in `tests/e2e/`. CI enforces these.

## When to Use This Skill

- Writing a new Playwright spec file under `tests/e2e/`
- Reviewing a PR that adds or modifies an e2e test
- Debugging a CI failure on the e2e suite that looks like a convention violation rather
  than a real regression

## 1. Feature annotation header

Every spec file must start with a feature annotation comment:

```typescript
// @feature session:create, session:list
```

## 2. No `waitForTimeout`

**Wrong:**
```typescript
await page.waitForTimeout(1000);
```

**Right:**
```typescript
await expect(locator).toHaveValue('expected');
// or
await page.waitForSelector('[data-testid="my-element"]');
```

## 3. Locators: `data-testid` or ARIA roles only

**Wrong:**
```typescript
page.locator('.session-card .title')  // CSS class selector
```

**Right:**
```typescript
page.getByTestId('session-title')
page.getByRole('button', { name: 'Create session' })
```

## 4. New page helpers go in `tests/e2e/pages/`

Extract reusable page interaction logic into helper classes under `tests/e2e/pages/` — don't inline repeated navigation or form-filling logic in spec files.

## Test server

Tests run against `http://localhost:8544`. Start it before running:
```bash
STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &
```
