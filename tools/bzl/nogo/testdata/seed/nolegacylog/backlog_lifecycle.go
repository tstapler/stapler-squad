// Package nolegacylog is a deliberate nolegacylog violation (the basename backlog_lifecycle.go is protected); never import it.
package nolegacylog

import "github.com/tstapler/stapler-squad/log"

// Violation calls the legacy log.<Level>Log().Printf API.
func Violation(err error) { log.WarningLog().Printf("seed: %v", err) }
