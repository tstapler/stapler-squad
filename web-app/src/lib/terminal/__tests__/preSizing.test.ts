import { readRenderedCellMetrics, computePreSize } from '../preSizing';

const terminalWithCell = (cell: unknown) =>
  ({ _core: { _renderService: { dimensions: { css: { cell } } } } }) as any;

describe('readRenderedCellMetrics', () => {
  it('reads finite cell width/height', () => {
    expect(readRenderedCellMetrics(terminalWithCell({ width: 8.4, height: 17 })))
      .toEqual({ cellWidth: 8.4, cellHeight: 17 });
  });

  it.each([
    ['NaN width', { width: NaN, height: 17 }],
    ['Infinity height', { width: 8, height: Infinity }],
    ['zero width', { width: 0, height: 17 }],
    ['missing cell', undefined],
  ])('returns null for %s', (_label, cell) => {
    expect(readRenderedCellMetrics(terminalWithCell(cell))).toBeNull();
  });

  it('returns null when the terminal or private API is absent', () => {
    expect(readRenderedCellMetrics(null)).toBeNull();
    expect(readRenderedCellMetrics({} as any)).toBeNull();
  });
});

describe('computePreSize', () => {
  const cell = { cellWidth: 8, cellHeight: 16 };

  it('floors so cols/rows never exceed what fits', () => {
    expect(computePreSize({ width: 1003, height: 805 }, cell)).toEqual({ ok: true, cols: 125, rows: 50 });
  });

  it('reports a zero-size container', () => {
    expect(computePreSize({ width: 0, height: 500 }, cell)).toEqual({ ok: false, reason: 'zero-size' });
  });

  it('reports below-minimum results with the computed size', () => {
    expect(computePreSize({ width: 100, height: 100 }, cell))
      .toEqual({ ok: false, reason: 'below-minimum', cols: 12, rows: 6 });
  });

  it('accepts exactly the minimum', () => {
    expect(computePreSize({ width: 30 * 8, height: 10 * 16 }, cell)).toEqual({ ok: true, cols: 30, rows: 10 });
  });
});
