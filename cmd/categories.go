package cmd

// Standard command categories for organizing help display
const (
	CategorySession      Category = "Session Management"
	CategoryGit          Category = "Git Integration"
	CategoryNavigation   Category = "Navigation"
	CategoryOrganization Category = "Organization"
	CategorySystem       Category = "System"
	CategoryLegacy       Category = "Legacy"
	CategorySpecial      Category = "Special" // Hidden from main help
)

// CategoryOrder defines the display order for help screens
var CategoryOrder = []Category{
	CategorySession,
	CategoryGit,
	CategoryOrganization,
	CategoryNavigation,
	CategorySystem,
	CategoryLegacy,
}

