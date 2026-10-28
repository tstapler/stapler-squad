import { toJsonString } from "@bufbuild/protobuf";
import type { DescMessage, DescMethodUnary, MessageShape } from "@bufbuild/protobuf";

/**
 * Hand-maintained list of idempotent SessionService reads. `idempotency_level` is
 * absent from session.proto, so nothing is derived: a method is coalesced, replayed
 * or sent over the WebSocket only if it is named here. idempotentMethods.test.ts
 * checks every name against the generated service descriptor.
 */
export const IDEMPOTENT_METHODS = {
  /** Identical in-flight calls may share one request. */
  coalescible: [
    "GetFeatureFlags",
    "ListProgramsConfig",
    "GetVCSStatus",
    "GetSessionDiff",
    "ListFiles",
    "GetFileContent",
    "StatFile",
    "GetFileTransportCapabilities",
    "ListShells",
    "ListWorkflows",
  ],
  /** Safe to re-send after a transport failure. */
  replayable: [
    "GetFeatureFlags",
    "ListProgramsConfig",
    "GetVCSStatus",
    "GetSessionDiff",
    "ListFiles",
    "GetFileContent",
    "StatFile",
    "GetFileTransportCapabilities",
    "ListShells",
    "ListWorkflows",
  ],
  /** May be sent over the unary WebSocket; must equal ws-unary-allowlist.json's wsUnaryAllowed. */
  wsEligible: [
    "GetVCSStatus",
    "GetSessionDiff",
    "ListFiles",
    "GetFileContent",
    "StatFile",
    "GetFileTransportCapabilities",
    "ListShells",
    "ListWorkflows",
    "ListProgramsConfig",
  ],
  /** Kept in a client cache between calls; separate so enabling one purpose never enables another. */
  cacheable: ["GetFeatureFlags", "ListProgramsConfig", "GetFileTransportCapabilities"],
} as const;

const COALESCIBLE = new Set<string>(IDEMPOTENT_METHODS.coalescible);

export function isCoalescible(methodName: string): boolean {
  return COALESCIBLE.has(methodName);
}

/** Key for "identical call": the fully qualified method plus the canonical JSON of its request. */
export function buildDedupeKey<I extends DescMessage>(
  method: DescMethodUnary<I, DescMessage>,
  message: MessageShape<I>,
): string {
  return `${method.parent.typeName}/${method.name}|${toJsonString(method.input, message)}`;
}
