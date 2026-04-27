package backend

import (
	"bufio"
	"os"
	"regexp"
	"time"
)

// BackendFeature represents a single RPC endpoint discovered from proto or marker scanning.
type BackendFeature struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Service      string    `json:"service"`
	Method       string    `json:"method"`
	ProtoFile    string    `json:"protoFile"`
	MarkerFound  bool      `json:"markerFound"`
	HandlerFile  string    `json:"handlerFile,omitempty"`
	Tested       bool      `json:"tested"`
	TestIDs      []string  `json:"testIds"`
	LastModified time.Time `json:"lastModified"`
}

// methodToID maps proto RPC method names to their canonical feature IDs.
var methodToID = map[string]string{
	"CreateSession":            "session:create",
	"GetSession":               "session:get",
	"UpdateSession":            "session:update",
	"DeleteSession":            "session:delete",
	"ListSessions":             "session:list",
	"WatchSessions":            "session:watch",
	"StreamTerminal":           "session:stream-terminal",
	"GetSessionDiff":           "session:get-diff",
	"GetVCSStatus":             "session:get-vcs-status",
	"GetReviewQueue":           "review-queue:get",
	"AcknowledgeSession":       "session:acknowledge",
	"GetLogs":                  "logs:get",
	"WatchReviewQueue":         "review-queue:watch",
	"LogUserInteraction":       "interaction:log",
	"GetClaudeConfig":          "claude-config:get",
	"ListClaudeConfigs":        "claude-config:list",
	"UpdateClaudeConfig":       "claude-config:update",
	"ListClaudeHistory":        "history:list",
	"GetClaudeHistoryDetail":   "history:get-detail",
	"GetClaudeHistoryMessages": "history:get-messages",
	"SearchClaudeHistory":      "history:search",
	"GetPRInfo":                "pr:get-info",
	"GetPRComments":            "pr:get-comments",
	"PostPRComment":            "pr:post-comment",
	"MergePR":                  "pr:merge",
	"ClosePR":                  "pr:close",
	"SendNotification":         "notification:send",
	"FocusWindow":              "window:focus",
	"RenameSession":            "session:rename",
	"RestartSession":           "session:restart",
	"GetWorkspaceInfo":         "workspace:get-info",
	"ListWorkspaceTargets":     "workspace:list-targets",
	"SwitchWorkspace":          "workspace:switch",
	"ResolveApproval":          "approval:resolve",
	"ListPendingApprovals":     "approval:list-pending",
	"CreateDebugSnapshot":      "debug:create-snapshot",
	"GetNotificationHistory":   "notification:get-history",
	"MarkNotificationRead":     "notification:mark-read",
	"ClearNotificationHistory": "notification:clear-history",
	"ListApprovalRules":        "approval:list-rules",
	"UpsertApprovalRule":       "approval:upsert-rule",
	"DeleteApprovalRule":       "approval:delete-rule",
	"GetApprovalAnalytics":     "approval:get-analytics",
	"ListDatabases":            "database:list",
	"GetCurrentDatabase":       "database:get-current",
	"SwitchDatabase":           "database:switch",
	"MergeDatabase":            "database:merge",
	"CreateCheckpoint":         "checkpoint:create",
	"ListCheckpoints":          "checkpoint:list",
	"ForkSession":              "session:fork",
	"ListFiles":                "file:list",
	"GetFileContent":           "file:get-content",
	"SearchFiles":              "file:search",
	"ListPathCompletions":      "path:list-completions",
	"ListWorktrees":            "worktree:list",
	// Project management RPCs
	"CreateProject":           "project:create",
	"ListProjects":            "project:list",
	"UpdateProject":           "project:update",
	"DeleteProject":           "project:delete",
	"AssignSessionsToProject": "project:assign-sessions",
	// Prompt history RPCs
	"ListPromptHistory":   "session:list-prompt-history",
	"DeletePromptHistory": "session:delete-prompt-history",
	// Session execution RPCs
	"RunOneShot":          "session:run-one-shot",
	"BatchCreateSessions": "session:batch-create",
	"GetTerminalSnapshot": "session:get-terminal-snapshot",
	"ListBranches":        "session:list-branches",
	// Profile and defaults RPCs
	"UpsertProfile":        "profile:upsert",
	"DeleteProfile":        "profile:delete",
	"GetSessionDefaults":   "defaults:get",
	"UpdateGlobalDefaults": "defaults:update-global",
	"ResolveDefaults":      "defaults:resolve",
	// Directory rules RPCs
	"UpsertDirectoryRule": "directory-rule:upsert",
	"DeleteDirectoryRule": "directory-rule:delete",
}

// rpcPattern matches lines like:   rpc MethodName(  (indented or not)
var rpcPattern = regexp.MustCompile(`^\s*rpc\s+(\w+)\s*\(`)

// servicePattern matches lines like:  service ServiceName {
var servicePattern = regexp.MustCompile(`^\s*service\s+(\w+)\s*\{`)

// ScanProto reads a proto file and returns BackendFeature entries for each RPC method found.
func ScanProto(protoFile string) ([]BackendFeature, error) {
	info, err := os.Stat(protoFile)
	if err != nil {
		return nil, err
	}
	lastMod := info.ModTime()

	f, err := os.Open(protoFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var features []BackendFeature
	currentService := "SessionService"
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if svcMatches := servicePattern.FindStringSubmatch(line); svcMatches != nil {
			currentService = svcMatches[1]
			continue
		}
		matches := rpcPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		method := matches[1]
		id, ok := methodToID[method]
		if !ok {
			// Fallback: use method name as-is
			id = method
		}
		features = append(features, BackendFeature{
			ID:           id,
			Type:         "backend",
			Service:      currentService,
			Method:       method,
			ProtoFile:    protoFile,
			MarkerFound:  false,
			Tested:       false,
			TestIDs:      []string{},
			LastModified: lastMod,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return features, nil
}
