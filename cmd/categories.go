package cmd

// Standard command categories for organizing help display
const (
	CategorySession      Category = "Session Management"
	CategoryGit          Category = "Git Integration"
	CategoryVC           Category = "Version Control" // VC tab operations (Git/Jujutsu)
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
	CategoryVC,
	CategoryOrganization,
	CategoryNavigation,
	CategorySystem,
	CategoryLegacy,
}
