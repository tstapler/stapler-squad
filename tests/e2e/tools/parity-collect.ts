/**
 * Browser-side collection for the parity harness: a bounded computed-style sample of the page,
 * the resolved :root custom-property map, and :hover / :focus-visible states on the first
 * focusable elements (forced through CDP, since Playwright cannot hold a pseudo-class).
 */
import type { BrowserContext, Page } from '@playwright/test';
import type { StyleSnapshot } from './parity-core.ts';

export const THEME_NAMES = ['matrix', 'cyberpunk77', 'wh40k', 'clean', 'light', 'dark'] as const;
export const FOCUS_SAMPLE = 5;
const ELEMENT_CAP = 300;
/** Data-driven surfaces that appear and disappear on their own; they cannot be compared across loads. */
const TRANSIENT_SURFACES = '[data-testid=memory-pressure-status],[data-testid=notification-tray],[data-testid^=toast]';

const PROPERTIES = [
  'color', 'background-color', 'font-family', 'font-size', 'font-weight', 'line-height', 'letter-spacing',
  'padding-top', 'padding-right', 'padding-bottom', 'padding-left',
  'margin-top', 'margin-right', 'margin-bottom', 'margin-left',
  'border-top-width', 'border-top-color', 'border-top-style', 'border-radius',
  'display', 'position', 'opacity', 'box-shadow', 'z-index',
  'outline-color', 'outline-style', 'outline-width', 'text-decoration-line', 'cursor',
];

/** Pre-load storage so the first paint uses `theme`. */
export async function primePage(page: Page | BrowserContext, theme: string): Promise<void> {
  await page.addInitScript((t) => {
    localStorage.setItem('stapler-theme', t);
    localStorage.setItem('stapler-squad:onboarded', 'true');
  }, theme);
}

/** In-page: stable structural selectors (no hashed class names) for the elements to sample. */
function pageSample(args: { properties: string[]; cap: number; focusSample: number; ignored: string }) {
  const ignored = args.ignored;
  const describe = (el: Element): string => {
    const parts: string[] = [];
    let node: Element | null = el;
    for (let depth = 0; node && node !== document.body && depth < 7; depth++) {
      // Ids minted per load (pane-leaf-1a2b3c4d, window-tab-...) would make selectors unrepeatable.
      const testId = node.getAttribute('data-testid')?.replace(/[0-9a-f]{8}(-[0-9a-f]{4}){0,4}|\d{5,}/g, '*');
      const role = node.getAttribute('role');
      let part = node.tagName.toLowerCase();
      if (testId) part += `[data-testid=${testId}]`;
      else if (role) part += `[role=${role}]`;
      else if (node.parentElement) {
        const same = Array.from(node.parentElement.children).filter((c) => c.tagName === node!.tagName);
        if (same.length > 1) part += `:nth-of-type(${same.indexOf(node) + 1})`;
      }
      parts.unshift(part);
      node = node.parentElement;
    }
    return parts.join('>');
  };

  const skip = new Set(['SCRIPT', 'STYLE', 'LINK', 'META', 'NOSCRIPT', 'TEMPLATE']);
  const elements = Array.from(document.body.querySelectorAll('*'))
    .filter((el) => !skip.has(el.tagName) && !el.closest('svg') && !el.closest(ignored))
    .slice(0, args.cap);

  const styles: Record<string, string> = {};
  for (const el of elements) {
    const computed = getComputedStyle(el);
    const selector = describe(el);
    for (const property of args.properties) {
      styles[`${selector}|base|${property}`] = computed.getPropertyValue(property);
    }
  }

  const focusable = Array.from(
    document.body.querySelectorAll('a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])'),
  )
    .filter((el) => (el as HTMLElement).offsetParent !== null)
    .slice(0, args.focusSample);
  focusable.forEach((el, i) => el.setAttribute('data-parity-focus', String(i)));

  const rootVars: Record<string, string> = {};
  const rootStyle = getComputedStyle(document.documentElement);
  const names = new Set<string>();
  for (const sheet of Array.from(document.styleSheets)) {
    let rules: CSSRuleList;
    try {
      rules = sheet.cssRules;
    } catch {
      continue;
    }
    const visit = (list: CSSRuleList) => {
      for (const rule of Array.from(list)) {
        const style = (rule as CSSStyleRule).style;
        if (style) for (const name of Array.from(style)) if (name.startsWith('--')) names.add(name);
        if ((rule as CSSGroupingRule).cssRules) visit((rule as CSSGroupingRule).cssRules);
      }
    };
    visit(rules);
  }
  Array.from(names).sort().forEach((name) => {
    rootVars[name] = rootStyle.getPropertyValue(name).trim();
  });
  return { styles, rootVars, focusCount: focusable.length };
}

async function stateStyles(page: Page, index: number, state: string): Promise<Record<string, string>> {
  return page.evaluate(
    ({ i, st, properties }) => {
      const el = document.querySelector(`[data-parity-focus="${i}"]`);
      if (!el) return {};
      const computed = getComputedStyle(el);
      const out: Record<string, string> = {};
      for (const property of properties) out[`focus-${i}|${st}|${property}`] = computed.getPropertyValue(property);
      return out;
    },
    { i: index, st: state, properties: PROPERTIES },
  );
}

export async function collectSnapshot(page: Page): Promise<StyleSnapshot> {
  const sample = await page.evaluate(pageSample, { properties: PROPERTIES, cap: ELEMENT_CAP, focusSample: FOCUS_SAMPLE, ignored: TRANSIENT_SURFACES });
  const styles: Record<string, string> = { ...sample.styles };
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('DOM.enable');
  await cdp.send('CSS.enable');
  const { root } = await cdp.send('DOM.getDocument', { depth: 0 });
  for (let i = 0; i < sample.focusCount; i++) {
    const { nodeId } = await cdp.send('DOM.querySelector', { nodeId: root.nodeId, selector: `[data-parity-focus="${i}"]` });
    if (!nodeId) continue;
    for (const state of ['hover', 'focus-visible']) {
      await cdp.send('CSS.forcePseudoState', { nodeId, forcedPseudoClasses: [state] });
      Object.assign(styles, await stateStyles(page, i, state));
    }
    await cdp.send('CSS.forcePseudoState', { nodeId, forcedPseudoClasses: [] });
  }
  await cdp.detach();
  return { rootVars: sample.rootVars, styles };
}
