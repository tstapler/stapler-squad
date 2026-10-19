// @feature files:tabs
// Task 0.1.4c (ADR-036 S-T3) and Task 1.6.4c: measures the two lazy-document models plus the production engine ("engine") on the 1,000,000-line virtual file served by
// web-app/src/app/spike-lazydoc (throwaway page). Pass/fail per criterion is printed as "SPIKE 0.1.4c ..." lines.
import { test, expect, type Page } from "@playwright/test";
import { suppressOnboarding } from "./pages/files-tab.page";

const PHONE_BUDGET_BYTES = 8 * 1024 * 1024;

interface Api {
  lineHeight(): number;
  topAbs(): number;
  topLineClientTop(): number;
  scrollToAbs(n: number): void;
  loadAboveViewport(n: number): { ok: boolean; before: { top: number; y: number } };
  cycles(n: number): void;
  type(t: string): void;
  placeCaretInViewport(): void;
  undoDepth(): number;
  stats(): { loadedChars: number; segs: number; heap: number; dirtySegs: number };
  composeProbe(): { dispatchedWhileComposing: boolean };
}

async function call<T>(page: Page, fn: keyof Api, ...args: unknown[]): Promise<T> {
  return page.evaluate(
    async ([name, a]) => {
      const api = (window as unknown as { __spike: { api: Record<string, (...x: unknown[]) => unknown> } }).__spike.api;
      const out = api[name as string](...(a as unknown[]));
      // Let CodeMirror measure and re-anchor before the caller reads layout.
      await new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r())));
      return out;
    },
    [fn, args] as const
  ) as Promise<T>;
}

for (const model of ["a2", "a1", "engine"] as const) {
  test.describe(`lazy document model ${model}`, () => {
    test.beforeEach(async ({ page }) => {
      await suppressOnboarding(page);
      await page.goto(`/spike-lazydoc/?m=${model}`);
      await expect(page.getByTestId("lazydoc-editor")).toHaveAttribute("data-ready", "true");
    });

    test(`lazyDoc_should_KeepScrollAnchorWithin1LineHeight_On50MbVirtualFile_${model}`, async ({ page, browserName }) => {
      const lh = await call<number>(page, "lineHeight");
      await call(page, "scrollToAbs", 500_000);
      // a2 fills the viewport asynchronously; measure only once the top line is real text.
      await expect.poll(() => call<number>(page, "topAbs")).toBeGreaterThan(0);
      const top0 = await call<number>(page, "topAbs");
      const y0 = await call<number>(page, "topLineClientTop");
      const { ok } = await call<{ ok: boolean }>(page, "loadAboveViewport", 3000);
      await expect.poll(() => call<number>(page, "topAbs")).toBeGreaterThan(0);
      const top1 = await call<number>(page, "topAbs");
      const y1 = await call<number>(page, "topLineClientTop");
      console.log(`SPIKE 0.1.4c ${model} ${browserName} anchor: loaded=${ok} topAbs ${top0}->${top1} clientTop ${y0}->${y1} lineHeight=${lh}`);
      expect(Math.abs(top1 - top0)).toBeLessThanOrEqual(1);
      expect(Math.abs(y1 - y0)).toBeLessThanOrEqual(lh);
    });

    test(`lazyDoc_should_KeepUndoAndHeapBudget_Across100LoadEvictCycles_${model}`, async ({ page, browserName }) => {
      await call(page, "scrollToAbs", 300_000);
      await expect.poll(() => call<number>(page, "topAbs")).toBeGreaterThan(0);
      await call(page, "placeCaretInViewport");
      await call(page, "type", "ab");
      const depth = await call<number>(page, "undoDepth");
      expect(depth).toBeGreaterThan(0);
      await call(page, "cycles", 100);
      const after = await call<number>(page, "undoDepth");
      const stats = await call<{ loadedChars: number; segs: number; heap: number; dirtySegs: number }>(page, "stats");
      console.log(`SPIKE 0.1.4c ${model} ${browserName} cycles: undo ${depth}->${after} stats=${JSON.stringify(stats)}`);
      expect(after).toBe(depth);
      expect(stats.loadedChars).toBeLessThanOrEqual(PHONE_BUDGET_BYTES);
    });

    test(`lazyDoc_should_NotDispatchWhileComposing_${model}`, async ({ page }) => {
      const probe = await call<{ dispatchedWhileComposing: boolean }>(page, "composeProbe");
      expect(probe.dispatchedWhileComposing).toBe(false);
    });
  });
}
