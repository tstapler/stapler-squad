package session

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateSessionLifecycleDiagram = flag.Bool("update-session-lifecycle-diagram", false, "regenerate the session lifecycle reference")

func sessionLifecycleReference(defs []TransitionDef) string {
	edges := make([]string, 0, len(defs))
	for _, def := range defs {
		edges = append(edges, fmt.Sprintf("    %s --> %s", def.From.String(), def.To.String()))
	}
	sort.Strings(edges)
	return "# Session lifecycle\n\n" +
		"Generated from `session/state_machine.go`'s `transitionDefs`. Regenerate with:\n\n" +
		"```sh\ngo test ./session -run '^TestSessionLifecycleDiagram$' -update-session-lifecycle-diagram\n```\n\n" +
		"```mermaid\n%%{init: {'theme': 'base', 'themeVariables': {'fontFamily': 'ui-sans-serif, system-ui'}}}%%\nstateDiagram-v2\n" +
		strings.Join(edges, "\n") + "\n```\n\n" +
		"This graph shows table-validated lifecycle transitions, not every status write. " +
		"`Restoring` and `PermanentlyFailed` are statuses outside this transition table. " +
		"Detected agent states (processing, idle, approval/input required), retry budgets, " +
		"and `ArchivedAt` are separate dimensions; they are not nodes in this lifecycle graph. " +
		"Edges describe legality, not permission to auto-revive an archived session.\n"
}

func TestSessionLifecycleDiagram(t *testing.T) {
	path := filepath.Join("..", "docs", "reference", "session-lifecycle.md")
	want := sessionLifecycleReference(transitionDefs)
	if *updateSessionLifecycleDiagram {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatal("session lifecycle diagram is stale; run go test ./session -run '^TestSessionLifecycleDiagram$' -update-session-lifecycle-diagram")
	}
}

func TestSessionLifecycleReference_DeterministicAndSensitiveToEdges(t *testing.T) {
	t.Parallel()
	forward := []TransitionDef{{From: Active, To: Stopped}, {From: Stopped, To: Active}}
	reverse := []TransitionDef{forward[1], forward[0]}
	if sessionLifecycleReference(forward) != sessionLifecycleReference(reverse) {
		t.Fatal("diagram must not depend on transition declaration order")
	}
	if sessionLifecycleReference(forward) == sessionLifecycleReference(forward[:1]) {
		t.Fatal("diagram must reflect a removed runtime transition")
	}
}
