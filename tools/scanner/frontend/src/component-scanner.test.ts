import * as path from 'path';
import { scanFile, scanComponents } from './component-scanner';

const FIXTURES = path.join(__dirname, '__fixtures__');

describe('scanFile', () => {
  it('UT-2.1a: extracts feature ID from marked component in first 10 lines', () => {
    const result = scanFile(path.join(FIXTURES, 'marked-component.tsx'), process.cwd());
    expect(result).not.toBeNull();
    expect(result!.id).toBe('ui:session-list');
    expect(result!.type).toBe('frontend');
    expect(result!.frontend.component).toBe('SessionList');
  });

  it('UT-2.1b: returns null for unmarked component', () => {
    const result = scanFile(path.join(FIXTURES, 'unmarked-component.tsx'), process.cwd());
    expect(result).toBeNull();
  });

  it('UT-2.1c: excludes _pb.ts files regardless of content', () => {
    const result = scanFile(path.join(FIXTURES, 'generated_pb.ts'), process.cwd());
    expect(result).toBeNull();
  });

  it('UT-2.1d: excludes .test.tsx files', () => {
    const result = scanFile(path.join(FIXTURES, 'component.test.tsx'), process.cwd());
    expect(result).toBeNull();
  });

  it('UT-2.1f: does NOT detect marker on line 12 (beyond first 10)', () => {
    const result = scanFile(path.join(FIXTURES, 'marker-on-line-12.tsx'), process.cwd());
    expect(result).toBeNull();
  });
});

describe('scanComponents', () => {
  it('UT-2.1a (integration): finds only marked-component.tsx in fixtures dir', () => {
    const features = scanComponents(FIXTURES);
    // Only marked-component.tsx should be found
    // (generated_pb.ts, component.test.tsx, marker-on-line-12.tsx are excluded)
    expect(features).toHaveLength(1);
    expect(features[0].id).toBe('ui:session-list');
  });
});
