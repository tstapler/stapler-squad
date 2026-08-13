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
