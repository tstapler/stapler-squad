import type { NotificationData } from "@/lib/types/notification";
import type { RuleBuilderPrefill } from "@/lib/ruleBuilderPrefill";
import type { ApprovalDisplay } from "./ActionRow";

/** Gates descriptor-driven rendering; flag-off keeps the legacy Approve/Deny path. */
export const INLINE_ACTIONS_FLAG = "notification_inline_actions";

/** Gates the question card; its render stage chooses between the card and the plain option buttons. */
export const STRUCTURED_QUESTION_FLAG = "askuserquestion_structured";
export const STRUCTURED_RENDER_SCOPE = "stage:render";

/** Approval fields the server stamps on the notification's metadata. */
export function approvalDisplay(n: NotificationData): ApprovalDisplay {
  const md = n.metadata ?? {};
  const text = md.tool_input_command || md.tool_input_file || undefined;
  const program = md.tool_input_command?.trim().split(/\s+/)[0];
  return {
    tool: md.tool_name || undefined,
    text,
    unrenderable: md.tool_input_unrenderable === "true",
    ruleName: md.classifier_rule_name || undefined,
    ruleScope: [md.tool_name, program].filter(Boolean).join(" ") || undefined,
  };
}

/**
 * The local listener binds loopback; the remote one is reached by another host name.
 * Only gates the More menu, so a wrong answer cannot widen what the server allows.
 */
export function isLocalListener(): boolean {
  if (typeof window === "undefined") return true;
  return ["localhost", "127.0.0.1", "[::1]", "::1"].includes(window.location.hostname);
}

/** Seeds the rule builder from an approval notification. */
export function rulePrefillFor(n: NotificationData): RuleBuilderPrefill {
  const program = n.metadata?.tool_input_command?.trim().split(/\s+/)[0];
  return { toolName: n.metadata?.tool_name, programs: program ? [program] : undefined };
}
