import { useMemo } from "react";
import type { AliasEntry } from "./useAliases";

export function useAliasSuggestions(input: string, aliases: AliasEntry[]) {
  const isAliasBrowse = input === "@";
  const isAliasCompletion = /^@[\w-]+$/.test(input) && input.length > 1;

  const filteredAliases = useMemo(() => {
    if (!isAliasBrowse && !isAliasCompletion) return [];
    const partial = input.slice(1).toLowerCase();
    if (!partial) return aliases;
    return aliases.filter(
      (a) =>
        a.name.toLowerCase().includes(partial) ||
        a.description.toLowerCase().includes(partial) ||
        a.group.toLowerCase().includes(partial)
    );
  }, [input, aliases, isAliasBrowse, isAliasCompletion]);

  function complete(alias: AliasEntry): string {
    return "@" + alias.name + " ";
  }

  return { isAliasBrowse, isAliasCompletion, filteredAliases, complete };
}
