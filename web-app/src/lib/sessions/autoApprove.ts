// Agents auto-approve (yolo mode) can inject a bypass flag for. Mirrors the backend's
// yoloFlagByAgent map (session/instance_tmux.go) -- keep in sync manually; small enough
// surface that a shared RPC isn't warranted for two agents.
const AUTO_APPROVE_SUPPORTED_AGENTS = ["claude", "aider"];

// isAutoApproveSupported reports whether program is a recognized agent auto-approve can
// inject a bypass flag for. An empty program ("System default" at create time) is treated
// as supported -- the option's own label documents it resolves to claude, and the server
// re-validates against the actually-resolved program anyway (CreateSession's
// AutoApproveSupported guard), so this is only an optimistic UI default, not a
// correctness gap. A persisted session's program is always server-resolved and non-empty.
export function isAutoApproveSupported(program: string): boolean {
  if (program.trim() === "") return true;
  const base = program.trim().split(/\s+/)[0]?.split("/").pop() ?? "";
  return AUTO_APPROVE_SUPPORTED_AGENTS.includes(base);
}

// Programs the pi approval extension (ssq-approval.ts, Phase 4) covers. Deliberately
// narrower than AUTO_APPROVE_SUPPORTED_AGENTS above -- "claude" enforces approval rules
// via its own hook mechanism (not this extension), and "opencode" has its own out-of-scope
// hook, so neither belongs here even though both are legitimate agents elsewhere.
const APPROVAL_EXTENSION_SUPPORTED_AGENTS = ["pi"];

// isApprovalExtensionSupported reports whether program is a recognized agent the pi
// approval extension applies to. Used to decide whether the creation panel's capability
// warning area is even eligible to render for the selected program (Story 3.1.2) -- the
// warning itself still depends on the program's live extension-health signal (Phase 4).
export function isApprovalExtensionSupported(program: string): boolean {
  const base = program.trim().split(/\s+/)[0]?.split("/").pop() ?? "";
  return APPROVAL_EXTENSION_SUPPORTED_AGENTS.includes(base);
}
