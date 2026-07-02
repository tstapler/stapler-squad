"use client";

import { useDeferredValue, useMemo } from "react";
import { RuleCriteria, SubcommandStat, computePreview, computeCoverage } from "@/lib/rulePreview";
import {
  wrapper, heading, grid, column, colHeading, matchHead, noMatchHead,
  exampleRow, empty, notice,
  coverageBar, coverageLabel, coverageTrack, coverageFill,
  suggestionsRow, suggestionLabel, suggestionChip,
} from "./RulePreview.css";

interface RulePreviewProps {
  criteria: RuleCriteria;
  showSafePythonNotice?: boolean;
  subcommandStats?: SubcommandStat[];
  onAddSubcommand?: (sub: string) => void;
}

export function RulePreview({ criteria, showSafePythonNotice, subcommandStats, onAddSubcommand }: RulePreviewProps) {
  const deferred = useDeferredValue(criteria);
  const preview = useMemo(() => computePreview(deferred), [deferred]);
  const coverage = useMemo(
    () => (subcommandStats ? computeCoverage(deferred, subcommandStats) : null),
    [deferred, subcommandStats],
  );

  const hasAnyCriteria =
    deferred.programs.length > 0 ||
    deferred.subcommands.length > 0 ||
    deferred.blockedSubcommands.length > 0 ||
    deferred.requiredFlags.length > 0 ||
    deferred.forbiddenFlags.length > 0 ||
    deferred.requiredFlagPrefixes.length > 0 ||
    deferred.pythonModes.length > 0;

  if (!hasAnyCriteria) {
    return (
      <div className={wrapper}>
        <p className={`${heading} ${empty}`}>Add criteria above to see matching examples.</p>
      </div>
    );
  }

  const coveragePct = coverage && coverage.total > 0
    ? Math.round((coverage.covered / coverage.total) * 100)
    : null;

  return (
    <div className={wrapper}>
      <p className={heading}>Preview</p>

      {coverage && coverage.total > 0 && (
        <div className={coverageBar}>
          <span className={coverageLabel}>
            <span>Covers {coverage.covered} of {coverage.total} real decisions</span>
            <span>{coveragePct}%</span>
          </span>
          <div className={coverageTrack}>
            <div className={coverageFill} style={{ width: `${coveragePct ?? 0}%` }} />
          </div>
        </div>
      )}

      {coverage && coverage.uncoveredSubcommands.length > 0 && onAddSubcommand && (
        <div className={suggestionsRow}>
          <span className={suggestionLabel}>Also add:</span>
          {coverage.uncoveredSubcommands.map((sub) => (
            <button
              key={sub}
              className={suggestionChip}
              type="button"
              title={`Add "${sub}" to subcommands`}
              onClick={() => onAddSubcommand(sub)}
            >
              {sub}
            </button>
          ))}
        </div>
      )}

      <div className={grid}>
        <div className={column}>
          <p className={`${colHeading} ${matchHead}`}>✓ Would match</p>
          {preview.matches.length === 0 ? (
            <p className={empty}>No examples found — check program spelling.</p>
          ) : (
            preview.matches.map((cmd) => (
              <code key={cmd} className={exampleRow} title={cmd}>{cmd}</code>
            ))
          )}
        </div>
        <div className={column}>
          <p className={`${colHeading} ${noMatchHead}`}>✗ Would not match</p>
          {preview.nonMatches.length === 0 ? (
            <p className={empty}>All examples match.</p>
          ) : (
            preview.nonMatches.map((cmd) => (
              <code key={cmd} className={exampleRow} title={cmd}>{cmd}</code>
            ))
          )}
        </div>
      </div>
      {showSafePythonNotice && (
        <p className={notice}>
          Note: "Safe stdlib imports only" filtering is not shown in preview — save the rule to test it.
        </p>
      )}
    </div>
  );
}
