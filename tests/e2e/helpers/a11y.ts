import type { Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

/** WCAG 2.x A/AA violations on the page, as "rule: target" strings (empty when clean). */
export async function axeAaViolations(page: Page): Promise<string[]> {
  const result = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"]).analyze();
  return result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`);
}

/** No horizontal page scroll, and every visible button/link/input sits inside the viewport horizontally. */
export async function assertReflow(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const problems: string[] = [];
    const el = document.scrollingElement as HTMLElement;
    if (el.scrollWidth > window.innerWidth) problems.push(`scrollWidth ${el.scrollWidth} > innerWidth ${window.innerWidth}`);
    for (const node of document.querySelectorAll<HTMLElement>("button, a[href], input, [role=button]")) {
      const r = node.getBoundingClientRect();
      if (r.width === 0 || r.height === 0) continue;
      // A deliberately scrollable strip (e.g. the session tab list) may extend past the viewport.
      let scroller = false;
      for (let p = node.parentElement; p && p !== document.body; p = p.parentElement) {
        const ox = getComputedStyle(p).overflowX;
        if ((ox === "auto" || ox === "scroll") && p.scrollWidth > p.clientWidth) { scroller = true; break; }
      }
      if (scroller) continue;
      if (r.left < -1 || r.right > window.innerWidth + 1) problems.push(`${node.getAttribute("aria-label") ?? node.textContent?.trim()} leaves the viewport`);
    }
    return problems;
  });
}

/** Accessible names of the focus stops reached by pressing Tab `count` times from the current focus. */
export async function tabOrderNames(page: Page, count: number): Promise<string[]> {
  const names: string[] = [];
  for (let i = 0; i < count; i++) {
    await page.keyboard.press("Tab");
    names.push(
      await page.evaluate(() => {
        const a = document.activeElement as HTMLElement | null;
        return a?.getAttribute("aria-label") ?? a?.textContent?.trim() ?? a?.tagName ?? "";
      }),
    );
  }
  return names;
}

/** Stubs window.visualViewport so a test can emulate the soft keyboard (real keyboards are a manual check). */
export async function stubVisualViewport(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const target = new EventTarget();
    const vv = Object.assign(target, { height: window.innerHeight, width: window.innerWidth, offsetTop: 0, offsetLeft: 0, scale: 1 });
    Object.defineProperty(window, "visualViewport", { configurable: true, value: vv });
    (window as unknown as { __setVisualViewportHeight: (h: number) => void }).__setVisualViewportHeight = (h: number) => {
      vv.height = h;
      target.dispatchEvent(new Event("resize"));
    };
  });
}

export async function setVisualViewportHeight(page: Page, height: number): Promise<void> {
  await page.evaluate((h) => (window as unknown as { __setVisualViewportHeight: (n: number) => void }).__setVisualViewportHeight(h), height);
}
