/**
 * The single shared probe mock (jscpd budget). Every test touching
 * useProbeProgram, ProbeStatusBadge, or a form that embeds them imports this
 * instead of re-declaring jest.mock blocks.
 *
 * Client-level tests (hook):
 *   jest.mock("@connectrpc/connect", () => require("@/lib/hooks/__mocks__/probeProgramMock").connectMockFactory());
 *   jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));
 *   import { mockProbeClient, probeResponse } from "@/lib/hooks/__mocks__/probeProgramMock";
 *
 * Component-level tests (forms): stub the hook wholesale:
 *   jest.mock("@/lib/hooks/useProbeProgram", () => require("@/lib/hooks/__mocks__/probeProgramMock").hookMockFactory());
 *   then setProbeHookState(probeStates.notFound) and assert on mockProbeCheck calls.
 */
import { ProbeStatus } from "@/gen/session/v1/session_pb";
import type { ProbeUiState, UseProbeProgram } from "../useProbeProgram";

export const mockProbeClient = { probeProgram: jest.fn() };
export const mockProbeCheck = jest.fn();

let hookState: ProbeUiState = { kind: "idle" };
let hookToken = "";

/** Factory for jest.mock("@connectrpc/connect"): real Code/ConnectError, fake client. */
export function connectMockFactory() {
  const actual = jest.requireActual("@connectrpc/connect");
  return { ...actual, createClient: () => mockProbeClient };
}

/** Factory for jest.mock("@/lib/hooks/useProbeProgram"): real helpers, stubbed hook. */
export function hookMockFactory() {
  const actual = jest.requireActual("../useProbeProgram");
  return {
    ...actual,
    useProbeProgram: (): UseProbeProgram => ({
      state: hookState,
      checkedToken: hookToken,
      check: mockProbeCheck,
    }),
  };
}

export function setProbeHookState(state: ProbeUiState, checkedToken = "claude") {
  hookState = state;
  hookToken = checkedToken;
}

export function resetProbeMocks() {
  mockProbeClient.probeProgram.mockReset();
  mockProbeCheck.mockReset();
  hookState = { kind: "idle" };
  hookToken = "";
}

/** Plain-object ProbeProgramResponse (the hook only reads fields). */
export function probeResponse(overrides: Record<string, unknown> = {}) {
  return {
    found: true,
    resolvedPath: "/usr/bin/claude",
    flags: [],
    probeStatus: ProbeStatus.UNSPECIFIED,
    truncated: false,
    isWrapper: false,
    ...overrides,
  };
}

export function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

/** One representative ProbeUiState per variant. */
export const probeStates: Record<string, ProbeUiState> = {
  idle: { kind: "idle" },
  disabled: { kind: "disabled" },
  checking: { kind: "checking" },
  found: { kind: "found", path: "/usr/bin/claude", flagCount: 14, flags: [] },
  noFlags: { kind: "noFlags", path: "/usr/bin/claude" },
  timeout: { kind: "timeout", path: "/usr/bin/claude" },
  needsConfirm: { kind: "needsConfirm", path: "/usr/bin/claude" },
  wrapper: { kind: "wrapper" },
  notFound: { kind: "notFound" },
  busyOrError: { kind: "busyOrError" },
  transportError: { kind: "transportError" },
};
