"use client";

import { useState } from "react";
import { useApprovalAnalytics } from "@/lib/hooks/useApprovalAnalytics";
import { DailyBucketProto, SubcommandStatProto } from "@/gen/session/v1/types_pb";
import styles from "./ApprovalAnalyticsPanel.module.css";

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
    <div className={styles.barTrack}>
      <div className={`${styles.barFill} ${className}`} style={{ width: `${width}%` }} />
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
    <div className={styles.panel}>
      {/* ── Header ── */}
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <h2 className={styles.title}>Approval Analytics</h2>
          <button
            onClick={refresh}
            className={styles.refreshButton}
            disabled={loading}
            aria-label="Refresh analytics"
          >
            {loading ? "⟳" : "↻"}
          </button>
        </div>
        <p className={styles.subtitle}>
          Decision trends for auto-classification over time.
        </p>
      </div>

      {/* ── Window selector ── */}
      <div className={styles.windowSelector}>
        {WINDOW_OPTIONS.map((opt) => (
          <button
            key={opt.value}
            className={`${styles.windowBtn} ${windowDays === opt.value ? styles.windowBtnActive : ""}`}
            onClick={() => setWindowDays(opt.value)}
          >
            {opt.label}
          </button>
        ))}
      </div>

      {error && (
        <div className={styles.error}>
          Failed to load analytics: {error.message}
          <button onClick={refresh} className={styles.retryButton}>Retry</button>
        </div>
      )}

      {/* ── Summary cards ── */}
      <div className={styles.cards}>
        <div className={styles.card}>
          <span className={styles.cardValue}>{total}</span>
          <span className={styles.cardLabel}>Total decisions</span>
        </div>
        <div className={`${styles.card} ${styles.cardAllow}`}>
          <span className={styles.cardValue}>{autoAllowRate}%</span>
          <span className={styles.cardLabel}>Auto-allowed</span>
          <span className={styles.cardSub}>{autoAllowCount} requests</span>
        </div>
        <div className={`${styles.card} ${styles.cardDeny}`}>
          <span className={styles.cardValue}>{autoDenyRate}%</span>
          <span className={styles.cardLabel}>Auto-denied</span>
          <span className={styles.cardSub}>{autoDenyCount} requests</span>
        </div>
        <div className={`${styles.card} ${styles.cardManual}`}>
          <span className={styles.cardValue}>{manualRate}%</span>
          <span className={styles.cardLabel}>Manual review</span>
          <span className={styles.cardSub}>{escalateCount} requests</span>
        </div>
        <div className={styles.card}>
          <span className={styles.cardValue}>{avgPerDay}</span>
          <span className={styles.cardLabel}>Avg / day</span>
        </div>
      </div>

      {/* ── Daily breakdown ── */}
      {loading && dailyBuckets.length === 0 ? (
        <div className={styles.loading}>Loading analytics…</div>
      ) : dailyBuckets.length === 0 ? (
        <div className={styles.empty}>
          No data for the last {windowDays} days.
          <br />
          <span className={styles.emptyHint}>Analytics are recorded when Claude Code sends hook requests.</span>
        </div>
      ) : (
        <div className={styles.tableSection}>
          <h3 className={styles.sectionTitle}>Daily Breakdown</h3>
          <div className={styles.tableWrapper}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th className={styles.th}>Date</th>
                  <th className={`${styles.th} ${styles.thRight}`}>Total</th>
                  <th className={`${styles.th} ${styles.thRight}`}>Allow</th>
                  <th className={`${styles.th} ${styles.thRight}`}>Deny</th>
                  <th className={`${styles.th} ${styles.thRight}`}>Manual</th>
                  <th className={styles.th}>Volume</th>
                </tr>
              </thead>
              <tbody>
                {[...dailyBuckets].reverse().map((b) => {
                  const manualTotal = b.escalate + b.manualAllow + b.manualDeny;
                  return (
                    <tr key={b.date} className={styles.row}>
                      <td className={styles.td}>{formatDate(b.date)}</td>
                      <td className={`${styles.td} ${styles.tdRight}`}>{b.total}</td>
                      <td className={`${styles.td} ${styles.tdRight}`}>
                        <span className={styles.allowCount}>{b.autoAllow}</span>
                        <span className={styles.pctLabel}> {pct(b.autoAllow, b.total)}%</span>
                      </td>
                      <td className={`${styles.td} ${styles.tdRight}`}>
                        <span className={styles.denyCount}>{b.autoDeny}</span>
                        <span className={styles.pctLabel}> {pct(b.autoDeny, b.total)}%</span>
                      </td>
                      <td className={`${styles.td} ${styles.tdRight}`}>
                        <span className={styles.manualCount}>{manualTotal}</span>
                        <span className={styles.pctLabel}> {pct(manualTotal, b.total)}%</span>
                      </td>
                      <td className={`${styles.td} ${styles.tdBar}`}>
                        <Bar value={b.total} max={maxDayTotal} className={styles.barTotal} />
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
        <div className={styles.tableSection}>
          <h3 className={styles.sectionTitle}>Top Tools</h3>
          <div className={styles.tableWrapper}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th className={styles.th}>Tool</th>
                  <th className={`${styles.th} ${styles.thRight}`}>Requests</th>
                  <th className={styles.th}>Share</th>
                </tr>
              </thead>
              <tbody>
                {summary.topTools.map((t) => (
                  <tr key={t.toolName} className={styles.row}>
                    <td className={styles.td}><code className={styles.toolName}>{t.toolName}</code></td>
                    <td className={`${styles.td} ${styles.tdRight}`}>{t.count}</td>
                    <td className={`${styles.td} ${styles.tdBar}`}>
                      <Bar value={t.count} max={summary.topTools[0]?.count ?? 1} className={styles.barTool} />
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
        <div className={styles.tableSection}>
          <h3 className={styles.sectionTitle}>Top Triggered Rules</h3>
          <div className={styles.tableWrapper}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th className={styles.th}>Rule</th>
                  <th className={`${styles.th} ${styles.thRight}`}>Triggers</th>
                  <th className={styles.th}>Frequency</th>
                </tr>
              </thead>
              <tbody>
                {summary.topTriggeredRules.map((r) => (
                  <tr key={r.ruleId} className={styles.row}>
                    <td className={styles.td}>
                      <span className={styles.ruleName}>{r.ruleName || r.ruleId}</span>
                    </td>
                    <td className={`${styles.td} ${styles.tdRight}`}>{r.count}</td>
                    <td className={`${styles.td} ${styles.tdBar}`}>
                      <Bar value={r.count} max={summary.topTriggeredRules[0]?.count ?? 1} className={styles.barRule} />
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
        <div className={styles.tableSection}>
          <h3 className={styles.sectionTitle}>Top Bash Programs</h3>
          <div className={styles.tableWrapper}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th className={styles.th}>Program</th>
                  <th className={styles.th}>Category</th>
                  <th className={`${styles.th} ${styles.thRight}`}>Calls</th>
                  <th className={styles.th}>Share</th>
                </tr>
              </thead>
              <tbody>
                {summary.topCommandPrograms.map((p) => (
                  <tr key={p.programName} className={styles.row}>
                    <td className={styles.td}><code className={styles.toolName}>{p.programName}</code></td>
                    <td className={styles.td}>
                      <span className={styles.categoryBadge}>{p.category}</span>
                    </td>
                    <td className={`${styles.td} ${styles.tdRight}`}>{p.count}</td>
                    <td className={`${styles.td} ${styles.tdBar}`}>
                      <Bar value={p.count} max={summary.topCommandPrograms[0]?.count ?? 1} className={styles.barCmd} />
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
        <div className={styles.tableSection}>
          <h3 className={styles.sectionTitle}>Top Python Imports</h3>
          <div className={styles.tableWrapper}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th className={styles.th}>Module</th>
                  <th className={`${styles.th} ${styles.thRight}`}>Uses</th>
                  <th className={styles.th}>Share</th>
                </tr>
              </thead>
              <tbody>
                {summary.topPythonImports.map((imp) => (
                  <tr key={imp.module} className={styles.row}>
                    <td className={styles.td}><code className={styles.toolName}>{imp.module}</code></td>
                    <td className={`${styles.td} ${styles.tdRight}`}>{imp.count}</td>
                    <td className={`${styles.td} ${styles.tdBar}`}>
                      <Bar value={imp.count} max={summary.topPythonImports[0]?.count ?? 1} className={styles.barPython} />
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
        <div className={styles.tableSection}>
          <h3 className={styles.sectionTitle}>Command Distribution</h3>
          <CommandDistributionTable stats={summary.commandSubcommandStats} />
        </div>
      )}

      {/* ── Rule coverage gaps ── */}
      {summary && summary.coverageGapCount > 0 && (
        <div className={styles.tableSection}>
          <CoverageGapHeader gapCount={summary.coverageGapCount} gapRate={summary.coverageGapRate} total={total} />

          {summary.topUncoveredTools.length > 0 && (
            <>
              <h4 className={styles.subSectionTitle}>Uncovered Tools</h4>
              <div className={styles.tableWrapper}>
                <table className={styles.table}>
                  <thead>
                    <tr>
                      <th className={styles.th}>Tool</th>
                      <th className={`${styles.th} ${styles.thRight}`}>Unmatched</th>
                      <th className={styles.th}>Share of gaps</th>
                      <th className={styles.th}></th>
                    </tr>
                  </thead>
                  <tbody>
                    {summary.topUncoveredTools.map((t) => (
                      <tr key={t.toolName} className={styles.row}>
                        <td className={styles.td}><code className={styles.toolName}>{t.toolName}</code></td>
                        <td className={`${styles.td} ${styles.tdRight}`}>{t.count}</td>
                        <td className={`${styles.td} ${styles.tdBar}`}>
                          <Bar value={t.count} max={summary.topUncoveredTools[0]?.count ?? 1} className={styles.barGap} />
                        </td>
                        <td className={styles.td}>
                          <a href="/rules" className={styles.addRuleLink} title="Add a rule to cover this tool">
                            Add rule →
                          </a>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          {summary.topUncoveredPrograms.length > 0 && (
            <>
              <h4 className={styles.subSectionTitle}>Uncovered Bash Programs</h4>
              <div className={styles.tableWrapper}>
                <table className={styles.table}>
                  <thead>
                    <tr>
                      <th className={styles.th}>Program</th>
                      <th className={styles.th}>Category</th>
                      <th className={`${styles.th} ${styles.thRight}`}>Unmatched</th>
                      <th className={styles.th}>Share of gaps</th>
                      <th className={styles.th}></th>
                    </tr>
                  </thead>
                  <tbody>
                    {summary.topUncoveredPrograms.map((p) => (
                      <tr key={p.programName} className={styles.row}>
                        <td className={styles.td}><code className={styles.toolName}>{p.programName}</code></td>
                        <td className={styles.td}><span className={styles.categoryBadge}>{p.category}</span></td>
                        <td className={`${styles.td} ${styles.tdRight}`}>{p.count}</td>
                        <td className={`${styles.td} ${styles.tdBar}`}>
                          <Bar value={p.count} max={summary.topUncoveredPrograms[0]?.count ?? 1} className={styles.barGap} />
                        </td>
                        <td className={styles.td}>
                          <a href="/rules" className={styles.addRuleLink} title="Add a rule to cover this program">
                            Add rule →
                          </a>
                        </td>
                      </tr>
                    ))}
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
        className={styles.filterInput}
      />
      <div className={styles.tableWrapper}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th className={styles.th}>Program</th>
              <th className={styles.th}>Subcommand</th>
              <th className={styles.th}>Category</th>
              <th className={`${styles.th} ${styles.thRight}`}>Calls</th>
              <th className={styles.th}>Share</th>
              <th className={styles.th}></th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((s) => (
              <tr key={s.programName + ":" + s.subcommand} className={styles.row}>
                <td className={styles.td}>
                  <code className={styles.toolName}>{s.programName}</code>
                </td>
                <td className={styles.td}>
                  <code className={styles.toolName}>{s.subcommand}</code>
                </td>
                <td className={styles.td}>
                  <span className={styles.categoryBadge}>{s.category}</span>
                </td>
                <td className={`${styles.td} ${styles.tdRight}`}>{s.count}</td>
                <td className={`${styles.td} ${styles.tdBar}`}>
                  <Bar value={s.count} max={maxCount} className={styles.barCmd} />
                </td>
                <td className={styles.td}>
                  <a
                    href="/rules"
                    className={styles.addRuleLink}
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
    <div className={`${styles.coverageGapHeader} ${isHigh ? styles.coverageGapHigh : isMed ? styles.coverageGapMed : styles.coverageGapLow}`}>
      <div className={styles.coverageGapTitleRow}>
        <span className={styles.coverageGapIcon}>{isHigh ? "⚠️" : isMed ? "💡" : "✓"}</span>
        <h3 className={styles.coverageGapTitle}>Rule Coverage Gaps</h3>
        <span className={styles.coverageGapBadge}>{rounded}% uncovered</span>
      </div>
      <p className={styles.coverageGapDesc}>
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
