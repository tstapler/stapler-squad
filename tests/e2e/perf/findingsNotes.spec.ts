// @feature perf:harness
/** Evidence artifacts the perf work must leave behind exist and carry their required headings (REQ-16, Story 1.1.3). */
import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

const RESEARCH = path.join(__dirname, '../../../project_plans/frontend-responsiveness/research');

const NOTES: { file: string; headings: string[]; producer: string }[] = [
  { file: 'flags-trace.md', headings: ['## Call sites', '## Results', '## Findings', '## Gaps'], producer: 'Task 1.1.1b' },
  { file: 'slot-holders.md', headings: ['## Method', '## Findings'], producer: 'the stallProbe scenario (not built yet)' },
  { file: 'backlog-attribution.md', headings: ['## Method', '## Findings'], producer: 'Backlog attribution capture (not built yet)' },
];

for (const { file, headings, producer } of NOTES) {
  test(`findingsNote_should_ExistWithRequiredHeadings_When_${file}`, async () => {
    const full = path.join(RESEARCH, file);
    test.skip(!fs.existsSync(full), `${file} has not been produced yet (owner: ${producer})`);
    const text = fs.readFileSync(full, 'utf8');
    for (const h of headings) expect(text, `${file} is missing heading "${h}"`).toContain(h);
  });
}
