import {
  getCachedDimensions,
  saveDimensions,
  validateCellDimensions,
  shouldOverwriteCache,
  getCurrentWindowDimensions,
  MIN_COLS,
  MIN_ROWS,
} from '../TerminalDimensionCache';

const KEY = 'terminal-dimensions-session-base';
const FONT = { size: 14, family: 'Menlo, monospace' };

beforeEach(() => {
  localStorage.clear();
  jest.spyOn(console, 'log').mockImplementation(() => {});
  jest.spyOn(console, 'warn').mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
  localStorage.clear();
});

describe('validateCellDimensions', () => {
  it('keeps cell metrics when the font is unchanged', () => {
    const result = validateCellDimensions(
      { cols: 100, rows: 30, cellWidth: 8, cellHeight: 17, fontSize: FONT.size, fontFamily: FONT.family },
      FONT.size,
      FONT.family,
    );
    expect(result).toEqual({
      cols: 100, rows: 30, cellWidth: 8, cellHeight: 17, fontSize: FONT.size, fontFamily: FONT.family,
    });
  });

  it('returns null when the font size changed', () => {
    const result = validateCellDimensions(
      { cols: 100, rows: 30, cellWidth: 8, cellHeight: 17, fontSize: 12, fontFamily: FONT.family },
      FONT.size,
      FONT.family,
    );
    expect(result).toBeNull();
  });

  it('returns null when the font family changed', () => {
    const result = validateCellDimensions(
      { cols: 100, rows: 30, cellWidth: 8, cellHeight: 17, fontSize: FONT.size, fontFamily: 'Courier' },
      FONT.size,
      FONT.family,
    );
    expect(result).toBeNull();
  });

  it('adopts the current font for a legacy entry that has cell metrics but no font fields', () => {
    const result = validateCellDimensions(
      { cols: 100, rows: 30, cellWidth: 8, cellHeight: 17 },
      FONT.size,
      FONT.family,
    );
    expect(result).toMatchObject({ cellWidth: 8, cellHeight: 17, fontSize: FONT.size, fontFamily: FONT.family });
  });

  it('returns only cols/rows when there are no cell metrics, and does not mutate the input', () => {
    const cached = { cols: 100, rows: 30 };
    expect(validateCellDimensions(cached, FONT.size, FONT.family)).toEqual({ cols: 100, rows: 30 });
    expect(cached).toEqual({ cols: 100, rows: 30 });
  });
});

describe('shouldOverwriteCache', () => {
  it('allows a write when nothing is cached', () => {
    expect(shouldOverwriteCache(KEY, 120, 40)).toBe(true);
  });

  it('refuses dimensions that are not larger than the cached ones', () => {
    localStorage.setItem(KEY, JSON.stringify({ cols: 120, rows: 40 }));
    expect(shouldOverwriteCache(KEY, 120, 40)).toBe(false);
    expect(shouldOverwriteCache(KEY, 100, 30)).toBe(false);
  });

  it('allows larger dimensions over an entry without cell metrics', () => {
    localStorage.setItem(KEY, JSON.stringify({ cols: 100, rows: 30 }));
    expect(shouldOverwriteCache(KEY, 120, 40)).toBe(true);
  });

  it('refuses larger dimensions that would drop cached cell metrics', () => {
    localStorage.setItem(KEY, JSON.stringify({ cols: 100, rows: 30, cellWidth: 8, cellHeight: 17 }));
    expect(shouldOverwriteCache(KEY, 120, 40)).toBe(false);
    expect(shouldOverwriteCache(KEY, 120, 40, { cellWidth: 8, cellHeight: 17 })).toBe(true);
  });

  it('refuses to overwrite an unparseable entry', () => {
    localStorage.setItem(KEY, 'not-json');
    expect(shouldOverwriteCache(KEY, 120, 40)).toBe(false);
  });
});

describe('getCurrentWindowDimensions', () => {
  it('never returns less than the minimum cols/rows', () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 40 });
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 40 });

    expect(getCurrentWindowDimensions()).toEqual({ cols: MIN_COLS, rows: MIN_ROWS });
  });
});

describe('storage failures', () => {
  it('does not throw when setItem throws (quota exceeded)', () => {
    jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('QuotaExceededError');
    });

    expect(() => saveDimensions('session-base', 120, 40)).not.toThrow();
    expect(console.warn).toHaveBeenCalled();
  });

  it('returns null when getItem throws', () => {
    jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError');
    });

    expect(getCachedDimensions('session-base')).toBeNull();
    expect(console.warn).toHaveBeenCalled();
  });
});
