package domain

import "cmp"

// TmuxNameTitle is the title to derive the tmux session name from. A rename keeps
// the running tmux session under its old name, so deriving from the new title would
// miss it and start a second copy.
func (d InstanceData) TmuxNameTitle() string { return cmp.Or(d.TmuxTitle, d.Title) }

// TmuxNameTitle is InstanceData.TmuxNameTitle for a snapshot.
func (s *InstanceSnapshot) TmuxNameTitle() string { return cmp.Or(s.TmuxTitle, s.Title) }

// ClaimsTitle reports whether a new session may not take title: it is this session's
// title, or the name its still-running tmux session was left under by a rename.
func (d InstanceData) ClaimsTitle(title string) bool {
	return d.Title == title || (d.ArchivedAt == nil && d.TmuxTitle == title)
}
