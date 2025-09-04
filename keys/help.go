package keys

// HelpCategory organizes commands by function
type HelpCategory string

const (
	HelpCategoryManaging   HelpCategory = "Managing"
	HelpCategoryHandoff    HelpCategory = "Handoff"
	HelpCategoryNavigation HelpCategory = "Navigation"
	HelpCategoryOrganize   HelpCategory = "Organization"
	HelpCategoryOther      HelpCategory = "Other"
	HelpCategoryUncategory HelpCategory = "Uncategorized" // For keys without categories
	HelpCategorySpecial    HelpCategory = "Special"       // For special keys that shouldn't show in main help
)

// KeyHelpInfo adds extended help information to key bindings
type KeyHelpInfo struct {
	Description string       // Extended description for help text
	Category    HelpCategory // Category for organizing in help screens
}

// KeyHelpMap maps KeyNames to their help information
var KeyHelpMap = map[KeyName]KeyHelpInfo{
	// Managing category
	KeyNew:    {Description: "Create a new session", Category: HelpCategoryManaging},
	KeyPrompt: {Description: "Create a new session with a prompt (Vim-like command mode)", Category: HelpCategoryManaging},
	KeyKill:   {Description: "Kill (delete) the selected session", Category: HelpCategoryManaging},
	KeyEnter:  {Description: "Attach to the selected session", Category: HelpCategoryManaging},

	// Handoff category
	KeySubmit:   {Description: "Push branch (legacy - use 'g' for git workflow)", Category: HelpCategoryHandoff},
	KeyCheckout: {Description: "Checkout: commit changes and pause session", Category: HelpCategoryHandoff},
	KeyResume:   {Description: "Resume a paused session", Category: HelpCategoryHandoff},

	// Organization category
	KeySearch:       {Description: "Search sessions by title (Vim-style search)", Category: HelpCategoryOrganize},
	KeyRight:        {Description: "Expand selected category (Vim h/j/k/l navigation)", Category: HelpCategoryOrganize},
	KeyLeft:         {Description: "Collapse selected category (Vim h/j/k/l navigation)", Category: HelpCategoryOrganize},
	KeyToggleGroup:  {Description: "Toggle expand/collapse category", Category: HelpCategoryOrganize},
	KeyFilterPaused: {Description: "Toggle visibility of paused sessions", Category: HelpCategoryOrganize},
	KeyClearFilters: {Description: "Clear all filters and search", Category: HelpCategoryOrganize},
	KeyGit:          {Description: "Open git status interface (fugitive-style)", Category: HelpCategoryHandoff},

	// Navigation category
	KeyUp:        {Description: "Navigate up (Vim j/k keys supported)", Category: HelpCategoryNavigation},
	KeyDown:      {Description: "Navigate down (Vim j/k keys supported)", Category: HelpCategoryNavigation},
	KeyShiftUp:   {Description: "Scroll up (Vim Ctrl+u supported)", Category: HelpCategoryNavigation},
	KeyShiftDown: {Description: "Scroll down (Vim Ctrl+d supported)", Category: HelpCategoryNavigation},

	// Other category
	KeyTab:  {Description: "Switch between preview and diff tabs", Category: HelpCategoryOther},
	KeyEsc:  {Description: "Cancel/exit current mode", Category: HelpCategoryOther},
	KeyQuit: {Description: "Quit the application", Category: HelpCategoryOther},
	KeyHelp: {Description: "Show help screen", Category: HelpCategoryOther},

	// Special category (not shown in main help)
	KeySubmitName: {Description: "Submit name for new instance", Category: HelpCategorySpecial},
	KeyReview:     {Description: "Review code", Category: HelpCategorySpecial},
	KeyPush:       {Description: "Push changes", Category: HelpCategorySpecial},
}

// GetKeyHelp returns the help information for a key
func GetKeyHelp(keyName KeyName) KeyHelpInfo {
	info, exists := KeyHelpMap[keyName]
	if !exists {
		// Return default help for unknown keys
		return KeyHelpInfo{
			Description: "No description",
			Category:    HelpCategoryUncategory,
		}
	}
	return info
}

// GetKeysInCategory returns all key bindings in a given category
func GetKeysInCategory(category HelpCategory) []KeyName {
	var keys []KeyName
	for k, info := range KeyHelpMap {
		if info.Category == category {
			keys = append(keys, k)
		}
	}
	return keys
}

// GetAllCategories returns all categories that have at least one key
func GetAllCategories() []HelpCategory {
	categoryMap := make(map[HelpCategory]bool)
	for _, info := range KeyHelpMap {
		categoryMap[info.Category] = true
	}

	// Convert map to slice
	categories := make([]HelpCategory, 0, len(categoryMap))
	for category := range categoryMap {
		// Skip special category
		if category != HelpCategorySpecial {
			categories = append(categories, category)
		}
	}

	return categories
}
