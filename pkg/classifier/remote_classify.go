package classifier

// RemoteClassifyProtocolVersion is stamped on every POST /api/hooks/classify response. The
// client rejects any response without it, so a stray service answering on the same port with
// `{}` can never be mistaken for a decision (ClassificationDecision's zero value is AutoAllow).
const RemoteClassifyProtocolVersion = 1

// RemoteClassifyRequest is the POST /api/hooks/classify request body.
type RemoteClassifyRequest struct {
	Payload PermissionRequestPayload `json:"payload"`
	// Env holds the caller's value for every $VAR referenced by a Bash command. The server
	// expands from this map only, never its own environment (ClassificationContext.IsolatedEnv), so a command is judged on the
	// same text the caller's shell would run.
	Env map[string]string `json:"env,omitempty"`
}

// RemoteClassifyResponse is the POST /api/hooks/classify response body.
type RemoteClassifyResponse struct {
	Version int                  `json:"version"`
	Result  ClassificationResult `json:"result"`
	// ConfigDir is the server's state directory. The client uses the answer only when this
	// matches the directory it would have loaded rules from itself (instance, workspace and
	// preferred-workspace selection all change it).
	ConfigDir string `json:"config_dir"`
}

// ReferencedEnvVars returns the distinct variable names a command references via $VAR or
// ${VAR}, in first-seen order.
func ReferencedEnvVars(cmd string) []string {
	var names []string
	seen := map[string]bool{}
	for _, m := range envVarRefPattern.FindAllStringSubmatch(cmd, -1) {
		name := m[1]
		if name == "" {
			name = m[2]
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}
