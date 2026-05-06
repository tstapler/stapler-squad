import type { OmnibarFormState } from "@/components/sessions/Omnibar";

type SessionTypeValue = OmnibarFormState["sessionType"];

export const KNOWN_SLASH_COMMANDS: Record<string, SessionTypeValue> = {
  oneoff: "one_off",
  worktree: "new_worktree",
  dir: "directory",
  existing: "existing_worktree",
  project: "new_project",
};

export interface SlashCommandResult {
  sessionType: SessionTypeValue;
  remainder: string;
}

/**
 * Parses a `/command [remainder]` prefix from the omnibar input.
 * Returns null for any input that isn't a known slash command.
 * Unknown `/foo` inputs fall through to normal detection (e.g. LocalPathDetector).
 *
 * Must be called BEFORE detect() in the debounce effect so the slash prefix
 * is consumed before path detectors can misidentify it.
 */
export function parseSlashCommand(input: string): SlashCommandResult | null {
  const match = /^\/([a-z]+)(?:\s+(.*))?$/i.exec(input.trim());
  if (!match) return null;
  const cmd = match[1].toLowerCase();
  const sessionType = KNOWN_SLASH_COMMANDS[cmd];
  if (!sessionType) return null;
  return { sessionType, remainder: match[2]?.trim() ?? "" };
}
