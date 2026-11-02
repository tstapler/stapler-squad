// Package tags is the S7 spike: one file per build tag, each registering itself in Active.
package tags

import "sort"

var active []string

func register(s string) { active = append(active, s) }

// Active returns the sorted list of tags whose files were compiled in.
func Active() []string {
	out := append([]string(nil), active...)
	sort.Strings(out)
	return out
}
