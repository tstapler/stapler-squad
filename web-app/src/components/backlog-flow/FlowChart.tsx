"use client";

import { Component, useId, useState, type ReactNode } from "react";
import * as styles from "./FlowChart.css";
import { DrillButton } from "./DrillButton";

export type FlowChartKind = "line" | "stacked-bar";

export interface FlowChartSeries {
  name: string;
  /** One value per category, in category order. */
  values: readonly number[];
}

export interface FlowChartProps {
  /** Visible figcaption naming the chart. */
  caption: string;
  /** The chart's one-sentence takeaway, read as its accessible name ("FAIL share fell from 41% to 28%"). */
  takeaway: string;
  categories: readonly string[];
  series: readonly FlowChartSeries[];
  kind: FlowChartKind;
  width?: number;
  height?: number;
  /** One drill key per category; with `onDrill`, that category's point and table row open its items. */
  drillKeys?: readonly string[];
  onDrill?: (key: string) => void;
}

interface Drill {
  keys: readonly string[];
  onDrill: (key: string) => void;
}

const DEFAULT_WIDTH = 240;
const DEFAULT_HEIGHT = 72;
const PAD = 4;
const MARKER_SIZE = 3;
const MARKERS = ["circle", "square", "triangle", "diamond"] as const;
const DASHES = [undefined, "5 3", "1.5 3", "7 3 1.5 3"] as const;
const PATTERNS = ["solid", "diagonal", "dots", "grid"] as const;
const TOKEN_SLOTS = 5;

type Marker = (typeof MARKERS)[number];
type PatternKind = (typeof PATTERNS)[number];

const markerFor = (index: number): Marker => MARKERS[index % MARKERS.length];
const patternFor = (index: number): PatternKind => PATTERNS[index % PATTERNS.length];
const slot = (index: number) => (index % TOKEN_SLOTS) as 0 | 1 | 2 | 3 | 4;

function hasData({ categories, series }: Pick<FlowChartProps, "categories" | "series">): boolean {
  return categories.length > 0 && series.some((s) => s.values.length > 0);
}

function MarkerShape({ marker, x, y, className }: { marker: Marker; x: number; y: number; className: string }) {
  const r = MARKER_SIZE;
  switch (marker) {
    case "circle":
      return <circle cx={x} cy={y} r={r} className={className} />;
    case "square":
      return <rect x={x - r} y={y - r} width={2 * r} height={2 * r} className={className} />;
    case "triangle":
      return <polygon points={`${x},${y - r - 1} ${x + r + 1},${y + r} ${x - r - 1},${y + r}`} className={className} />;
    case "diamond":
      return <polygon points={`${x},${y - r - 1} ${x + r + 1},${y} ${x},${y + r + 1} ${x - r - 1},${y}`} className={className} />;
  }
}

function PatternDefs({ prefix, indices }: { prefix: string; indices: readonly number[] }) {
  return (
    <defs>
      {indices.map((i) => {
        const pattern = patternFor(i);
        const base = styles.seriesFill[slot(i)];
        const line = styles.patternStroke[slot(i)];
        return (
          <pattern key={i} id={`${prefix}-p${i}`} patternUnits="userSpaceOnUse" width={6} height={6}>
            <rect width={6} height={6} className={base} fillOpacity={pattern === "solid" ? 1 : 0.3} />
            {pattern === "diagonal" && <path d="M0,6 L6,0" className={line} />}
            {pattern === "dots" && <circle cx={3} cy={3} r={1.2} className={styles.seriesFill[slot(i)]} />}
            {pattern === "grid" && <path d="M0,3 L6,3 M3,0 L3,6" className={line} />}
          </pattern>
        );
      })}
    </defs>
  );
}

function LinePlot({ categories, series, width, height, drill }: Required<Pick<FlowChartProps, "categories" | "series" | "width" | "height">> & { drill?: Drill }) {
  const max = Math.max(1, ...series.flatMap((s) => s.values));
  const step = categories.length > 1 ? (width - 2 * PAD) / (categories.length - 1) : 0;
  const x = (i: number) => (categories.length > 1 ? PAD + i * step : width / 2);
  const y = (v: number) => height - PAD - (v / max) * (height - 2 * PAD);
  return (
    <>
      <line x1={PAD} x2={width - PAD} y1={height - PAD} y2={height - PAD} className={styles.axis} />
      {series.map((s, i) => (
        <g key={s.name}>
          <polyline
            points={s.values.map((v, j) => `${x(j)},${y(v)}`).join(" ")}
            className={styles.seriesStroke[slot(i)]}
            strokeDasharray={DASHES[i % DASHES.length]}
          />
          {s.values.map((v, j) => {
            const key = drill?.keys[j] ?? "";
            const marker = <MarkerShape marker={markerFor(i)} x={x(j)} y={y(v)} className={styles.seriesFill[slot(i)]} />;
            if (drill === undefined || key === "") return <g key={j}>{marker}</g>;
            // Pointer-only: the svg is aria-hidden, keyboard and AT users drill from the data table rows.
            return (
              <g key={j} data-testid="flow-chart-point" className={styles.drillPoint} onClick={() => drill.onDrill(key)}>
                <circle cx={x(j)} cy={y(v)} r={MARKER_SIZE * 2} fill="transparent" />
                {marker}
              </g>
            );
          })}
        </g>
      ))}
    </>
  );
}

function StackedBars({ categories, series, width, height, prefix }: Required<Pick<FlowChartProps, "categories" | "series" | "width" | "height">> & { prefix: string }) {
  const totals = categories.map((_, c) => series.reduce((sum, s) => sum + (s.values[c] ?? 0), 0));
  const max = Math.max(1, ...totals);
  const slotWidth = (width - 2 * PAD) / categories.length;
  const barWidth = slotWidth * 0.7;
  const scale = (height - 2 * PAD) / max;
  return (
    <>
      <PatternDefs prefix={prefix} indices={series.map((_, i) => i)} />
      <line x1={PAD} x2={width - PAD} y1={height - PAD} y2={height - PAD} className={styles.axis} />
      {categories.map((category, c) => {
        let top = height - PAD;
        return (
          <g key={category}>
            {series.map((s, i) => {
              const h = (s.values[c] ?? 0) * scale;
              top -= h;
              return (
                <rect key={s.name} x={PAD + c * slotWidth + (slotWidth - barWidth) / 2} y={top} width={barWidth} height={h} fill={`url(#${prefix}-p${i})`} className={styles.bar} />
              );
            })}
          </g>
        );
      })}
    </>
  );
}

function Swatch({ kind, index, prefix }: { kind: FlowChartKind; index: number; prefix: string }) {
  return (
    <svg width={16} height={12} aria-hidden="true" data-encoding={kind === "line" ? markerFor(index) : patternFor(index)}>
      {kind === "line" ? (
        <MarkerShape marker={markerFor(index)} x={8} y={6} className={styles.seriesFill[slot(index)]} />
      ) : (
        <>
          <PatternDefs prefix={`${prefix}-l${index}`} indices={[index]} />
          <rect width={16} height={12} fill={`url(#${prefix}-l${index}-p${index})`} />
        </>
      )}
    </svg>
  );
}

function DataTable({ caption, categories, series, visible, drill }: Pick<FlowChartProps, "caption" | "categories" | "series"> & { visible: boolean; drill?: Drill }) {
  return (
    <table className={`${styles.table}${visible ? "" : ` ${styles.tableHidden}`}`} data-testid="flow-chart-table">
      <caption>{caption}</caption>
      <thead>
        <tr>
          <th scope="col" className={styles.cell}>
            Period
          </th>
          {series.map((s) => (
            <th key={s.name} scope="col" className={styles.cell}>
              {s.name}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {categories.map((category, c) => (
          <tr key={category}>
            <th scope="row" className={styles.cell}>
              {drill ? (
                <DrillButton drillKey={drill.keys[c] ?? ""} onDrill={drill.onDrill}>
                  {category}
                </DrillButton>
              ) : (
                category
              )}
            </th>
            {series.map((s) => (
              <td key={s.name} className={styles.cell}>
                {s.values[c] ?? 0}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** Shows `fallback` instead of the chart when drawing it throws. */
class ChartBoundary extends Component<{ fallback: ReactNode; children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}

function Plot({ kind, prefix, drill, ...rest }: Required<Pick<FlowChartProps, "categories" | "series" | "width" | "height" | "kind">> & { prefix: string; drill?: Drill }) {
  switch (kind) {
    case "line":
      return <LinePlot {...rest} drill={drill} />;
    case "stacked-bar":
      return <StackedBars {...rest} prefix={prefix} />;
    default:
      throw new Error(`unknown chart kind: ${String(kind)}`);
  }
}

/**
 * The one chart wrapper, so no chart can skip the accessibility rules: a figure with a caption,
 * a takeaway as the accessible name, series told apart by marker or pattern (not hue), and the
 * same values in a table that assistive tech always reaches. A drawing failure falls back to
 * the table alone.
 */
export function FlowChart({ caption, takeaway, categories, series, kind, width = DEFAULT_WIDTH, height = DEFAULT_HEIGHT, drillKeys, onDrill }: FlowChartProps) {
  const drill = drillKeys && onDrill ? { keys: drillKeys, onDrill } : undefined;
  const [tableShown, setTableShown] = useState(false);
  const prefix = useId().replace(/[^a-zA-Z0-9]/g, "");
  const tableId = `${prefix}-table`;

  if (!hasData({ categories, series })) {
    return (
      <figure className={styles.figure} data-testid="flow-chart">
        <figcaption className={styles.caption}>{caption}</figcaption>
        <p className={styles.empty}>No data in this window</p>
      </figure>
    );
  }

  const unavailable = (
    <>
      <p className={styles.note}>Chart unavailable, showing data</p>
      <DataTable caption={caption} categories={categories} series={series} visible drill={drill} />
    </>
  );

  return (
    <figure className={styles.figure} data-testid="flow-chart">
      <figcaption className={styles.caption}>{caption}</figcaption>
      <ChartBoundary fallback={unavailable}>
        <div role="img" aria-label={takeaway}>
          <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} className={styles.plot} aria-hidden="true">
            <Plot kind={kind} categories={categories} series={series} width={width} height={height} prefix={prefix} drill={drill} />
          </svg>
        </div>
        <ul className={styles.legend}>
          {series.map((s, i) => (
            <li key={s.name} className={styles.legendItem}>
              <Swatch kind={kind} index={i} prefix={prefix} />
              {s.name}
            </li>
          ))}
        </ul>
        <button type="button" className={styles.toggle} aria-expanded={tableShown} aria-controls={tableId} onClick={() => setTableShown((shown) => !shown)}>
          {tableShown ? "Hide data table" : "Show data table"}
        </button>
        <div id={tableId}>
          <DataTable caption={caption} categories={categories} series={series} visible={tableShown} drill={drill} />
        </div>
      </ChartBoundary>
    </figure>
  );
}
