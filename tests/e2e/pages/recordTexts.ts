import type { Locator } from "@playwright/test";

/**
 * Records every text `region` takes from now on and returns a reader of that log. Use it instead of toHaveText
 * on live regions that clear themselves after about 1s, which a stalled runner can miss.
 */
export async function recordTexts(region: Locator): Promise<() => Promise<string[]>> {
  const log = await region.evaluateHandle((el) => {
    const seen: string[] = [];
    new MutationObserver(() => seen.push(el.textContent ?? "")).observe(el, { childList: true, characterData: true, subtree: true });
    return seen;
  });
  return () => log.jsonValue();
}
