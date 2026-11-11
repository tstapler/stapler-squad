// @feature website-demos
import { describe, expect, test } from 'bun:test';
import { CaptionLog } from '../transcript';

describe('CaptionLog', () => {
  test('a caption at 4.2 s is logged as tMs 4200', () => {
    let now = 1_000;
    const log = new CaptionLog(() => now);
    now += 4_200;
    log.add('Approve the push');
    expect(log.toJSON()).toEqual([{ tMs: 4200, text: 'Approve the push' }]);
  });

  test('time before ready() is trimmed and the title card offset is added', () => {
    let now = 0;
    const log = new CaptionLog(() => now, 1500);
    now = 3_000;
    log.add('during page load');
    log.markReady();
    now = 5_000;
    log.add('after ready');
    expect(log.toJSON()).toEqual([{ tMs: 3500, text: 'after ready' }]);
    expect(log.elapsedMs()).toBe(3500);
  });
});
