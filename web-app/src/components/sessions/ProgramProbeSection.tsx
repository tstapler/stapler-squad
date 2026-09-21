// +feature: session-create settings-programs
"use client";

import { useEffect } from "react";
import type { ProgramOption } from "@/lib/constants/programs";
import { useProbeProgram } from "@/lib/hooks/useProbeProgram";
import { ProbeStatusBadge } from "@/components/ui/ProbeStatusBadge";

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

  return (
    <ProbeStatusBadge
      state={state}
      checkedToken={checkedToken}
      onRetry={() => check({ immediate: true })}
      onConfirm={() => check({ explicit: true })}
      testId={state.kind === "notFound" ? NOT_FOUND_TEST_ID : "program-probe-badge"}
      id={PROGRAM_PROBE_STATUS_ID}
    />
  );
}
