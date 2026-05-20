"use client";

import React, { useState } from "react";
import { useApprovalAnalytics } from "@/lib/hooks/useApprovalAnalytics";
import { useGenerateRule } from "@/lib/hooks/useGenerateRule";
import { DailyBucketProto, SubcommandStatProto, SuggestionSource } from "@/gen/session/v1/types_pb";
import { SuggestedRuleCard } from "./SuggestedRuleCard";
import { ProgramDetailPanel } from "./ProgramDetailPanel";
import {
  panel, header, titleRow, title, subtitle, refreshButton,
  windowSelector, windowBtn, windowBtnActive,
  error as errorClass, retryButton,
  cards, card, cardAllow, cardDeny, cardManual, cardValue, cardLabel, cardSub,
  loading as loadingClass, empty, emptyHint,
  sectionTitle, tableSection, tableWrapper, table, th, thRight, td, tdRight, tdBar, row,
  allowCount, denyCount, manualCount, pctLabel, toolName, ruleName,
  barTrack, barFill, barTotal, barTool, barRule, barCmd, barPython, barGap,
  categoryBadge, subSectionTitle, filterInput, addRuleLink,
  coverageGapHeader, coverageGapHigh, coverageGapMed, coverageGapLow,
  coverageGapTitleRow, coverageGapIcon, coverageGapTitle, coverageGapBadge, coverageGapDesc,
  suggestRuleButton, addRuleManualLink, rowActions, rowGeneratingText,
  inlineSuggestionRow, inlineSuggestionCell, inlineErrorText,
} from "./ApprovalAnalyticsPanel.css";

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
    <div className={barTrack}>
      <div className={`${barFill} ${className}`} style={{ width: `${width}%` }} />
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
  const { summary, dailyBuckets, loading, error, refresh } = useApprovalAnalytics({ windowDays });
  const { suggestions, loading: generateLoading, error: generateError, generate, clear } = useGenerateRule();
  const [activeRowKey, setActiveRowKey] = useState<string | null>(null);
  const [selectedProgram, setSelectedProgram] = useState<string | null>(null);

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

  // Max total across days — used to scale inline bars.
  const maxDayTotal = dailyBuckets.reduce((m, b) => Math.max(m, b.total), 0);

  return (
    <div className={panel}>
      {/* ── Header ── */}
      <div className={header}>
        <div className={titleRow}>
          <h2 className={title}>Approval Analytics</h2>
          <button
            onClick={refresh}
            className={refreshButton}
            disabled={loading}
            aria-label="Refresh analytics"
          >
            {loading ? "⟳" : "↻"}
          </button>
        </div>
        <p className={subtitle}>
          Decision trends for auto-classification over time.
        </p>
      </div>

      {/* ── Window selector ── */}
      <div className={windowSelector}>
        {WINDOW_OPTIONS.map((opt) => (
          <button
            key={opt.value}
            className={`${windowBtn} ${windowDays === opt.value ? windowBtnActive : ""}`}
            onClick={() => setWindowDays(opt.value)}
          >
            {opt.label}
          </button>
        ))}
      </div>

      {error && (
        <div className={errorClass}>
          Failed to load analytics: {error.message}
          <button onClick={refresh} className={retryButton}>Retry</button>
        </div>
      )}

      {/* ── Summary cards ── */}
      <div className={cards}>
        <div className={card}>
          <span className={cardValue}>{total}</span>
          <span className={cardLabel}>Total decisions</span>
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
          <span className={cardSub}>{escalateCount} requests</span>
        </div>
        <div className={card}>
          <span className={cardValue}>{avgPerDay}</span>
          <span className={cardLabel}>Avg / day</span>
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
                        <Bar value={b.total} max={maxDayTotal} className={barTotal} />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Top tools ── */}
      {summary && summary.topTools.length > 0 && (
        <div className={tableSection}>
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

      {/* ── Top triggered rules ── */}
      {summary && summary.topTriggeredRules.length > 0 && (
        <div className={tableSection}>
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
                    <td className={td}>
                      <span className={ruleName}>{r.ruleName || r.ruleId}</span>
                    </td>
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

      {/* ── Top command programs (Bash AST) ── */}
      {summary && summary.topCommandPrograms.length > 0 && (
        <div className={tableSection}>
          <h3 className={sectionTitle}>Top Bash Programs</h3>
          <div className={tableWrapper}>
            <table className={table}>
              <thead>
                <tr>
                  <th className={th}>Program</th>
                  <th className={th}>Category</th>
                  <th className={`${th} ${thRight}`}>Calls</th>
                  <th className={th}>Share</th>
                </tr>
              </thead>
              <tbody>
                {summary.topCommandPrograms.map((p) => (
                  <tr key={p.programName} className={row}>
                    <td className={td}><code className={toolName}>{p.programName}</code></td>
                    <td className={td}>
                      <span className={categoryBadge}>{p.category}</span>
                    </td>
                    <td className={`${td} ${tdRight}`}>{p.count}</td>
                    <td className={`${td} ${tdBar}`}>
                      <Bar value={p.count} max={summary.topCommandPrograms[0]?.count ?? 1} className={barCmd} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
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

      {/* ── Command distribution ── */}
      {summary && summary.commandSubcommandStats.length > 0 && (
        <div className={tableSection}>
          <h3 className={sectionTitle}>Command Distribution</h3>
          <CommandDistributionTable stats={summary.commandSubcommandStats} />
        </div>
      )}

      {/* ── Rule coverage gaps ── */}
      {summary && summary.coverageGapCount > 0 && (
        <div className={tableSection}>
          <CoverageGapHeader gapCount={summary.coverageGapCount} gapRate={summary.coverageGapRate} total={total} />

          {summary.topUncoveredTools.length > 0 && (
            <>
              <h4 className={subSectionTitle}>Uncovered Tools</h4>
              <div className={tableWrapper}>
                <table className={table}>
                  <thead>
                    <tr>
                      <th className={th}>Tool</th>
                      <th className={`${th} ${thRight}`}>Unmatched</th>
                      <th className={th}>Share of gaps</th>
                      <th className={th}></th>
                    </tr>
                  </thead>
                  <tbody>
                    {summary.topUncoveredTools.map((t) => {
                      const isActive = activeRowKey === `tool:${t.toolName}`;
                      const isGenerating = isActive && generateLoading;
                      const activeSuggestion = isActive && !generateLoading && suggestions.length > 0 ? suggestions[0] : null;
                      const activeError = isActive && !generateLoading ? generateError : null;
                      return (
                        <React.Fragment key={t.toolName}>
                          <tr className={row}>
                            <td className={td}><code className={toolName}>{t.toolName}</code></td>
                            <td className={`${td} ${tdRight}`}>{t.count}</td>
                            <td className={`${td} ${tdBar}`}>
                              <Bar value={t.count} max={summary.topUncoveredTools[0]?.count ?? 1} className={barGap} />
                            </td>
                            <td className={td}>
                              <div className={rowActions}>
                                {isGenerating ? (
                                  <span className={rowGeneratingText}>Generating…</span>
                                ) : (
                                  <button
                                    className={suggestRuleButton}
                                    data-testid={`suggest-rule-tool-${t.toolName}`}
                                    disabled={generateLoading}
                                    onClick={() => {
                                      setActiveRowKey(`tool:${t.toolName}`);
                                      void generate({
                                        source: SuggestionSource.ANALYTICS_GAPS,
                                        toolNameFilter: t.toolName,
                                        windowDays,
                                      });
                                    }}
                                  >
                                    Suggest Rule
                                  </button>
                                )}
                                <a href="/rules" className={addRuleManualLink} title="Add a rule manually">
                                  or add manually →
                                </a>
                              </div>
                            </td>
                          </tr>
                          {activeError && (
                            <tr className={inlineSuggestionRow}>
                              <td colSpan={4} className={inlineErrorText}>
                                Error: {activeError.message}
                              </td>
                            </tr>
                          )}
                          {activeSuggestion && (
                            <tr className={inlineSuggestionRow}>
                              <td colSpan={4} className={inlineSuggestionCell}>
                                <SuggestedRuleCard
                                  suggestion={activeSuggestion}
                                  onAccept={() => {
                                    clear();
                                    setActiveRowKey(null);
                                    refresh();
                                  }}
                                  onDiscard={() => {
                                    clear();
                                    setActiveRowKey(null);
                                  }}
                                />
                              </td>
                            </tr>
                          )}
                        </React.Fragment>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </>
          )}

          {summary.topUncoveredPrograms.length > 0 && (
            <>
              <h4 className={subSectionTitle}>Uncovered Bash Programs</h4>
              <div className={tableWrapper}>
                <table className={table}>
                  <thead>
                    <tr>
                      <th className={th}>Program</th>
                      <th className={th}>Category</th>
                      <th className={`${th} ${thRight}`}>Unmatched</th>
                      <th className={th}>Share of gaps</th>
                      <th className={th}></th>
                    </tr>
                  </thead>
                  <tbody>
                    {summary.topUncoveredPrograms.map((p) => {
                      const isActive = activeRowKey === `program:${p.programName}`;
                      const isGenerating = isActive && generateLoading;
                      const activeSuggestion = isActive && !generateLoading && suggestions.length > 0 ? suggestions[0] : null;
                      const activeError = isActive && !generateLoading ? generateError : null;
                      const isDrillOpen = selectedProgram === p.programName;
                      return (
                        <React.Fragment key={p.programName}>
                          <tr
                            className={row}
                            style={{ cursor: "pointer" }}
                            tabIndex={0}
                            role="button"
                            aria-expanded={isDrillOpen}
                            aria-label={`${p.programName} — click to ${isDrillOpen ? "collapse" : "expand"} details`}
                            onClick={() =>
                              setSelectedProgram(isDrillOpen ? null : p.programName)
                            }
                            onKeyDown={(e) => {
                              if (e.key === "Enter" || e.key === " ") {
                                e.preventDefault();
                                setSelectedProgram(isDrillOpen ? null : p.programName);
                              }
                            }}
                          >
                            <td className={td}><code className={toolName}>{p.programName}</code></td>
                            <td className={td}><span className={categoryBadge}>{p.category}</span></td>
                            <td className={`${td} ${tdRight}`}>{p.count}</td>
                            <td className={`${td} ${tdBar}`}>
                              <Bar value={p.count} max={summary.topUncoveredPrograms[0]?.count ?? 1} className={barGap} />
                            </td>
                            <td className={td}>
                              <div className={rowActions}>
                                {isGenerating ? (
                                  <span className={rowGeneratingText}>Generating…</span>
                                ) : (
                                  <button
                                    className={suggestRuleButton}
                                    data-testid={`suggest-rule-program-${p.programName}`}
                                    disabled={generateLoading}
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      setActiveRowKey(`program:${p.programName}`);
                                      void generate({
                                        source: SuggestionSource.ANALYTICS_GAPS,
                                        programNameFilter: p.programName,
                                        windowDays,
                                      });
                                    }}
                                  >
                                    Suggest Rule
                                  </button>
                                )}
                                <a
                                  href="/rules"
                                  className={addRuleManualLink}
                                  title="Add a rule manually"
                                  onClick={(e) => e.stopPropagation()}
                                >
                                  or add manually →
                                </a>
                              </div>
                            </td>
                          </tr>
                          {activeError && (
                            <tr className={inlineSuggestionRow}>
                              <td colSpan={5} className={inlineErrorText}>
                                Error: {activeError.message}
                              </td>
                            </tr>
                          )}
                          {activeSuggestion && (
                            <tr className={inlineSuggestionRow}>
                              <td colSpan={5} className={inlineSuggestionCell}>
                                <SuggestedRuleCard
                                  suggestion={activeSuggestion}
                                  onAccept={() => {
                                    clear();
                                    setActiveRowKey(null);
                                    refresh();
                                  }}
                                  onDiscard={() => {
                                    clear();
                                    setActiveRowKey(null);
                                  }}
                                />
                              </td>
                            </tr>
                          )}
                          {isDrillOpen && (
                            <tr>
                              <td colSpan={5} onClick={(e) => e.stopPropagation()}>
                                <ProgramDetailPanel
                                  program={p.programName}
                                  windowDays={windowDays}
                                  onClose={() => setSelectedProgram(null)}
                                />
                              </td>
                            </tr>
                          )}
                        </React.Fragment>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}

// ── CommandDistributionTable ───────────────────────────────────────────────────

function CommandDistributionTable({ stats }: { stats: SubcommandStatProto[] }) {
  const [filter, setFilter] = useState("");
  const lc = filter.toLowerCase();
  const filtered = lc
    ? stats.filter(
        (s) =>
          s.programName.toLowerCase().includes(lc) ||
          s.subcommand.toLowerCase().includes(lc)
      )
    : stats;
  const maxCount = filtered[0]?.count ?? 1;

  return (
    <>
      <input
        type="text"
        placeholder="Filter by program or subcommand (e.g. gh, sed, aws s3)…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        className={filterInput}
        aria-label="Filter command distribution entries"
      />
      <div className={tableWrapper}>
        <table className={table}>
          <thead>
            <tr>
              <th className={th}>Program</th>
              <th className={th}>Subcommand</th>
              <th className={th}>Category</th>
              <th className={`${th} ${thRight}`}>Calls</th>
              <th className={th}>Share</th>
              <th className={th}></th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((s) => (
              <tr key={s.programName + ":" + s.subcommand} className={row}>
                <td className={td}>
                  <code className={toolName}>{s.programName}</code>
                </td>
                <td className={td}>
                  <code className={toolName}>{s.subcommand}</code>
                </td>
                <td className={td}>
                  <span className={categoryBadge}>{s.category}</span>
                </td>
                <td className={`${td} ${tdRight}`}>{s.count}</td>
                <td className={`${td} ${tdBar}`}>
                  <Bar value={s.count} max={maxCount} className={barCmd} />
                </td>
                <td className={td}>
                  <a
                    href="/rules"
                    className={addRuleLink}
                    title={`Add a rule for ${s.programName} ${s.subcommand}`}
                  >
                    Add rule →
                  </a>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

// ── CoverageGapHeader ─────────────────────────────────────────────────────────

function CoverageGapHeader({ gapCount, gapRate, total }: { gapCount: number; gapRate: number; total: number }) {
  const rounded = Math.round(gapRate);
  const isHigh  = rounded >= 30;
  const isMed   = rounded >= 10;

  return (
    <div className={`${coverageGapHeader} ${isHigh ? coverageGapHigh : isMed ? coverageGapMed : coverageGapLow}`}>
      <div className={coverageGapTitleRow}>
        <span className={coverageGapIcon}>{isHigh ? "⚠️" : isMed ? "💡" : "✓"}</span>
        <h3 className={coverageGapTitle}>Rule Coverage Gaps</h3>
        <span className={coverageGapBadge}>{rounded}% uncovered</span>
      </div>
      <p className={coverageGapDesc}>
        {gapCount} of {total} decision{total !== 1 ? "s" : ""} had no matching rule and went to manual review.{" "}
        {isHigh
          ? "High gap rate — adding rules for the patterns below could significantly reduce manual review."
          : isMed
          ? "Consider adding rules for frequently unmatched patterns."
          : "Coverage is good. Review any new patterns to stay ahead."}
      </p>
    </div>
  );
}
