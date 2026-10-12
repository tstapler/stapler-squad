import { filterLines, readBufferLines, MAX_FILTER_RESULTS } from '../bufferFilter';

const LINES = ['build started', 'ERROR: disk full', 'warning: slow', 'Error again', ''];

describe('filterLines', () => {
  it('returns nothing for an empty query', () => {
    expect(filterLines(LINES, '')).toEqual({ matches: [], truncated: false });
  });

  it('matches case-insensitively by default and keeps 1-based line numbers', () => {
    const { matches } = filterLines(LINES, 'error');
    expect(matches).toEqual([
      { line: 2, text: 'ERROR: disk full' },
      { line: 4, text: 'Error again' },
    ]);
  });

  it('honors caseSensitive', () => {
    expect(filterLines(LINES, 'Error', { caseSensitive: true }).matches).toEqual([
      { line: 4, text: 'Error again' },
    ]);
  });

  it('supports regex and reports an invalid pattern instead of throwing', () => {
    expect(filterLines(LINES, '^(warning|build)', { regex: true }).matches.map((m) => m.line)).toEqual([1, 3]);

    const bad = filterLines(LINES, '(', { regex: true });
    expect(bad.matches).toEqual([]);
    expect(bad.error).toEqual(expect.any(String));
  });

  it('treats regex metacharacters literally when regex is off', () => {
    expect(filterLines(['a.b', 'axb'], 'a.b').matches.map((m) => m.line)).toEqual([1]);
  });

  it('keeps only the most recent matches past the cap', () => {
    const many = Array.from({ length: MAX_FILTER_RESULTS + 10 }, (_, i) => `hit ${i}`);
    const result = filterLines(many, 'hit');
    expect(result.truncated).toBe(true);
    expect(result.matches).toHaveLength(MAX_FILTER_RESULTS);
    expect(result.matches[result.matches.length - 1].text).toBe(`hit ${MAX_FILTER_RESULTS + 9}`);
  });
});

describe('readBufferLines', () => {
  it('reads every line as trimmed text, tolerating missing lines', () => {
    const rows: Array<string | undefined> = ['one', undefined, 'three'];
    const terminal = {
      buffer: {
        active: {
          length: rows.length,
          getLine: (i: number) => (rows[i] === undefined ? undefined : { translateToString: () => rows[i] as string }),
        },
      },
    };
    expect(readBufferLines(terminal)).toEqual(['one', '', 'three']);
  });

  it('returns [] without a terminal', () => {
    expect(readBufferLines(null)).toEqual([]);
  });
});
