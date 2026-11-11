// @feature website-demos
import { describe, expect, test } from 'bun:test';
import * as fs from 'fs';
import * as path from 'path';
import { type MediaEntry, MANIFEST_FILE, failedEntry, mergeEntries, readParts, shardStart, writePart, writeShardManifest } from '../manifest';
import { tmpDir } from './helpers';

const row = (name: string, format: MediaEntry['format'], extra: Partial<MediaEntry> = {}): MediaEntry => ({ name, format, status: 'recorded', bytes: 1, commit: 'C', themeId: 'grimdark', ...extra });

describe('manifest', () => {
  test('mergeEntries sorts by name then format and rejects duplicates', () => {
    expect(mergeEntries([row('b', 'webm'), row('a', 'png'), row('a', 'mp4')]).map((e) => `${e.name}.${e.format}`)).toEqual(['a.mp4', 'a.png', 'b.webm']);
    expect(() => mergeEntries([row('a', 'webm'), row('a', 'webm')])).toThrow(/duplicate/);
  });

  test('parts from separate scenes merge into one shard manifest in the Go field shape', () => {
    const out = tmpDir();
    writePart(out, 'a', [row('a', 'webm', { attempts: 2, seeded: 'rpc', durationMs: 4200, formFactor: 'desktop' })]);
    writePart(out, 'b', [failedEntry('b', 'webm', 'C', 1)]);
    expect(readParts(out)).toHaveLength(2);
    const m = writeShardManifest(out, 'C', new Date('2026-11-10T00:00:00Z'), new Date('2026-11-10T00:05:00Z'));
    const onDisk = JSON.parse(fs.readFileSync(path.join(out, MANIFEST_FILE), 'utf8'));
    expect(onDisk).toEqual(JSON.parse(JSON.stringify(m)));
    expect(onDisk.version).toBe(1);
    expect(onDisk.entries.map((e: MediaEntry) => [e.name, e.status])).toEqual([['a', 'recorded'], ['b', 'failed']]);
    expect(onDisk.entries[0]).toMatchObject({ attempts: 2, seeded: 'rpc', durationMs: 4200, formFactor: 'desktop', themeId: 'grimdark' });
  });

  test('shardStart keeps the first stamp across calls', () => {
    const out = tmpDir();
    const first = shardStart(out);
    expect(shardStart(out).toISOString()).toBe(first.toISOString());
  });
});
