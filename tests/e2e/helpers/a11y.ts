import type { Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

/**
 * WCAG 2.x A/AA violations, as "rule: target" strings (empty when clean). Scoped to the Files panel and its
 * dialogs: the surrounding session chrome (tab bar, nav) is not this feature's to fix. The CodeMirror
 * scroller is excluded: its syntax-token colors on the active-line/match highlight (primary on #3e3e42 is
 * 2.02:1) and axe's scrollable-region-focusable on its tabindex=-1 scroller (the contenteditable inside is
 * focusable) are an editor-theme follow-up, not a Files layout defect.
 */
export async function axeAaViolations(page: Page, scope?: string): Promise<string[]> {
  // Axe reads the colours painted right now: let finite fade-in/transition animations finish first, or a half-blended foreground reads as a contrast failure.
  await page.evaluate(() =>
    Promise.all(
      document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
        .map((a) => a.finished.catch(() => undefined)),
    ),
  );
  const builder = new AxeBuilder({ page });
  const result = await (scope ? builder.include(scope) : builder.include("[data-layout]").include("[role=dialog]").exclude(".cm-scroller")).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"]).analyze();
  return result.violations.map((v) => `${v.id}: ${v.nodes.map((n) => `${n.target.join(" ")} [${(n.any[0]?.message ?? n.failureSummary ?? "").slice(0, 160)}]`).join(", ")}`);
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

/**
 * Non-text contrast (WCAG 1.4.11, axe does not check it): for each element found by `data-testid`, the best ratio
 * between one of its visual cues (outline, border, own fill) and the background behind it must reach `min`.
 * Runs under both color schemes; returns the failing cases (empty when clean).
 */
export async function assertNonTextContrast(page: Page, testIds: string[], min = 3): Promise<string[]> {
  const problems: string[] = [];
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    const rows = await page.evaluate((ids) => {
      type Rgba = [number, number, number, number];
      const parse = (css: string): Rgba | null => {
        const m = css.match(/rgba?\(([^)]+)\)/);
        if (!m) return null;
        const [r, g, b, a = 1] = m[1].split(/[ ,/]+/).filter(Boolean).map(Number);
        return [r, g, b, a];
      };
      const lum = ([r, g, b]: Rgba) => {
        const f = (c: number) => { const v = c / 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; };
        return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
      };
      const ratio = (a: Rgba, b: Rgba) => { const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x); return (hi + 0.05) / (lo + 0.05); };
      const backdrop = (el: Element | null): Rgba => {
        for (let n = el; n; n = n.parentElement) {
          const c = parse(getComputedStyle(n).backgroundColor);
          if (c && c[3] > 0.99) return c;
        }
        return [255, 255, 255, 1];
      };
      return ids.map((id) => {
        const el = document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
        if (!el) return { id, best: -1, cues: "missing" };
        const cs = getComputedStyle(el);
        const behind = backdrop(el.parentElement);
        const cues: [string, Rgba | null][] = [
          ["outline", cs.outlineStyle !== "none" && parseFloat(cs.outlineWidth) > 0 ? parse(cs.outlineColor) : null],
          ["border", parseFloat(cs.borderTopWidth) > 0 ? parse(cs.borderTopColor) : null],
          ["fill", parse(cs.backgroundColor)],
        ];
        const ratios = cues.filter(([, c]) => c && c[3] > 0.99).map(([name, c]) => [name, ratio(c as Rgba, behind)] as const);
        return { id, best: Math.max(-1, ...ratios.map(([, r]) => r)), cues: ratios.map(([n, r]) => `${n} ${r.toFixed(2)}`).join(", ") };
      });
    }, testIds);
    for (const r of rows) if (r.best < min) problems.push(`${scheme}: ${r.id} best cue ${r.best.toFixed(2)}:1 < ${min}:1 (${r.cues})`);
  }
  await page.emulateMedia({ colorScheme: null });
  return problems;
}

/** Runs `fn` with Chromium's achromatopsia emulation on (WCAG 1.4.1: state must not ride on color alone). Chromium only. */
export async function withAchromatopsia<T>(page: Page, fn: () => Promise<T>): Promise<T> {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setEmulatedVisionDeficiency", { type: "achromatopsia" });
  try {
    return await fn();
  } finally {
    await cdp.send("Emulation.setEmulatedVisionDeficiency", { type: "none" });
    await cdp.detach();
  }
}

/** Visible text of every element matching `locatorText`, as an array; a fixed list the caller compares for distinctness. */
export async function visibleTexts(page: Page, selector: string): Promise<string[]> {
  return page.$$eval(selector, (els) => els.map((e) => (e.textContent ?? "").replace(/\s+/g, " ").trim()).filter(Boolean));
}

/**
 * Dark then light AA runs of one scope. The app picks its theme from localStorage "stapler-theme" (ThemeContext),
 * not from prefers-color-scheme, so each run sets it, reloads and calls `reopen` to get the surface back.
 * Returns violations tagged with the scheme.
 */
export async function axeLightAndDark(page: Page, scope: string, reopen: () => Promise<void>): Promise<string[]> {
  const out: string[] = [];
  for (const scheme of ["dark", "light"] as const) {
    await page.evaluate((t) => localStorage.setItem("stapler-theme", t), scheme);
    await page.reload({ waitUntil: "domcontentloaded" });
    await reopen();
    out.push(...(await axeAaViolations(page, scope)).map((v) => `[${scheme}] ${v}`));
  }
  return out;
}

/** Asserts a visible control's box is at least 44x44 CSS px (WCAG 2.5.5 / the repo's touch-target rule). */
export async function touchTargetProblems(...targets: import("@playwright/test").Locator[]): Promise<string[]> {
  const problems: string[] = [];
  for (const target of targets) {
    const box = await target.boundingBox();
    const name = await target.evaluate((el) => el.getAttribute("aria-label") ?? el.textContent?.trim() ?? el.tagName);
    if (!box) problems.push(`${name}: not visible`);
    else if (box.width < 43.5 || box.height < 43.5) problems.push(`${name}: ${Math.round(box.width)}x${Math.round(box.height)} is under 44px`);
  }
  return problems;
}

/** Asserts the mobile precondition shared by every *-mobile spec: coarse pointer, touch points and a mobile UA. */
export async function mobilePreconditionFacts(page: Page): Promise<{ coarse: boolean; touchPoints: number; ua: string }> {
  return page.evaluate(() => ({ coarse: matchMedia("(pointer: coarse)").matches, touchPoints: navigator.maxTouchPoints, ua: navigator.userAgent }));
}
