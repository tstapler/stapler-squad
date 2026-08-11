export function formatPauseReason(reason: string): string {
  switch (reason) {
    case "manual": return "Paused manually";
    case "auto:inactivity": return "Paused automatically — no recent activity";
    case "auto:session_limit": return "Paused automatically — too many active sessions";
    case "auto:resource": return "Paused automatically — resource pressure";
    default: return "Paused";
  }
}
