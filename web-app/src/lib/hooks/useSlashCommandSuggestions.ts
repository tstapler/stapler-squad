import { useMemo } from "react";
import type { SlashCommandInfo } from "./useSlashCommands";

// Matches a /word at the cursor — the command name is still being typed (no space yet).
// Works both for whole-input (omnibar) and cursor-word (textarea) detection.
const SLASH_WORD = /\/([a-zA-Z0-9:_-]*)$/;

export interface SlashCommandSuggestionState {
  /** True when the active word (up to cursor) starts with /. */
  isActive: boolean;
  /** Typed portion of the command name, without the leading /. */
  query: string;
  /** Filtered commands matching query as a prefix. */
  suggestions: SlashCommandInfo[];
  /** Index of the first character of the active /-word in the full value string. */
  wordStart: number;
  /** Index just past the last character of the active /-word (= cursorPos). */
  wordEnd: number;
  /**
   * Replace the active /-word in `value` with the selected command.
   * Returns { newValue, newCursorPos } ready to set on the input.
   */
  complete: (value: string, cmd: SlashCommandInfo) => { newValue: string; newCursorPos: number };
}

/**
 * Detects /command autocomplete at the cursor in any text input or textarea.
 *
 * @param value      Current full text value.
 * @param cursorPos  Cursor position within value (use input.selectionStart).
 * @param commands   Available slash commands from useSlashCommands.
 */
export function useSlashCommandSuggestions(
  value: string,
  cursorPos: number,
  commands: SlashCommandInfo[]
): SlashCommandSuggestionState {
  return useMemo<SlashCommandSuggestionState>(() => {
    const textUpToCursor = value.slice(0, cursorPos);
    const match = textUpToCursor.match(SLASH_WORD);

    if (!match) {
      return { isActive: false, query: "", suggestions: [], wordStart: 0, wordEnd: 0, complete };
    }

    const query = match[1].toLowerCase();
    const wordStart = cursorPos - match[0].length;
    const wordEnd = cursorPos;

    const suggestions = query
      ? commands.filter((c) => c.name.toLowerCase().startsWith(query))
      : commands;

    function complete(v: string, cmd: SlashCommandInfo) {
      const insertion = `/${cmd.name} `;
      const newValue = v.slice(0, wordStart) + insertion + v.slice(wordEnd);
      return { newValue, newCursorPos: wordStart + insertion.length };
    }

    return { isActive: true, query, suggestions, wordStart, wordEnd, complete };
  }, [value, cursorPos, commands]);
}
