// +feature: session-create settings-programs
"use client";

import { useEffect, useMemo } from "react";
import type { ProgramOption } from "@/lib/constants/programs";
import { useProbeProgram } from "@/lib/hooks/useProbeProgram";
import { ProbeStatusBadge } from "@/components/ui/ProbeStatusBadge";
import { UnknownFlagsWarning } from "@/components/ui/UnknownFlagsWarning";
import { validateFlags } from "@/lib/flags/validateFlags";

export const PROGRAM_PROBE_STATUS_ID = "omnibar-program-probe-status";
// Kept so the not-found variant stays addressable by its pre-probe testid.
const NOT_FOUND_TEST_ID = "preset-program-warning";

export interface ProgramProbeSectionProps {
  option: ProgramOption | undefined;
}

/** Probe status for the selected program. Selection only resolves; running --help needs an explicit Check. */
export function ProgramProbeSection({ option }: ProgramProbeSectionProps) {
  const command = option?.command ?? "";
  const { state, checkedToken, check } = useProbeProgram(command, "picker");

  useEffect(() => {
    check();
  }, [check]);

  // Only the saved cli_flags are validated; alias extraFlags never reach the panel (Flagged Choice 2).
  const unknown = useMemo(
    () => (state.kind === "found" ? validateFlags(option?.cliFlags ?? "", state.flags) : []),
    [state, option?.cliFlags],
  );

  return (
    <>
      <ProbeStatusBadge
        state={state}
        checkedToken={checkedToken}
        onRetry={() => check({ immediate: true })}
        onConfirm={() => check({ explicit: true })}
        testId={state.kind === "notFound" ? NOT_FOUND_TEST_ID : "program-probe-badge"}
        id={PROGRAM_PROBE_STATUS_ID}
      />
      <UnknownFlagsWarning
        id="omnibar-flags-warning-text"
        testId="omnibar-flags-warning"
        program={checkedToken}
        unknown={unknown}
      />
    </>
  );
}
