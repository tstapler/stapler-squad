// Package norawgitopen is a deliberate norawgitopen violation; never import it.
package norawgitopen

import git "github.com/go-git/go-git/v5"

// Violation calls go-git's PlainOpen directly.
func Violation(path string) { _, _ = git.PlainOpen(path) }
