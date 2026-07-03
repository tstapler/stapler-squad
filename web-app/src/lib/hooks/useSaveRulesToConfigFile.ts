import { useCallback } from "react";
import type { ApprovalRuleProto } from "@/gen/session/v1/types_pb";

type SaveParams = { ruleIds: string[] } | { rule: Partial<ApprovalRuleProto> };

// ponytail: stub — SaveRulesToConfigFile RPC not yet implemented; hook exists to satisfy import
export function useSaveRulesToConfigFile() {
  const loading = false;

  const saveToConfigFile = useCallback(async (_params: SaveParams): Promise<{ filePath: string }> => {
    throw new Error("SaveRulesToConfigFile: not yet implemented");
  }, []);

  return { saveToConfigFile, loading };
}
