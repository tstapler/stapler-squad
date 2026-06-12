"use client";

import { useDeferredValue, useMemo } from "react";
import { RuleCriteria, computePreview } from "@/lib/rulePreview";
import {
  wrapper, heading, grid, column, colHeading, matchHead, noMatchHead,
  exampleRow, empty, notice,
} from "./RulePreview.css";

interface RulePreviewProps {
  criteria: RuleCriteria;
  showSafePythonNotice?: boolean;
}

export function RulePreview({ criteria, showSafePythonNotice }: RulePreviewProps) {
  const deferred = useDeferredValue(criteria);
  const preview = useMemo(() => computePreview(deferred), [deferred]);

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

  return (
    <div className={wrapper}>
      <p className={heading}>Preview</p>
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
