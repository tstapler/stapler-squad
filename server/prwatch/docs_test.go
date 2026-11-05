package prwatch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/prwatch"
)

const operatorDocPath = "../../docs/how-to/inbox-pr-watch.md"

func readOperatorDoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash(operatorDocPath))
	if err != nil {
		t.Fatalf("operator doc missing: %v", err)
	}
	return string(b)
}

func TestOperatorDoc_should_ContainEveryRequiredSection_When_CheckedAgainstConfigAndPolicy(t *testing.T) {
	doc := readOperatorDoc(t)
	var missing []string
	need := func(what, needle string) {
		if !strings.Contains(doc, needle) {
			missing = append(missing, what+": "+needle)
		}
	}

	for _, key := range config.InboxPRWatchConfigKeys() {
		need("config key", "| `"+key+"` |")
	}
	for _, r := range prwatch.AllDropReasons() {
		need("drop reason", "`"+string(r)+"`")
	}
	for _, r := range prwatch.AllHoldReasons() {
		need("hold reason", "`"+string(r)+"`")
	}
	for _, r := range prwatch.AllSkipReasons() {
		need("skip reason", "`"+string(r)+"`")
	}
	for _, c := range prwatch.AllActorClasses() {
		need("actor class", "`"+string(c)+"`")
	}
	for _, title := range prwatch.AllNotificationTitles() {
		need("notification title", "`"+title+"`")
	}

	for _, s := range []struct{ what, needle string }{
		{"prerequisites section", "## Prerequisites"},
		{"enable and disable section", "## Turn the feature on and off"},
		{"feature flag key", "feature_flags"},
		{"feature flag name", "inbox_pr_watch"},
		{"flag read live", "reads the flag live"},
		{"subscriptions persist when off", "subscriptions stay in the inbox"},
		{"token mode", "mode 0600"},
		{"gh auth", "gh auth status"},
		{"watch command", "gh-signal inbox watch"},
		{"live versus restart", "needs a restart"},
		{"status URL", "http://localhost:8543/actuator/inbox-pr-watch?format=text"},
		{"status sample", "PR watch status"},
		{"status sample counts", "Counts since start:"},
		{"symptom table", "## Symptoms and fixes"},
		{"verify section", "## Verify it works"},
		{"pane prefix", "[stapler-squad automated notice]"},
		{"own web UI comments dropped", "comments you type in the GitHub web UI"},
		{"steer content section", "## What the steer says"},
		{"steer never contains", "never contains comment text"},
		{"residual risk", "residual risk"},
		{"human_feedback_mode", "`human_feedback_mode`"},
		{"actor_class", "`actor_class`"},
		{"StrangerActivity notice", "StrangerActivity"},
		{"copilot allowlist", "copilot[bot]"},
		{"opt out section", "## Opt out"},
		{"no per-session snooze", "no per-session snooze"},
		{"paused section", "## While a session is paused or hibernated, and while the flag is off"},
		{"catch-up on resume", "catch-up"},
		{"stale replay section", "## When you turn it back on"},
		{"stale replay rule", "younger than `held_age_ceiling_hours`"},
		{"notifications section", "## Notifications and what to do"},
		{"cleanup section", "## Clean up subscriptions"},
		{"list subscriptions command", "gh-signal inbox subscriptions --consumer squad:<hostname>"},
		{"unsubscribe command", "gh-signal inbox unsubscribe <sub_id> --consumer squad:<hostname>"},
		{"hostname change", "hostname changes"},
		{"old hostname", "use the old hostname"},
		{"discovery window", "1 to 6 minutes"},
		{"glossary", "## Glossary"},
		{"glossary steer", "**steer**"},
		{"glossary waiting", "**waiting**"},
		{"glossary not steered", "**not steered**"},
		{"glossary covered", "**covered**"},
		{"glossary window", "**window**"},
		{"history resets on restart", "resets when Squad restarts"},
		{"remote listener unverified", "UNVERIFIED"},
		{"remote listener unsupported", "not a supported way"},
		{"coalescing window", "## What a batching window does"},
		{"window announces later", "announced when the window closes, not lost"},
	} {
		need(s.what, s.needle)
	}

	if len(missing) > 0 {
		t.Fatalf("operator doc %s is missing %d item(s):\n  %s", operatorDocPath, len(missing), strings.Join(missing, "\n  "))
	}
}

func TestOperatorDoc_should_BePlainTextWithoutEmojiOrColorOnlyMeaning_When_Scanned(t *testing.T) {
	doc := readOperatorDoc(t)
	if strings.Contains(doc, "\x1b") {
		t.Error("doc contains an ANSI escape")
	}
	for i, line := range strings.Split(doc, "\n") {
		for _, r := range line {
			if r > 0x7e || (r < 0x20 && r != '\t') {
				t.Errorf("line %d has non-ASCII or control rune %U (emoji and symbol markers are not allowed)", i+1, r)
				break
			}
		}
	}
	// Every state shown in a table must be spelled in words.
	for _, state := range []string{"covered", "NOT covered", "unknown", "unreachable", "unauthorized"} {
		if !strings.Contains(doc, state) {
			t.Errorf("state %q is not spelled in words", state)
		}
	}
}
