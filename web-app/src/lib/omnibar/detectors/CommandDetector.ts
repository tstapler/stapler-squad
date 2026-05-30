import { DetectionResult, InputType } from "../types";
import { Detector } from "../detector";

/**
 * CommandDetector — detects VS Code-style `>command` prefix.
 * Priority 5 — runs before all other detectors.
 *
 * Recognized commands:
 *   >theme matrix | cyberpunk77 | wh40k | clean | light | dark
 *   >go sessions | review | history
 *   >shell [optional name/command]
 */
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
        const resultType = cmd.commandType === "spawn_shell" ? InputType.SpawnShell : InputType.Command;
        // For >shell, capture the optional argument (name/command) from the capture group
        const commandArg = cmd.commandType === "spawn_shell" ? (match[1] ?? "") : cmd.commandArg;
        return {
          type: resultType,
          confidence: 1.0,
          parsedValue: trimmed,
          suggestedName: cmd.suggestedName,
          metadata: {
            commandType: cmd.commandType,
            commandArg,
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
