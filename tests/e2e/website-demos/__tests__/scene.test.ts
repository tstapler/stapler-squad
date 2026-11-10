// @feature website-demos
import { describe, expect, test } from 'bun:test';
import * as fs from 'fs';
import * as path from 'path';
import { discoverScenes, sceneFiles, selectScenes, validateScene } from '../scene';
import { tmpDir } from './helpers';

const sceneSrc = (name: string) => `export const scene = { name: '${name}', title: 'T', scenes: async () => {} };\n`;

describe('scene discovery', () => {
  test('globs *.ts, skipping helpers, tests and declarations', () => {
    const dir = tmpDir();
    for (const f of ['b.ts', 'a.ts', '_template.ts', 'a.test.ts', 'x.d.ts', 'a.mdx']) fs.writeFileSync(path.join(dir, f), sceneSrc('x'));
    expect(sceneFiles(dir)).toEqual(['a.ts', 'b.ts']);
  });

  test('a scene file is picked up with no other file edited', () => {
    const dir = tmpDir();
    fs.writeFileSync(path.join(dir, 'zz-fixture.ts'), sceneSrc('zz-fixture'));
    expect(discoverScenes(dir).map((s) => s.name)).toEqual(['zz-fixture']);
  });

  test('the file name must equal scene.name', () => {
    expect(() => validateScene('a.ts', { scene: { name: 'b', title: 't', scenes: async () => {} } })).toThrow(/scene.name/);
    expect(() => validateScene('a.ts', {})).toThrow(/export `scene`/);
  });

  test('the real tours directory exists and its scenes validate', () => {
    const dir = path.resolve(__dirname, '../tours');
    expect(fs.existsSync(dir)).toBe(true);
    expect(() => discoverScenes(dir)).not.toThrow();
  });
});

describe('selectScenes', () => {
  const all = [{ name: 'a' }, { name: 'b' }, { name: 'c' }];
  test('empty selection keeps everything', () => expect(selectScenes(all, undefined)).toEqual(all));
  test('--only filters and unknown ids fail loudly', () => {
    expect(selectScenes(all, 'a, c')).toEqual([{ name: 'a' }, { name: 'c' }]);
    expect(() => selectScenes(all, 'a,nope')).toThrow(/nope/);
  });
});
