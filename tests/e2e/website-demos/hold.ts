// @feature website-demos
// Pacing for recorded clips. A hold is a deliberate pause so a viewer can read the
// frame; state changes must wait on locators instead, never on a hold.
import type { Page } from '@playwright/test';

/** The one capability hold needs, so tests can pass a stub instead of a browser. */
export type Evaluator = Pick<Page, 'evaluate'>;

/** Pauses `ms` inside the page, so the recording keeps painting frames while it waits. */
export async function hold(page: Evaluator, ms: number): Promise<void> {
  if (!Number.isFinite(ms) || ms < 0) throw new RangeError(`hold(${ms}): ms must be a finite, non-negative number`);
  if (ms === 0) return;
  await page.evaluate((delay) => new Promise<void>((resolve) => setTimeout(resolve, delay)), ms);
}
