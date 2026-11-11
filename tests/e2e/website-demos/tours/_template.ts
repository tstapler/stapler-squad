// @feature website-demos
// Copy to tours/<tour-id>.ts (file name = tour id = scene.name). Files starting with "_"
// are not scenes. Pair it with website/src/content/docs/tours/<tour-id>.mdx; no other file
// is edited to add a tour (scenes are found by glob, shards by `mediatool shard-plan`).
import { caption } from '../recorder';
import { hold } from '../hold';
import type { Scene } from '../scene';

export const scene: Scene = {
  name: 'tour-id',
  title: 'Title-card text',
  // flags: ['backlog'],        // feature flags this tour needs on
  // landing: { path: '/', landmark: '[data-testid="..."]' },
  // quotes: ['text a step quotes'],
  scenes: async (page, clip) => {
    await page.goto('/');
    await page.getByRole('main').first().waitFor();
    await clip.ready(); // asserts Grimdark, no open dialog, landing; trims page load
    await caption(page, 'One sentence per step');
    await hold(page, 800);
  },
  shots: [
    // { name: 'tour-id-1', route: '/', landmark: '[data-testid="..."]', annotations: [{ selector: '...', n: 1 }] },
    // { name: 'dashboard-mobile', route: '/', landmark: '...', formFactor: 'mobile' },
  ],
};
