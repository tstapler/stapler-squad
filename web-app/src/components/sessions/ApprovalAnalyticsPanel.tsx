"use client";

import React, { useState, useCallback, useMemo } from "react";
import { buildPrefillHref } from "@/lib/ruleBuilderPrefill";
import { useApprovalAnalytics } from "@/lib/hooks/useApprovalAnalytics";
import { useApprovalRules } from "@/lib/hooks/useApprovalRules";
import { useGenerateRule } from "@/lib/hooks/useGenerateRule";
import { DailyBucketProto, SubcommandStatProto, AutoDecision, SuggestionSource } from "@/gen/session/v1/types_pb";
import { ProgramDetailPanel } from "./ProgramDetailPanel";
import { SuggestedRuleCard } from "./SuggestedRuleCard";
import {
  panel, titleRow, title, refreshButton,
  windowSelector, windowBtn, windowBtnActive,
  error as errorClass, retryButton,
  cards, card, cardAllow, cardDeny, cardManual, cardValue, cardLabel, cardSub,
  loading as loadingClass, empty, emptyHint,
  sectionTitle, tableSection, tableWrapper, table, th, thRight, td, tdRight, tdBar, row,
  allowCount, denyCount, manualCount, pctLabel, toolName, ruleName,
  barTrack, barFill, barTool, barRule, barCmd, barPython, barGap,
  categoryBadge, filterInput, addRuleLink,
  twoColGrid, twoColCell,
  stackedBarTrack, stackedAllow, stackedDeny, stackedManual,
  gapBadgeHigh, gapBadgeMed, gapBadgeLow, gapBadgeDesc,
  checkboxTh, checkboxTd,
  bulkActionBar, bulkActionCount, bulkAddBtn, bulkClearBtn,
  bulkReviewPanel, bulkReviewHeader, bulkReviewActions, bulkSaveBtn, bulkDiscardBtn,
  bulkResultMsg, decisionSelect, removeEntryBtn,
} from "./ApprovalAnalyticsPanel.css";

// ── types ─────────────────────────────────────────────────────────────────────

type BulkEntry = {
  key: string;
  program: string;
  subcommand: string;
  decision: AutoDecision;
};

type BulkUpsertFn = (rules: Array<{ id: string; name: string; programs: string[]; subcommands: string[]; decision: AutoDecision }>) => Promise<{ created: number; updated: number; errors: string[] }>;

// ── helpers ───────────────────────────────────────────────────────────────────

function pct(count: number, total: number): number {
  if (total === 0) return 0;
  return Math.round((count / total) * 100);
}

function formatDate(iso: string): string {
  // "2006-01-02" → "Jan 15"
  try {
    const d = new Date(iso + "T00:00:00");
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  } catch {
    return iso;
  }
}

// Simple inline bar component — no charting library required.
function Bar({ value, max, className }: { value: number; max: number; className: string }) {
  const width = max === 0 ? 0 : Math.round((value / max) * 100);
  return (
    <div className={barTrack} aria-hidden="true">
      <div className={`${barFill} ${className}`} style={{ width: `${width}%` }} />
    </div>
  );
}

// Stacked bar showing allow/deny/manual composition scaled to max total.
function StackedBar({ allow, deny, manual, total }: { allow: number; deny: number; manual: number; total: number }) {
  const rowTotal = allow + deny + manual;
  const scale = total === 0 ? 0 : rowTotal / total;
  const ap = Math.round(scale * (allow / (rowTotal || 1)) * 100);
  const dp = Math.round(scale * (deny  / (rowTotal || 1)) * 100);
  const mp = Math.round(scale * (manual / (rowTotal || 1)) * 100);
  return (
    <div className={stackedBarTrack} aria-hidden="true">
      {ap > 0 && <div className={stackedAllow} style={{ width: `${ap}%` }} />}
      {dp > 0 && <div className={stackedDeny}  style={{ width: `${dp}%` }} />}
      {mp > 0 && <div className={stackedManual} style={{ width: `${mp}%` }} />}
    </div>
  );
}

// ── component ─────────────────────────────────────────────────────────────────

const WINDOW_OPTIONS = [
  { label: "7 days",  value: 7  },
  { label: "14 days", value: 14 },
  { label: "30 days", value: 30 },
  { label: "90 days", value: 90 },
];

/**
 * ApprovalAnalyticsPanel displays time-series and aggregate data for
 * auto-approval classification decisions.
 *
 * Shows:
 * - Window selector (7 / 14 / 30 / 90 days)
 * - Summary cards: total, auto-allow rate, manual review rate, avg/day
 * - Day-by-day breakdown table with inline bar charts
 * - Top tools and top triggered rules
 */
export function ApprovalAnalyticsPanel() {
  const [windowDays, setWindowDays] = useState(7);
  const [selectedProgram, setSelectedProgram] = useState<string | null>(null);
  const { summary, dailyBuckets, loading, error, refresh } = useApprovalAnalytics({ windowDays });
  const { bulkUpsertRules } = useApprovalRules();

  const total = summary?.totalDecisions ?? 0;
  const autoAllowCount = summary?.decisionCounts["auto_allow"] ?? 0;
  const autoDenyCount  = summary?.decisionCounts["auto_deny"]  ?? 0;
  const escalateCount  = (summary?.decisionCounts["escalate"] ?? 0)
                       + (summary?.decisionCounts["manual_allow"] ?? 0)
                       + (summary?.decisionCounts["manual_deny"] ?? 0);

  const autoAllowRate = pct(autoAllowCount, total);
  const autoDenyRate  = pct(autoDenyCount, total);
  const manualRate    = pct(escalateCount, total);
  const avgPerDay     = dailyBuckets.length > 0 ? Math.round(total / windowDays) : 0;
  const manualAllow   = summary?.decisionCounts["manual_allow"] ?? 0;
  const manualDeny    = summary?.decisionCounts["manual_deny"]  ?? 0;
  const manualTotal   = manualAllow + manualDeny;
  const manualAllowPct = manualTotal > 0 ? Math.round((manualAllow / manualTotal) * 100) : null;

  // Max total across days — used to scale inline bars.
  const maxDayTotal = dailyBuckets.reduce((m, b) => Math.max(m, b.total), 0);

  return (
    <div className={panel}>
      {/* ── Header + window selector ── */}
      <div className={titleRow}>
        <h2 className={title}>Approval Analytics</h2>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <div className={windowSelector} role="group" aria-label="Time window">
            {WINDOW_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                className={`${windowBtn} ${windowDays === opt.value ? windowBtnActive : ""}`}
                onClick={() => setWindowDays(opt.value)}
                aria-pressed={windowDays === opt.value}
              >
                {opt.label}
              </button>
            ))}
          </div>
          <button onClick={refresh} className={refreshButton} disabled={loading} aria-label="Refresh analytics">
            {loading ? "⟳" : "↻"}
          </button>
        </div>
      </div>

      {error && (
        <div className={errorClass} role="alert">
          Failed to load analytics: {error.message}
          <button onClick={refresh} className={retryButton}>Retry</button>
        </div>
      )}

      {/* ── Summary cards ── */}
      <div className={cards}>
        <div className={card}>
          <span className={cardValue}>{total}</span>
          <span className={cardLabel}>Total decisions</span>
          <span className={cardSub}>{avgPerDay}/day avg</span>
        </div>
        <div className={`${card} ${cardAllow}`}>
          <span className={cardValue}>{autoAllowRate}%</span>
          <span className={cardLabel}>Auto-allowed</span>
          <span className={cardSub}>{autoAllowCount} requests</span>
        </div>
        <div className={`${card} ${cardDeny}`}>
          <span className={cardValue}>{autoDenyRate}%</span>
          <span className={cardLabel}>Auto-denied</span>
          <span className={cardSub}>{autoDenyCount} requests</span>
        </div>
        <div className={`${card} ${cardManual}`}>
          <span className={cardValue}>{manualRate}%</span>
          <span className={cardLabel}>Manual review</span>
          <span className={cardSub}>
            {escalateCount} requests
            {manualAllowPct !== null && ` · ${manualAllowPct}% allowed`}
          </span>
        </div>
      </div>

      {/* ── Daily breakdown ── */}
      {loading && dailyBuckets.length === 0 ? (
        <div className={loadingClass}>Loading analytics…</div>
      ) : dailyBuckets.length === 0 ? (
        <div className={empty}>
          No data for the last {windowDays} days.
          <br />
          <span className={emptyHint}>Analytics are recorded when Claude Code sends hook requests.</span>
        </div>
      ) : (
        <div className={tableSection}>
          <h3 className={sectionTitle}>Daily Breakdown</h3>
          <div className={tableWrapper}>
            <table className={table}>
              <thead>
                <tr>
                  <th className={th}>Date</th>
                  <th className={`${th} ${thRight}`}>Total</th>
                  <th className={`${th} ${thRight}`}>Allow</th>
                  <th className={`${th} ${thRight}`}>Deny</th>
                  <th className={`${th} ${thRight}`}>Manual</th>
                  <th className={th}>Volume</th>
                </tr>
              </thead>
              <tbody>
                {[...dailyBuckets].reverse().map((b) => {
                  const manualTotal = b.escalate + b.manualAllow + b.manualDeny;
                  return (
                    <tr key={b.date} className={row}>
                      <td className={td}>{formatDate(b.date)}</td>
                      <td className={`${td} ${tdRight}`}>{b.total}</td>
                      <td className={`${td} ${tdRight}`}>
                        <span className={allowCount}>{b.autoAllow}</span>
                        <span className={pctLabel}> {pct(b.autoAllow, b.total)}%</span>
                      </td>
                      <td className={`${td} ${tdRight}`}>
                        <span className={denyCount}>{b.autoDeny}</span>
                        <span className={pctLabel}> {pct(b.autoDeny, b.total)}%</span>
                      </td>
                      <td className={`${td} ${tdRight}`}>
                        <span className={manualCount}>{manualTotal}</span>
                        <span className={pctLabel}> {pct(manualTotal, b.total)}%</span>
                      </td>
                      <td className={`${td} ${tdBar}`}>
                        <StackedBar
                          allow={b.autoAllow}
                          deny={b.autoDeny}
                          manual={manualTotal}
                          total={maxDayTotal}
                        />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Top tools + Top triggered rules (side-by-side) ── */}
      {summary && (summary.topTools.length > 0 || summary.topTriggeredRules.length > 0) && (
        <div className={twoColGrid}>
          {summary.topTools.length > 0 && (
            <div className={`${tableSection} ${twoColCell}`}>
              <h3 className={sectionTitle}>Top Tools</h3>
              <div className={tableWrapper}>
                <table className={table}>
                  <thead>
                    <tr>
                      <th className={th}>Tool</th>
                      <th className={`${th} ${thRight}`}>Requests</th>
                      <th className={th}>Share</th>
                    </tr>
                  </thead>
                  <tbody>
                    {summary.topTools.map((t) => (
                      <tr key={t.toolName} className={row}>
                        <td className={td}><code className={toolName}>{t.toolName}</code></td>
                        <td className={`${td} ${tdRight}`}>{t.count}</td>
                        <td className={`${td} ${tdBar}`}>
                          <Bar value={t.count} max={summary.topTools[0]?.count ?? 1} className={barTool} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
          {summary.topTriggeredRules.length > 0 && (
            <div className={`${tableSection} ${twoColCell}`}>
              <h3 className={sectionTitle}>Top Triggered Rules</h3>
              <div className={tableWrapper}>
                <table className={table}>
                  <thead>
                    <tr>
                      <th className={th}>Rule</th>
                      <th className={`${th} ${thRight}`}>Triggers</th>
                      <th className={th}>Frequency</th>
                    </tr>
                  </thead>
                  <tbody>
                    {summary.topTriggeredRules.map((r) => (
                      <tr key={r.ruleId} className={row}>
                        <td className={td}><span className={ruleName}>{r.ruleName || r.ruleId}</span></td>
                        <td className={`${td} ${tdRight}`}>{r.count}</td>
                        <td className={`${td} ${tdBar}`}>
                          <Bar value={r.count} max={summary.topTriggeredRules[0]?.count ?? 1} className={barRule} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}

      {/* ── Top Python imports ── */}
      {summary && summary.topPythonImports.length > 0 && (
        <div className={tableSection}>
          <h3 className={sectionTitle}>Top Python Imports</h3>
          <div className={tableWrapper}>
            <table className={table}>
              <thead>
                <tr>
                  <th className={th}>Module</th>
                  <th className={`${th} ${thRight}`}>Uses</th>
                  <th className={th}>Share</th>
                </tr>
              </thead>
              <tbody>
                {summary.topPythonImports.map((imp) => (
                  <tr key={imp.module} className={row}>
                    <td className={td}><code className={toolName}>{imp.module}</code></td>
                    <td className={`${td} ${tdRight}`}>{imp.count}</td>
                    <td className={`${td} ${tdBar}`}>
                      <Bar value={imp.count} max={summary.topPythonImports[0]?.count ?? 1} className={barPython} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Unified activity table ── */}
      {summary && (summary.commandSubcommandStats.length > 0 || summary.topTools.length > 0 || summary.topUncoveredTools.length > 0) && (
        <div className={tableSection}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 4, marginBottom: 12, flexWrap: "wrap" }}>
            <h3 className={sectionTitle} style={{ margin: 0 }}>Activity</h3>
            {summary.coverageGapCount > 0 && (() => {
              const rounded = Math.round(summary.coverageGapRate);
              const cls = rounded >= 30 ? gapBadgeHigh : rounded >= 10 ? gapBadgeMed : gapBadgeLow;
              return (
                <>
                  <span className={cls}>{rounded}% uncovered</span>
                  <span className={gapBadgeDesc}>{summary.coverageGapCount} of {total} decisions had no matching rule</span>
                </>
              );
            })()}
          </div>
          <UnifiedActivityTable
            commandStats={summary.commandSubcommandStats}
            uncoveredToolNames={new Set(summary.topUncoveredTools.map((t) => t.toolName))}
            uncoveredProgramNames={new Set(summary.topUncoveredPrograms.map((p) => p.programName))}
            allToolNames={summary.topTools}
            windowDays={windowDays}
            selectedProgram={selectedProgram}
            onSelectProgram={setSelectedProgram}
            bulkUpsert={bulkUpsertRules}
            onRuleAccepted={refresh}
          />
        </div>
      )}
    </div>
  );
}

// ── BulkReviewPanel ───────────────────────────────────────────────────────────

function BulkReviewPanel({
  entries,
  onEntriesChange,
  bulkUpsert,
  onDone,
}: {
  entries: BulkEntry[];
  onEntriesChange: (entries: BulkEntry[]) => void;
  bulkUpsert: BulkUpsertFn;
  onDone: () => void;
}) {
  const [saving, setSaving] = useState(false);
  const [result, setResult] = useState("");

  const setDecision = useCallback((key: string, decision: AutoDecision) => {
    onEntriesChange(entries.map((e) => (e.key === key ? { ...e, decision } : e)));
  }, [entries, onEntriesChange]);

  const removeEntry = useCallback((key: string) => {
    const next = entries.filter((e) => e.key !== key);
    if (next.length === 0) { onDone(); return; }
    onEntriesChange(next);
  }, [entries, onEntriesChange, onDone]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    setResult("");
    try {
      const rules = entries.map((e) => ({
        id: "",
        name: e.subcommand ? `${e.program} ${e.subcommand}` : e.program,
        programs: [e.program],
        subcommands: e.subcommand ? [e.subcommand] : [],
        decision: e.decision,
      }));
      const resp = await bulkUpsert(rules);
      setResult(`✓ Created ${resp.created}, updated ${resp.updated}`);
    } catch (err) {
      setResult(`Error: ${err instanceof Error ? err.message : "unknown error"}`);
    } finally {
      setSaving(false);
    }
  }, [entries, bulkUpsert, onDone]);

  return (
    <div className={bulkReviewPanel}>
      <div className={bulkReviewHeader}>
        <span>Review {entries.length} new rule{entries.length !== 1 ? "s" : ""}</span>
        <div className={bulkReviewActions}>
          {result && <span className={bulkResultMsg} role="status">{result}</span>}
          {result ? (
            <button className={bulkSaveBtn} onClick={onDone}>Done</button>
          ) : (
            <button className={bulkSaveBtn} onClick={handleSave} disabled={saving}>
              {saving ? "Saving…" : "Save all"}
            </button>
          )}
          {!result && <button className={bulkDiscardBtn} onClick={onDone} disabled={saving}>Cancel</button>}
        </div>
      </div>
      <div className={tableWrapper}>
        <table className={table}>
          <thead>
            <tr>
              <th className={th}>Program</th>
              <th className={th}>Subcommand</th>
              <th className={th}>Decision</th>
              <th className={th}></th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e) => (
              <tr key={e.key} className={row}>
                <td className={td}><code className={toolName}>{e.program}</code></td>
                <td className={td}>{e.subcommand ? <code className={toolName}>{e.subcommand}</code> : <span style={{ color: "var(--text-muted)" }}>any</span>}</td>
                <td className={td}>
                  <select
                    className={decisionSelect}
                    value={e.decision}
                    onChange={(ev) => setDecision(e.key, Number(ev.target.value) as AutoDecision)}
                    aria-label={`Decision for ${e.program} ${e.subcommand}`}
                  >
                    <option value={AutoDecision.ALLOW}>Allow</option>
                    <option value={AutoDecision.DENY}>Deny</option>
                    <option value={AutoDecision.ESCALATE}>Escalate (manual)</option>
                  </select>
                </td>
                <td className={td}>
                  <button className={removeEntryBtn} onClick={() => removeEntry(e.key)} aria-label="Remove">✕</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ── UnifiedActivityTable ──────────────────────────────────────────────────────
//
// Replaces the three separate tables (CommandDistribution, UncoveredTools,
// UncoveredPrograms) with a single filterable table.

type ActivityFilter = "all" | "needs-rule" | "has-manual";

type UnifiedRow = {
  key: string;
  type: "tool" | "command";
  name: string;        // tool name, or program
  subcommand: string;  // empty for tools
  category: string;
  count: number;
  manualCount: number;
  isUncovered: boolean;
  // for drill-down (command rows only)
  programName?: string;
  addRuleHref: string;
  bulkEntry: BulkEntry;
};

const FILTER_LABELS: Record<ActivityFilter, string> = {
  all: "All",
  "needs-rule": "Needs rule",
  "has-manual": "Has manual",
};

function UnifiedActivityTable({
  commandStats,
  uncoveredToolNames,
  uncoveredProgramNames,
  allToolNames,
  windowDays,
  selectedProgram,
  onSelectProgram,
  bulkUpsert,
  onRuleAccepted,
}: {
  commandStats: SubcommandStatProto[];
  uncoveredToolNames: Set<string>;
  uncoveredProgramNames: Set<string>;
  allToolNames: Array<{ toolName: string; count: number; manualAllow?: number; manualDeny?: number }>;
  windowDays: number;
  selectedProgram: string | null;
  onSelectProgram: (p: string | null) => void;
  bulkUpsert: BulkUpsertFn;
  onRuleAccepted?: () => void;
}) {
  const [filter, setFilter] = useState<ActivityFilter>("all");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [reviewEntries, setReviewEntries] = useState<BulkEntry[] | null>(null);
  const [activeRowKey, setActiveRowKey] = useState<string | null>(null);
  const { suggestions, loading: genLoading, error: genError, generate, clear: clearSuggestion } = useGenerateRule();

  // Build unified rows from both tools and commands
  const allRows: UnifiedRow[] = React.useMemo(() => {
    const rows: UnifiedRow[] = [];

    // Tool rows — all tools from topTools, mark uncovered ones
    for (const t of allToolNames) {
      rows.push({
        key: `tool:${t.toolName}`,
        type: "tool",
        name: t.toolName,
        subcommand: "",
        category: "tool",
        count: t.count,
        manualCount: (t.manualAllow ?? 0) + (t.manualDeny ?? 0),
        isUncovered: uncoveredToolNames.has(t.toolName),
        addRuleHref: buildPrefillHref({ toolName: t.toolName }),
        bulkEntry: { key: `tool:${t.toolName}`, program: t.toolName, subcommand: "", decision: AutoDecision.ALLOW },
      });
    }
    // Add uncovered tools not already present in topTools
    for (const name of uncoveredToolNames) {
      if (!allToolNames.find((t) => t.toolName === name)) {
        rows.push({
          key: `tool:${name}`, type: "tool", name, subcommand: "", category: "tool",
          count: 0, manualCount: 0, isUncovered: true,
          addRuleHref: buildPrefillHref({ toolName: name }),
          bulkEntry: { key: `tool:${name}`, program: name, subcommand: "", decision: AutoDecision.ALLOW },
        });
      }
    }

    // Command rows from commandSubcommandStats
    const seenPrograms = new Set<string>();
    for (const s of commandStats) {
      const manualCount = s.manualAllow + s.manualDeny;
      seenPrograms.add(s.programName);
      rows.push({
        key: `cmd:${s.programName}:${s.subcommand}`,
        type: "command",
        name: s.programName,
        subcommand: s.subcommand,
        category: s.category,
        count: s.count,
        manualCount,
        isUncovered: uncoveredProgramNames.has(s.programName),
        programName: s.programName,
        addRuleHref: buildPrefillHref({ programs: [s.programName], subcommands: s.subcommand ? [s.subcommand] : [] }),
        bulkEntry: { key: `cmd:${s.programName}:${s.subcommand}`, program: s.programName, subcommand: s.subcommand, decision: AutoDecision.ALLOW },
      });
    }
    // Also show uncovered programs not already present in commandStats
    for (const p of uncoveredProgramNames) {
      if (!seenPrograms.has(p)) {
        rows.push({
          key: `cmd:${p}:`,
          type: "command",
          name: p,
          subcommand: "",
          category: "",
          count: 0,
          manualCount: 0,
          isUncovered: true,
          programName: p,
          addRuleHref: buildPrefillHref({ programs: [p] }),
          bulkEntry: { key: `cmd:${p}:`, program: p, subcommand: "", decision: AutoDecision.ALLOW },
        });
      }
    }

    return rows;
  }, [commandStats, uncoveredToolNames, uncoveredProgramNames, allToolNames]);

  const filteredRows = React.useMemo(() => {
    let rows = allRows;
    if (filter === "needs-rule") rows = rows.filter((r) => r.isUncovered);
    if (filter === "has-manual") rows = rows.filter((r) => r.manualCount > 0);
    if (search) {
      const lc = search.toLowerCase();
      rows = rows.filter((r) =>
        r.name.toLowerCase().includes(lc) || r.subcommand.toLowerCase().includes(lc)
      );
    }
    return rows;
  }, [allRows, filter, search]);

  const maxCount = filteredRows.reduce((m, r) => Math.max(m, r.count), 1);

  const allFilteredKeys = filteredRows.map((r) => r.key);
  const allChecked = allFilteredKeys.length > 0 && allFilteredKeys.every((k) => selected.has(k));

  const toggleAll = () => {
    if (allChecked) {
      setSelected((prev) => { const next = new Set(prev); allFilteredKeys.forEach((k) => next.delete(k)); return next; });
    } else {
      setSelected((prev) => new Set([...prev, ...allFilteredKeys]));
    }
  };

  const toggleRow = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const openReview = () => {
    const entries = allRows
      .filter((r) => selected.has(r.key))
      .map((r) => ({ ...r.bulkEntry, decision: AutoDecision.ALLOW }));
    setReviewEntries(entries);
  };

  const closeReview = () => { setReviewEntries(null); setSelected(new Set()); };

  // Counts for filter badges
  const needsRuleCount = allRows.filter((r) => r.isUncovered).length;
  const hasManualCount = allRows.filter((r) => r.manualCount > 0).length;

  return (
    <>
      {/* Filter chips + search */}
      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 8 }}>
        <div className={windowSelector} role="group" aria-label="Activity filter">
          {(["all", "needs-rule", "has-manual"] as ActivityFilter[]).map((f) => {
            const badge = f === "needs-rule" ? needsRuleCount : f === "has-manual" ? hasManualCount : allRows.length;
            return (
              <button
                key={f}
                className={`${windowBtn} ${filter === f ? windowBtnActive : ""}`}
                onClick={() => setFilter(f)}
                aria-pressed={filter === f}
              >
                {FILTER_LABELS[f]}
                {badge > 0 && <span style={{ marginLeft: 4, opacity: 0.7, fontSize: "0.85em" }}>({badge})</span>}
              </button>
            );
          })}
        </div>
        <input
          type="text"
          placeholder="Search…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className={filterInput}
          style={{ flex: 1, minWidth: 120 }}
          aria-label="Search activity"
        />
      </div>

      {selected.size > 0 && !reviewEntries && (
        <div className={bulkActionBar}>
          <span className={bulkActionCount}>{selected.size} selected</span>
          <button className={bulkAddBtn} onClick={openReview}>Add rules for selected →</button>
          <button className={bulkClearBtn} onClick={() => setSelected(new Set())}>Clear</button>
        </div>
      )}
      {reviewEntries && (
        <BulkReviewPanel entries={reviewEntries} onEntriesChange={setReviewEntries} bulkUpsert={bulkUpsert} onDone={closeReview} />
      )}

      {filteredRows.length === 0 ? (
        <p style={{ color: "var(--text-muted)", fontSize: "0.85em", padding: "12px 0" }}>
          {filter === "needs-rule" ? "No uncovered executions — all activity has a matching rule." :
           filter === "has-manual" ? "No manual reviews in this window." : "No activity."}
        </p>
      ) : (
        <div className={tableWrapper}>
          <table className={table}>
            <thead>
              <tr>
                <th className={checkboxTh}>
                  <input type="checkbox" checked={allChecked} onChange={toggleAll} aria-label="Select all" />
                </th>
                <th className={th}>Name</th>
                <th className={th}>Type</th>
                <th className={th}>Category</th>
                <th className={`${th} ${thRight}`}>Count</th>
                <th className={`${th} ${thRight}`}>Manual</th>
                <th className={th}>Volume</th>
                <th className={th}></th>
              </tr>
            </thead>
            <tbody>
              {filteredRows.map((r) => {
                const isDrillOpen = r.type === "command" && selectedProgram === r.programName;
                const colSpan = 8;
                return (
                  <React.Fragment key={r.key}>
                    <tr
                      className={row}
                      style={r.type === "command" && r.programName ? { cursor: "pointer" } : undefined}
                      tabIndex={r.type === "command" && r.programName ? 0 : undefined}
                      role={r.type === "command" && r.programName ? "button" : undefined}
                      aria-expanded={r.type === "command" && r.programName ? isDrillOpen : undefined}
                      onClick={r.type === "command" && r.programName
                        ? () => onSelectProgram(isDrillOpen ? null : r.programName!)
                        : undefined}
                      onKeyDown={r.type === "command" && r.programName ? (e) => {
                        if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onSelectProgram(isDrillOpen ? null : r.programName!); }
                      } : undefined}
                    >
                      <td className={checkboxTd} onClick={(e) => e.stopPropagation()}>
                        <input
                          type="checkbox"
                          checked={selected.has(r.key)}
                          onChange={() => toggleRow(r.key)}
                          aria-label={`Select ${r.name} ${r.subcommand}`}
                        />
                      </td>
                      <td className={td}>
                        <code className={toolName}>{r.name}</code>
                        {r.subcommand && <> <code className={toolName} style={{ opacity: 0.7 }}>{r.subcommand}</code></>}
                        {r.isUncovered && (
                          <span className={gapBadgeLow} style={{ marginLeft: 6 }}>no rule</span>
                        )}
                      </td>
                      <td className={td}>
                        <span className={categoryBadge}>{r.type}</span>
                      </td>
                      <td className={td}>
                        {r.category !== "tool" && <span className={categoryBadge}>{r.category}</span>}
                      </td>
                      <td className={`${td} ${tdRight}`}>{r.count}</td>
                      <td className={`${td} ${tdRight}`}>
                        {r.manualCount > 0
                          ? <span className={manualCount}>{r.manualCount}</span>
                          : <span style={{ opacity: 0.3 }}>—</span>}
                      </td>
                      <td className={`${td} ${tdBar}`}>
                        <Bar value={r.count} max={maxCount} className={r.isUncovered ? barGap : barCmd} />
                      </td>
                      <td className={td} onClick={(e) => e.stopPropagation()}>
                        {r.isUncovered ? (
                          <span style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                            <button
                              data-testid={r.type === "tool" ? `suggest-rule-tool-${r.name}` : `suggest-rule-program-${r.name}`}
                              className={addRuleLink}
                              style={{ background: "none", border: "none", cursor: genLoading ? "not-allowed" : "pointer", padding: 0, font: "inherit" }}
                              disabled={genLoading}
                              onClick={() => {
                                setActiveRowKey(r.key);
                                void generate({
                                  source: SuggestionSource.ANALYTICS_GAPS,
                                  windowDays,
                                  ...(r.type === "tool" ? { toolNameFilter: r.name } : { programNameFilter: r.name }),
                                });
                              }}
                            >
                              {genLoading && activeRowKey === r.key ? "Generating…" : "Suggest rule →"}
                            </button>
                            <a href={r.addRuleHref} className={addRuleLink} style={{ fontSize: "0.8em", opacity: 0.7 }}>
                              or add manually →
                            </a>
                          </span>
                        ) : (
                          <a href={r.addRuleHref} className={addRuleLink} title={`Add a rule for ${r.name}`}>
                            Add rule →
                          </a>
                        )}
                      </td>
                    </tr>
                    {genError && activeRowKey === r.key && (
                      <tr>
                        <td colSpan={colSpan} style={{ color: "var(--error)", fontSize: "0.85em", padding: "4px 8px" }}>
                          {genError.message}
                        </td>
                      </tr>
                    )}
                    {!genError && activeRowKey === r.key && suggestions.length > 0 && (
                      <tr>
                        <td colSpan={colSpan} onClick={(e) => e.stopPropagation()}>
                          <SuggestedRuleCard
                            suggestion={suggestions[0]}
                            onAccept={() => { clearSuggestion(); setActiveRowKey(null); onRuleAccepted?.(); }}
                            onDiscard={() => { clearSuggestion(); setActiveRowKey(null); }}
                          />
                        </td>
                      </tr>
                    )}
                    {isDrillOpen && (
                      <tr>
                        <td colSpan={colSpan} onClick={(e) => e.stopPropagation()}>
                          <ProgramDetailPanel program={r.programName!} windowDays={windowDays} onClose={() => onSelectProgram(null)} />
                        </td>
                      </tr>
                    )}
                  </React.Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

