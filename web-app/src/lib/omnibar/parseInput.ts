import { toSessionSlug } from "./slugify";

export interface ParsedInput {
  name: string;
  firstPrompt: string;
}

/**
 * Splits bare-text omnibar input on the first `>` character.
 * Only call this when detection type is SessionSearch — never on path or URL inputs.
 *
 * "my feature > implement auth" → { name: "my-feature", firstPrompt: "implement auth" }
 * "my feature"                  → { name: "my-feature", firstPrompt: "" }
 * "> implement auth"            → { name: "", firstPrompt: "implement auth" }
 */
export function parseInputWithSeparator(input: string): ParsedInput {
  const idx = input.indexOf(">");
  if (idx === -1) {
    return { name: toSessionSlug(input), firstPrompt: "" };
  }
  const rawName = input.slice(0, idx);
  const firstPrompt = input.slice(idx + 1).trim();
  return { name: toSessionSlug(rawName), firstPrompt };
}
