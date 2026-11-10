package scenario

import "time"

func approvalsBundle(ref time.Time) Bundle {
	return Bundle{
		ApprovalRules: []ApprovalRule{
			{ID: "demo-git-push-ask", Name: "Ask before git push", ToolName: "Bash", CommandPattern: "^git push", Decision: "AUTO_DECISION_ESCALATE", RiskLevel: "medium", Priority: 50},
			{ID: "demo-read-allow", Name: "Allow file reads", ToolName: "Read", CommandPattern: ".*", Decision: "AUTO_DECISION_ALLOW", RiskLevel: "low", Priority: 10},
			{ID: "demo-rm-deny", Name: "Deny recursive delete", ToolName: "Bash", CommandPattern: "^rm -rf", Decision: "AUTO_DECISION_DENY", RiskLevel: "high", Priority: 90},
		},
		Terminals: []Terminal{{ID: "appr-pending", Title: "refactor-session-store", Transcript: "approval-pending.txt"}},
	}
}
