import { ApprovalRuleProto } from "@/gen/session/v1/types_pb";
import { describeRule } from "@/lib/describeRule";

interface MatchDescriptionProps {
  rule: ApprovalRuleProto;
  matchChipClass?: string;
}

export function MatchDescription({ rule, matchChipClass }: MatchDescriptionProps) {
  const desc = describeRule(rule);

  return (
    <div>
      {desc.isRegex ? (
        <code className={matchChipClass}>{desc.primary}</code>
      ) : (
        <span style={{ fontSize: "0.8rem" }}>{desc.primary}</span>
      )}
      {desc.secondary && (
        <div style={{ fontSize: "0.75rem", opacity: 0.65, marginTop: "2px" }}>
          {desc.secondary}
        </div>
      )}
    </div>
  );
}
