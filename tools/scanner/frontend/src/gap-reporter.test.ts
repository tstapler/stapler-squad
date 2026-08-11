import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import { generateGapReport } from './gap-reporter';

function writeRegistryFile(dir: string, name: string, features: object[]): string {
  const filePath = path.join(dir, name);
  fs.writeFileSync(filePath, JSON.stringify({ version: '1', features }, null, 2));
  return filePath;
}

describe('generateGapReport', () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'gap-reporter-test-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('UT-2.2a: no gaps when all domains match', () => {
    const backend = writeRegistryFile(tmpDir, 'backend.json', [
      { id: 'session:create', type: 'backend' },
      { id: 'session:list', type: 'backend' },
      { id: 'session:delete', type: 'backend' },
    ]);
    const frontend = writeRegistryFile(tmpDir, 'frontend.json', [
      { id: 'ui:session-list', type: 'frontend' },
      { id: 'ui:session-modal', type: 'frontend' },
      { id: 'ui:session-detail', type: 'frontend' },
    ]);

    const gaps = generateGapReport(backend, frontend);
    expect(gaps.unmatchedBackend).toHaveLength(0);
  });

  it('UT-2.2b: all backend features unmatched when no frontend features', () => {
    const backend = writeRegistryFile(tmpDir, 'backend.json', [
      { id: 'session:create', type: 'backend' },
      { id: 'history:search', type: 'backend' },
      { id: 'workspace:switch', type: 'backend' },
    ]);
    const frontend = writeRegistryFile(tmpDir, 'frontend.json', []);

    const gaps = generateGapReport(backend, frontend);
    expect(gaps.unmatchedBackend.length).toBeGreaterThan(0);
  });

  it('UT-2.2c: all frontend features unmatched when no backend features', () => {
    const backend = writeRegistryFile(tmpDir, 'backend.json', []);
    const frontend = writeRegistryFile(tmpDir, 'frontend.json', [
      { id: 'ui:session-list', type: 'frontend' },
      { id: 'ui:history-panel', type: 'frontend' },
      { id: 'ui:workspace-switcher', type: 'frontend' },
    ]);

    const gaps = generateGapReport(backend, frontend);
    expect(gaps.unmatchedFrontend.length).toBeGreaterThan(0);
  });

  it('UT-2.2d: throws on malformed JSON input (missing features array)', () => {
    const badFile = path.join(tmpDir, 'bad.json');
    fs.writeFileSync(badFile, JSON.stringify({ version: '1', data: [] }));
    const frontend = writeRegistryFile(tmpDir, 'frontend.json', []);

    expect(() => generateGapReport(badFile, frontend)).toThrow();
  });
});
