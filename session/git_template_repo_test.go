package session

import (
	"os"
	"sync"
)

// gitTemplateRepos caches one committed repo per branch name. Copying it replaces
// six git subprocesses per test (init, two configs, add, commit), whose
// fork/exec serializes on the process-wide fork lock when many parallel tests
// set up repos at once.
var gitTemplateRepos = struct {
	mu   sync.Mutex
	dirs map[string]string
}{dirs: map[string]string{}}

func removeGitTemplateRepos() {
	gitTemplateRepos.mu.Lock()
	defer gitTemplateRepos.mu.Unlock()
	for _, dir := range gitTemplateRepos.dirs {
		_ = os.RemoveAll(dir)
	}
}
