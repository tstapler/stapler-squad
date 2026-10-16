// +feature: settings-programs
"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import {
  ProbeStatus,
  SessionService,
  type FlagInfo,
  type ProbeProgramResponse,
} from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

export type ProbeUiState =
  | { kind: "idle" }
  | { kind: "disabled" }
  | { kind: "checking" }
  | { kind: "found"; path: string; flagCount: number; flags: FlagInfo[] }
  | { kind: "noFlags"; path: string }
  | { kind: "timeout"; path: string }
  | { kind: "needsConfirm"; path: string }
  | { kind: "wrapper" }
  | { kind: "notFound" }
  | { kind: "busyOrError" }
  | { kind: "transportError" };

export type ProbeMode = "program-config" | "picker";

export interface CheckOptions {
  /** The Check button: sends confirm_execute and bypasses the memo. */
  explicit?: boolean;
  /** Enter key: probes now, even if the command is unchanged and settled. */
  immediate?: boolean;
}

export interface UseProbeProgram {
  state: ProbeUiState;
  /** First whitespace-separated token of the command; what the server probes. */
  checkedToken: string;
  check: (opts?: CheckOptions) => void;
}

export function firstToken(command: string): string {
  return command.trim().split(/\s+/)[0] ?? "";
}

/** Maps a response to UI state; never yields notFound for ERROR/BUSY. */
export function toUiState(res: ProbeProgramResponse): ProbeUiState {
  if (res.isWrapper) return { kind: "wrapper" };
  const path = res.resolvedPath;
  switch (res.probeStatus) {
    case ProbeStatus.NOT_FOUND:
      return { kind: "notFound" };
    case ProbeStatus.ERROR:
    case ProbeStatus.BUSY:
      return { kind: "busyOrError" };
    case ProbeStatus.NEEDS_CONFIRM:
      return { kind: "needsConfirm", path };
    case ProbeStatus.TIMEOUT:
      return { kind: "timeout", path };
    case ProbeStatus.FOUND_NO_FLAGS:
      return { kind: "noFlags", path };
    default:
      return res.found
        ? { kind: "found", path, flagCount: res.flags.length, flags: res.flags }
        : { kind: "notFound" };
  }
}

// Transient or non-final results are re-probed on the next check.
const UNSETTLED = new Set<ProbeUiState["kind"]>([
  "idle",
  "checking",
  "busyOrError",
  "transportError",
]);
const MEMOIZABLE = new Set<ProbeUiState["kind"]>([
  "found",
  "noFlags",
  "wrapper",
  "notFound",
]);

function toFailureState(err: unknown): ProbeUiState {
  // Unimplemented = kill switch off. Anything else (network, PermissionDenied,
  // 403 from ProbeGuard) must read "couldn't check", never "not found".
  return ConnectError.from(err).code === Code.Unimplemented
    ? { kind: "disabled" }
    : { kind: "transportError" };
}

export function useProbeProgram(
  command: string,
  mode: ProbeMode,
): UseProbeProgram {
  const [state, setState] = useState<ProbeUiState>({ kind: "idle" });
  const client = useMemo(
    () => createClient(SessionService, getConnectTransport()),
    [],
  );
  const stateRef = useRef(state);
  stateRef.current = state;
  const tokenRef = useRef(0);
  const controllerRef = useRef<AbortController | null>(null);
  const inFlightRef = useRef<string | null>(null);
  const settledCommandRef = useRef<string | null>(null);
  const memoRef = useRef(new Map<string, ProbeUiState>());

  const cancel = useCallback(() => {
    tokenRef.current += 1;
    controllerRef.current?.abort();
    controllerRef.current = null;
    inFlightRef.current = null;
  }, []);

  useEffect(() => {
    settledCommandRef.current = null;
    setState({ kind: "idle" });
    return cancel;
  }, [command, cancel]);

  const check = useCallback(
    (opts: CheckOptions = {}) => {
      const cmd = command.trim();
      if (!cmd) {
        cancel();
        setState({ kind: "idle" });
        return;
      }
      const explicit = opts.explicit === true;
      // An explicit Check supersedes an implicit probe of the same command (blur fires before the click).
      if (inFlightRef.current === cmd && !explicit) return;
      if (!explicit && !opts.immediate) {
        const kind = stateRef.current.kind;
        if (settledCommandRef.current === cmd && !UNSETTLED.has(kind)) return;
        const memo = memoRef.current.get(cmd);
        if (memo) {
          settledCommandRef.current = cmd;
          setState(memo);
          return;
        }
      }

      cancel();
      const token = tokenRef.current;
      const controller = new AbortController();
      controllerRef.current = controller;
      inFlightRef.current = cmd;
      setState({ kind: "checking" });

      const request = {
        command: cmd,
        confirmExecute: explicit,
        resolveOnly: mode === "picker" && !explicit,
      };
      client
        .probeProgram(request, { signal: controller.signal })
        .then(
          (res) => toUiState(res),
          (err: unknown) => (controller.signal.aborted ? null : toFailureState(err)),
        )
        .then((next) => {
          if (next === null || token !== tokenRef.current) return;
          inFlightRef.current = null;
          settledCommandRef.current = cmd;
          if (MEMOIZABLE.has(next.kind)) memoRef.current.set(cmd, next);
          setState(next);
        });
    },
    [client, command, mode, cancel],
  );

  return { state, checkedToken: firstToken(command), check };
}
