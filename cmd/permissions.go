package cmd

import (
	"github.com/tstapler/stapler-squad/session"
)

// CommandPermissionRequirements maps command IDs to their required permissions
// Commands not in this map are considered always available (navigation, system commands)
var commandPermissionRequirements = map[CommandID]func(perms session.InstancePermissions) bool{
	// Session management commands
	"session.new":             func(perms session.InstancePermissions) bool { return perms.CanView }, // Anyone can attempt to create
	"session.kill":            func(perms session.InstancePermissions) bool { return perms.CanDestroy },
	"session.attach":          func(perms session.InstancePermissions) bool { return perms.CanAttach },
	"session.checkout":        func(perms session.InstancePermissions) bool { return perms.CanPause && perms.CanModifyGit },
	"session.resume":          func(perms session.InstancePermissions) bool { return perms.CanResume },
	"session.claude_settings": func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"session.tag_editor":      func(perms session.InstancePermissions) bool { return perms.CanModifyGit },

	// Git integration commands
	"git.status":        func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"git.stage":         func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"git.unstage":       func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"git.toggle":        func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"git.unstage_all":   func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"git.commit":        func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"git.commit_amend":  func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"git.diff":          func(perms session.InstancePermissions) bool { return perms.CanView }, // Read-only
	"git.push":          func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"git.pull":          func(perms session.InstancePermissions) bool { return perms.CanModifyGit },
	"git.legacy_submit": func(perms session.InstancePermissions) bool { return perms.CanModifyGit },

	// PTY management commands
	"pty.attach":       func(perms session.InstancePermissions) bool { return perms.CanAttach },
	"pty.send_command": func(perms session.InstancePermissions) bool { return perms.CanSendCommand },
	"pty.disconnect":   func(perms session.InstancePermissions) bool { return perms.CanAttach },

	// Navigation and organization commands that require only view permission
	"nav.search":               func(perms session.InstancePermissions) bool { return perms.CanView },
	"nav.next_review":          func(perms session.InstancePermissions) bool { return perms.CanView && perms.CanAddToQueue },
	"nav.previous_review":      func(perms session.InstancePermissions) bool { return perms.CanView && perms.CanAddToQueue },
	"nav.toggle_review_queue":  func(perms session.InstancePermissions) bool { return perms.CanView && perms.CanAddToQueue },
	"org.filter_paused":        func(perms session.InstancePermissions) bool { return perms.CanView },
	"org.clear_filters":        func(perms session.InstancePermissions) bool { return perms.CanView },
	"org.toggle_group":         func(perms session.InstancePermissions) bool { return perms.CanView },
	"pty.toggle_view":          func(perms session.InstancePermissions) bool { return perms.CanView },
	"pty.refresh":              func(perms session.InstancePermissions) bool { return perms.CanView },

	// Basic navigation always available if CanView is true
	"nav.up":        func(perms session.InstancePermissions) bool { return perms.CanView },
	"nav.down":      func(perms session.InstancePermissions) bool { return perms.CanView },
	"nav.left":      func(perms session.InstancePermissions) bool { return perms.CanView },
	"nav.right":     func(perms session.InstancePermissions) bool { return perms.CanView },
	"nav.page_up":   func(perms session.InstancePermissions) bool { return perms.CanView },
	"nav.page_down": func(perms session.InstancePermissions) bool { return perms.CanView },

	// System commands (always available - not filtered)
	// These are intentionally omitted from the map so they're always shown:
	// sys.help, sys.quit, sys.escape, sys.tab, sys.command_mode, sys.confirm
}

// CheckCommandPermission checks if the given permissions allow executing the command
func CheckCommandPermission(cmdID CommandID, perms session.InstancePermissions) bool {
	// If no permission check is defined, the command is always available (system commands)
	checkFunc, hasRequirement := commandPermissionRequirements[cmdID]
	if !hasRequirement {
		return true // System commands are always available
	}

	// Run the permission check function
	return checkFunc(perms)
}

// FilterCommandsByPermissions filters a list of commands based on instance permissions
func FilterCommandsByPermissions(commands []*Command, perms session.InstancePermissions) []*Command {
	filtered := make([]*Command, 0, len(commands))

	for _, cmd := range commands {
		if CheckCommandPermission(cmd.ID, perms) {
			filtered = append(filtered, cmd)
		}
	}

	return filtered
}
