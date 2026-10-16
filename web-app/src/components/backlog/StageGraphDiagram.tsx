"use client";

import { useMemo } from "react";
import type { BacklogStage, StageTransition } from "@/lib/hooks/useBacklogStages";
import { GATE_KIND_LABELS } from "@/lib/hooks/useBacklogStages";
import { visuallyHidden } from "@/styles/a11y.css";
import * as styles from "./StageGraphDiagram.css";

const COLUMN_WIDTH = 160;
const ROW_HEIGHT = 56;
const NODE_WIDTH = 128;
const NODE_HEIGHT = 32;
const MARGIN = 16;

interface LayoutNode {
  slug: string;
  name: string;
  x: number;
  y: number;
}

interface LayoutEdge {
  fromSlug: string;
  toSlug: string;
  fromName: string;
  toName: string;
  gateCount: number;
  gateKindLabels: string[];
}

/**
 * BFS-rank layered layout (Surface 3, research/ux.md §1/§3): each stage's
 * column is its shortest hop-distance from any `IsEntry` stage. Deliberately
 * not a general graph-drawing algorithm — no manual positioning, no drag,
 * nothing to lay out — so a stage unreachable from any entry stage still gets
 * a defined column (placed one past the furthest reached rank) rather than
 * being silently omitted.
 */
function computeLayout(stages: BacklogStage[], transitions: StageTransition[]): { nodes: LayoutNode[]; edges: LayoutEdge[] } {
  const bySlug = new Map(stages.map((s) => [s.slug, s]));
  const adjacency = new Map<string, string[]>();
  for (const s of stages) adjacency.set(s.slug, []);
  for (const t of transitions) {
    if (!bySlug.has(t.fromStageSlug) || !bySlug.has(t.toStageSlug)) continue;
    adjacency.get(t.fromStageSlug)?.push(t.toStageSlug);
  }

  const rank = new Map<string, number>();
  const entrySlugs = stages.filter((s) => s.isEntry).map((s) => s.slug);
  const queue: string[] = [...entrySlugs];
  for (const slug of entrySlugs) rank.set(slug, 0);
  while (queue.length > 0) {
    const current = queue.shift();
    if (current === undefined) break;
    const currentRank = rank.get(current) ?? 0;
    for (const next of adjacency.get(current) ?? []) {
      if (!rank.has(next)) {
        rank.set(next, currentRank + 1);
        queue.push(next);
      }
    }
  }
  const maxReachedRank = Math.max(0, ...Array.from(rank.values()));
  const unreachedRank = maxReachedRank + 1;
  for (const s of stages) {
    if (!rank.has(s.slug)) rank.set(s.slug, unreachedRank);
  }

  const columns = new Map<number, string[]>();
  for (const s of stages) {
    const r = rank.get(s.slug) ?? unreachedRank;
    const col = columns.get(r) ?? [];
    col.push(s.slug);
    columns.set(r, col);
  }

  const nodes: LayoutNode[] = [];
  for (const [col, slugs] of Array.from(columns.entries()).sort((a, b) => a[0] - b[0])) {
    slugs.forEach((slug, row) => {
      const stage = bySlug.get(slug);
      if (!stage) return;
      nodes.push({
        slug,
        name: stage.name,
        x: MARGIN + col * COLUMN_WIDTH,
        y: MARGIN + row * ROW_HEIGHT,
      });
    });
  }

  const edges: LayoutEdge[] = transitions
    .filter((t) => bySlug.has(t.fromStageSlug) && bySlug.has(t.toStageSlug))
    .map((t) => ({
      fromSlug: t.fromStageSlug,
      toSlug: t.toStageSlug,
      fromName: bySlug.get(t.fromStageSlug)?.name ?? t.fromStageSlug,
      toName: bySlug.get(t.toStageSlug)?.name ?? t.toStageSlug,
      gateCount: t.gates.length,
      gateKindLabels: t.gates.map((g) => GATE_KIND_LABELS[g.kind] ?? g.kind),
    }));

  return { nodes, edges };
}

export interface StageGraphDiagramProps {
  stages: BacklogStage[];
  transitions: StageTransition[];
}

/**
 * Surface 3's read-only generated graph diagram: computed inline SVG (no
 * graph/node-edge library — see `research/ux.md`'s build-vs-buy note) plus an
 * `sr-only` text-equivalent table, one row per edge, matching the SVG exactly.
 * The SVG is `aria-hidden` because the table — not an `aria-label` summary —
 * is the real text alternative (preserves per-edge gate-kind detail).
 */
export function StageGraphDiagram({ stages, transitions }: StageGraphDiagramProps) {
  const { nodes, edges } = useMemo(() => computeLayout(stages, transitions), [stages, transitions]);

  if (stages.length === 0) {
    return <p className={styles.empty}>No stages configured yet.</p>;
  }

  const maxX = Math.max(NODE_WIDTH, ...nodes.map((n) => n.x)) + NODE_WIDTH + MARGIN;
  const maxY = Math.max(NODE_HEIGHT, ...nodes.map((n) => n.y)) + NODE_HEIGHT + MARGIN;
  const nodeCenter = (slug: string): { x: number; y: number } | undefined => {
    const n = nodes.find((node) => node.slug === slug);
    if (!n) return undefined;
    return { x: n.x + NODE_WIDTH / 2, y: n.y + NODE_HEIGHT / 2 };
  };

  return (
    <figure className={styles.figure} aria-label="Workflow stage graph">
      <div className={styles.scrollArea}>
        <svg
          aria-hidden="true"
          width={maxX}
          height={maxY}
          viewBox={`0 0 ${maxX} ${maxY}`}
          data-testid="stage-graph-svg"
        >
          {edges.map((e, i) => {
            const from = nodeCenter(e.fromSlug);
            const to = nodeCenter(e.toSlug);
            if (!from || !to) return null;
            return (
              <g key={`edge-${i}`}>
                <line x1={from.x} y1={from.y} x2={to.x} y2={to.y} className={styles.edgeLine} />
                {e.gateCount > 0 && (
                  <text
                    x={(from.x + to.x) / 2}
                    y={(from.y + to.y) / 2 - 4}
                    className={styles.gateBadgeText}
                    textAnchor="middle"
                  >
                    {`\u{1F512}${e.gateCount}`}
                  </text>
                )}
              </g>
            );
          })}
          {nodes.map((n) => (
            <g key={n.slug}>
              <rect x={n.x} y={n.y} width={NODE_WIDTH} height={NODE_HEIGHT} rx={4} className={styles.nodeRect} />
              <text x={n.x + NODE_WIDTH / 2} y={n.y + NODE_HEIGHT / 2 + 4} textAnchor="middle" className={styles.nodeText}>
                {n.name}
              </text>
            </g>
          ))}
        </svg>
      </div>
      <span className={styles.caption}>{"\u{1F512}N badges mark an edge with N gate(s) attached"}</span>
      <table className={visuallyHidden} data-testid="stage-graph-sr-table">
        <caption>Stage transitions and gate counts</caption>
        <thead>
          <tr>
            <th>From</th>
            <th>To</th>
            <th>Gates</th>
          </tr>
        </thead>
        <tbody>
          {edges.map((e, i) => (
            <tr key={`row-${i}`}>
              <td>{e.fromName}</td>
              <td>{e.toName}</td>
              <td>
                {e.gateCount === 0 ? "0" : `${e.gateCount} (${e.gateKindLabels.join(", ")})`}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}
