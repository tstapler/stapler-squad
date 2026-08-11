// +feature: alias-detector
/**
 * AliasDetector detects @alias-name[:branch][ label][ --flags] syntax.
 * Priority 36 — after WorkflowDetector (25) and NewSessionDetector (35), before GitHubShorthandDetector (40).
 *
 * IMPORTANT: AliasDetector must claim ALL input starting with "@" and never return null
 * for such input. This prevents SessionSearchDetector from claiming "@foo" input.
 */

import { Detector } from "../detector";
import { DetectionResult, InputType } from "../types";
import type { AliasEntry } from "../../hooks/useAliases";

/** Shape of `DetectionResult.metadata` when `type === InputType.Alias`. */
export interface AliasMetadata {
  aliasName: string;
  branch?: string;
  label?: string;
  extraFlags?: string;
  alias: AliasEntry;
}

/** Name-only form of the alias regex (without the leading "@"). */
export const ALIAS_NAME_RE = /^[\w-]+$/;

/**
 * AliasDetector matches the full alias grammar:
 *   @<name>[:<branch>][ <label text>][ --<extra-flags>]
 *
 * Three modes:
 *   - "@" alone or "@<partial>" (no space) → AliasBrowse (palette/completion mode)
 *   - "@<name> " (with space, known alias) → Alias (invocation mode)
 *   - "@<name> " (with space, unknown alias) → AliasNotFound
 */
export class AliasDetector implements Detector {
  name = "AliasDetector";
  priority = 36;

  private aliases: AliasEntry[];

  constructor(aliases: AliasEntry[]) {
    this.aliases = aliases;
  }

  detect(input: string): DetectionResult | null {
    // Only claim input that starts with "@"
    if (!input.startsWith("@")) return null;

    const trimmed = input.trimEnd();
    // hasSpace is true when the original input has a space after the "@name" portion,
    // signalling that the user has committed to a name (invocation mode).
    const hasSpace = input !== trimmed || /^@[\w-]+(?::[^\s]+)?\s/.test(input);

    // "@" alone → browse palette
    if (trimmed === "@") {
      return {
        type: InputType.AliasBrowse,
        confidence: 1.0,
        parsedValue: trimmed,
        suggestedName: "",
        metadata: { partial: "" },
      };
    }

    // "@<partial>" with no space → completion/browse mode
    if (/^@[\w-]+$/.test(trimmed) && !hasSpace) {
      const partial = trimmed.slice(1); // strip "@"
      return {
        type: InputType.AliasBrowse,
        confidence: 1.0,
        parsedValue: trimmed,
        suggestedName: partial,
        metadata: { partial },
      };
    }

    // "@<name> ..." with space → invocation mode; parse full grammar
    // Extract just the name+optional-branch prefix to identify the alias
    const nameMatch = trimmed.match(/^@([\w-]+)(?::([^\s]+))?(?:\s|$)/);
    if (!nameMatch) {
      // Starts with @ but doesn't match any recognizable pattern → AliasBrowse fallback
      return {
        type: InputType.AliasBrowse,
        confidence: 0.5,
        parsedValue: trimmed,
        suggestedName: "",
        metadata: { partial: trimmed.slice(1) },
      };
    }

    const aliasName = nameMatch[1];
    const branch = nameMatch[2] ?? undefined;

    // Find the alias (case-insensitive)
    const found = this.aliases.find(
      (a) => a.name.toLowerCase() === aliasName.toLowerCase()
    );

    // Parse the rest of the string (everything after "@name[:branch] ")
    const afterPrefix = trimmed.slice(nameMatch[0].length).trim();
    let label: string | undefined;
    let extraFlags: string | undefined;

    if (afterPrefix) {
      const flagIdx = afterPrefix.indexOf(" --");
      if (flagIdx >= 0) {
        label = afterPrefix.slice(0, flagIdx).trim() || undefined;
        extraFlags = afterPrefix.slice(flagIdx + 1).trim() || undefined;
      } else if (afterPrefix.startsWith("--")) {
        extraFlags = afterPrefix;
      } else {
        label = afterPrefix || undefined;
      }
    }

    if (!found) {
      return {
        type: InputType.AliasNotFound,
        confidence: 1.0,
        parsedValue: trimmed,
        suggestedName: aliasName,
        metadata: { slug: aliasName },
      };
    }

    return {
      type: InputType.Alias,
      confidence: 1.0,
      parsedValue: trimmed,
      suggestedName: found.name,
      metadata: {
        aliasName: found.name,
        branch,
        label,
        extraFlags,
        alias: found,
      },
    };
  }
}
