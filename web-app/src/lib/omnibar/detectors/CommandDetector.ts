import { DetectionResult, InputType } from "../types";
import { Detector } from "../detector";

/**
 * CommandDetector — detects VS Code-style `>command` prefix.
 * Priority 5 — runs before all other detectors.
 *
 * Recognized commands:
 *   >theme matrix | cyberpunk77 | wh40k | clean | light | dark
 *   >go sessions | review | history
 *   >shell [dir] [-- command]   — open a terminal session, optionally rooted at
 *     `dir` and/or running `command` as the session program instead of a plain shell.
 */

/** Splits the raw text after `>shell` into an optional directory and an optional `-- command` tail. */
export function parseShellArgs(rawArgs: string | undefined): { dir?: string; command?: string } {
  const trimmed = (rawArgs ?? "").trim();
  if (!trimmed) return {};

  const tokens = trimmed.split(/\s+/);
  const dashIndex = tokens.indexOf("--");
  if (dashIndex === -1) return { dir: trimmed };

  const dir = tokens.slice(0, dashIndex).join(" ").trim();
  const command = tokens.slice(dashIndex + 1).join(" ").trim();
  return { dir: dir || undefined, command: command || undefined };
}

export class CommandDetector implements Detector {
  name = "CommandDetector";
  priority = 5;

  // Map of command string → type and argument
  private static COMMANDS: Array<{
    pattern: RegExp;
    commandType: "theme" | "navigate" | "spawn_shell";
    commandArg: string;
    suggestedName: string;
  }> = [
    { pattern: /^>theme\s+matrix$/i, commandType: "theme", commandArg: "matrix", suggestedName: "Switch to Matrix theme" },
    { pattern: /^>theme\s+cyberpunk77$/i, commandType: "theme", commandArg: "cyberpunk77", suggestedName: "Switch to Cyberpunk 77 theme" },
    { pattern: /^>theme\s+wh40k$/i, commandType: "theme", commandArg: "wh40k", suggestedName: "Switch to WH40K theme" },
    { pattern: /^>theme\s+clean$/i, commandType: "theme", commandArg: "clean", suggestedName: "Switch to Clean theme" },
    { pattern: /^>theme\s+light$/i, commandType: "theme", commandArg: "light", suggestedName: "Switch to Light theme" },
    { pattern: /^>theme\s+dark$/i, commandType: "theme", commandArg: "dark", suggestedName: "Switch to Dark theme" },
    { pattern: /^>go\s+sessions?$/i, commandType: "navigate", commandArg: "/", suggestedName: "Go to Sessions" },
    { pattern: /^>go\s+review$/i, commandType: "navigate", commandArg: "/review-queue", suggestedName: "Go to Review Queue" },
    { pattern: /^>go\s+history$/i, commandType: "navigate", commandArg: "/history", suggestedName: "Go to History" },
    { pattern: /^>shell(?:\s+(.+))?$/i, commandType: "spawn_shell", commandArg: "", suggestedName: "Spawn new shell" },
  ];

  detect(input: string): DetectionResult | null {
    const trimmed = input.trim();
    if (!trimmed.startsWith(">")) return null;

    for (const cmd of CommandDetector.COMMANDS) {
      const match = trimmed.match(cmd.pattern);
      if (match) {
        if (cmd.commandType === "spawn_shell") {
          const { dir, command } = parseShellArgs(match[1]);
          const suggestedName = command
            ? `Run "${command}"${dir ? ` in ${dir}` : ""}`
            : dir
            ? `Open terminal in ${dir}`
            : "Open terminal";
          return {
            type: InputType.SpawnShell,
            confidence: 1.0,
            parsedValue: trimmed,
            suggestedName,
            metadata: { commandType: cmd.commandType, shellDir: dir, shellCommand: command },
          };
        }
        return {
          type: InputType.Command,
          confidence: 1.0,
          parsedValue: trimmed,
          suggestedName: cmd.suggestedName,
          metadata: {
            commandType: cmd.commandType,
            commandArg: cmd.commandArg,
          },
        };
      }
    }

    // Unrecognized > command — still return a result with low confidence
    if (trimmed.length > 1) {
      return {
        type: InputType.Unknown,
        confidence: 0.3,
        parsedValue: trimmed,
        suggestedName: "Unknown command",
        metadata: {},
      };
    }

    return null;
  }
}
