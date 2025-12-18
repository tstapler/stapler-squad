package cmd

// Standard application contexts
const (
	ContextGlobal    ContextID = "global"
	ContextList      ContextID = "list"
	ContextPTYList   ContextID = "pty-list"
	ContextGitStatus ContextID = "git-status"
	ContextVCTab     ContextID = "vc-tab"
	ContextHelp      ContextID = "help"
	ContextPrompt    ContextID = "prompt"
	ContextSearch    ContextID = "search"
	ContextConfirm   ContextID = "confirm"
)

// InitializeContexts sets up the context hierarchy
func InitializeContexts(registry *CommandRegistry) error {
	contexts := []*Context{
		{
			ID:          ContextGlobal,
			Name:        "Global",
			Description: "Commands available in all contexts",
		},
		{
			ID:          ContextList,
			Name:        "Session List",
			Parent:      ptr(ContextGlobal),
			Description: "Commands available when viewing the session list",
		},
		{
			ID:          ContextPTYList,
			Name:        "PTY List",
			Parent:      ptr(ContextGlobal),
			Description: "Commands available when viewing the PTY list",
		},
		{
			ID:          ContextGitStatus,
			Name:        "Git Status",
			Parent:      ptr(ContextGlobal),
			Description: "Commands available in the fugitive-style git status interface",
		},
		{
			ID:          ContextVCTab,
			Name:        "VC Tab",
			Parent:      ptr(ContextList),
			Description: "Commands available in the version control tab",
		},
		{
			ID:          ContextHelp,
			Name:        "Help Screen",
			Parent:      ptr(ContextGlobal),
			Description: "Commands available when viewing help",
		},
		{
			ID:          ContextPrompt,
			Name:        "Prompt Input",
			Parent:      ptr(ContextGlobal),
			Description: "Commands available when entering text input",
		},
		{
			ID:          ContextSearch,
			Name:        "Search Mode",
			Parent:      ptr(ContextList),
			Description: "Commands available when searching sessions",
		},
		{
			ID:          ContextConfirm,
			Name:        "Confirmation Dialog",
			Parent:      ptr(ContextGlobal),
			Description: "Commands available in confirmation dialogs",
		},
	}

	for _, ctx := range contexts {
		if err := registry.RegisterContext(ctx); err != nil {
			return err
		}
	}

	return nil
}

// ptr is a helper to get a pointer to a value
func ptr[T any](v T) *T {
	return &v
}
