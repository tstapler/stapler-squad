package binaries

import (
	"regexp"
	"testing"
)

func mustMatch(t *testing.T, pattern, input string) {
	t.Helper()
	re := regexp.MustCompile(pattern)
	if !re.MatchString(input) {
		t.Errorf("pattern %q did not match %q", pattern, input)
	}
}

func mustNotMatch(t *testing.T, pattern, input string) {
	t.Helper()
	re := regexp.MustCompile(pattern)
	if re.MatchString(input) {
		t.Errorf("pattern %q unexpectedly matched %q", pattern, input)
	}
}

func TestAgyDetector_Name(t *testing.T) {
	t.Parallel()
	d := NewAgyDetector()
	if d.Name() != "agy" {
		t.Errorf("Name() = %q, want %q", d.Name(), "agy")
	}
}

func TestAgyDetector_Patterns_should_haveReadyPattern(t *testing.T) {
	t.Parallel()
	d := NewAgyDetector()
	p := d.Patterns()
	if len(p.Ready) == 0 {
		t.Fatal("Patterns().Ready is empty")
	}
	if p.Ready[0].Name != "agy_ready" {
		t.Errorf("Ready[0].Name = %q, want %q", p.Ready[0].Name, "agy_ready")
	}
}

func TestAgyDetector_Patterns_should_useAgyPrefixedNames(t *testing.T) {
	t.Parallel()
	d := NewAgyDetector()
	p := d.Patterns()
	for _, sp := range p.NeedsApproval {
		if len(sp.Name) < 4 || sp.Name[:4] != "agy_" {
			t.Errorf("NeedsApproval pattern Name %q does not have agy_ prefix", sp.Name)
		}
	}
}

func TestAgyDetector_FilterContent_should_returnUnchanged(t *testing.T) {
	t.Parallel()
	d := NewAgyDetector()
	input := "agy output"
	if got := d.FilterContent(input); got != input {
		t.Errorf("FilterContent(%q) = %q, want same", input, got)
	}
}

func TestAgyDetector_ready_pattern(t *testing.T) {
	t.Parallel()
	p := NewAgyDetector().Patterns().Ready[0].Pattern
	mustMatch(t, p, "◇ Ready")
	mustNotMatch(t, p, "Working...")
}

func TestAgyDetector_working_pattern(t *testing.T) {
	t.Parallel()
	p := NewAgyDetector().Patterns().Processing[0].Pattern
	mustMatch(t, p, "✦ Working")
	mustNotMatch(t, p, "◇ Ready")
}

func TestAgyDetector_permission_pattern(t *testing.T) {
	t.Parallel()
	p := NewAgyDetector().Patterns().NeedsApproval[0].Pattern
	mustMatch(t, p, "Yes, allow once")
	mustNotMatch(t, p, "yes deny")
}

func TestAgyDetector_allowExecution_pattern(t *testing.T) {
	t.Parallel()
	p := NewAgyDetector().Patterns().NeedsApproval[1].Pattern
	mustMatch(t, p, "Allow execution of:")
	mustNotMatch(t, p, "allow execution other")
}

func TestAgyDetector_idleReadline_pattern(t *testing.T) {
	t.Parallel()
	idle := NewAgyDetector().Patterns().Idle
	if len(idle) == 0 {
		t.Fatal("Idle patterns empty")
	}
	mustMatch(t, idle[0].Pattern, "> ▌")
	mustNotMatch(t, idle[0].Pattern, "> some text here")
}

func TestAgyDetector_idleInsert_pattern(t *testing.T) {
	t.Parallel()
	idle := NewAgyDetector().Patterns().Idle
	if len(idle) < 2 {
		t.Fatal("Idle patterns has fewer than 2 entries")
	}
	mustMatch(t, idle[1].Pattern, "[INSERT]")
	mustNotMatch(t, idle[1].Pattern, "[NORMAL]")
}

func TestAgyDetector_activeRunning_pattern(t *testing.T) {
	t.Parallel()
	active := NewAgyDetector().Patterns().Active
	if len(active) == 0 {
		t.Fatal("Active patterns empty")
	}
	mustMatch(t, active[0].Pattern, "= Running Agent...")
	mustNotMatch(t, active[0].Pattern, "Running")
}

func TestAgyDetector_activeThinking_pattern(t *testing.T) {
	t.Parallel()
	active := NewAgyDetector().Patterns().Active
	if len(active) < 2 {
		t.Fatal("Active patterns has fewer than 2 entries")
	}
	mustMatch(t, active[1].Pattern, "Thinking... (esc to cancel, 5s)")
	mustNotMatch(t, active[1].Pattern, "Thinking... (press enter)")
}

func TestAgyDetector_toolBullet_pattern(t *testing.T) {
	t.Parallel()
	var bullet string
	for _, sp := range NewAgyDetector().Patterns().Processing {
		if sp.Name == "agy_tool_bullet" {
			bullet = sp.Pattern
		}
	}
	if bullet == "" {
		t.Fatal("agy_tool_bullet pattern not found in Processing")
	}
	mustMatch(t, bullet, "• Bash(go test ./server/services 2>&1 | grep FAIL)")
	mustMatch(t, bullet, "• Read(~/.gemini/antigravity-cli/brain/uuid/system_generated/tasks/task-151.log)")
	mustMatch(t, bullet, "• ManageTask(list) (ctrl+o to expand)")
	mustMatch(t, bullet, "• Edit(~/Programming/stapler-squad/server/services/session_service.go)")
	mustNotMatch(t, bullet, "• Thought for 2s, 594 tokens")
	mustNotMatch(t, bullet, "Running go test ./server/services")
}

func TestAgyDetector_thought_pattern(t *testing.T) {
	t.Parallel()
	var thought string
	for _, sp := range NewAgyDetector().Patterns().Processing {
		if sp.Name == "agy_thought" {
			thought = sp.Pattern
		}
	}
	if thought == "" {
		t.Fatal("agy_thought pattern not found in Processing")
	}
	mustMatch(t, thought, "• Thought for 2s, 594 tokens")
	mustMatch(t, thought, "Thought for 10s, 1200 tokens")
	mustNotMatch(t, thought, "I thought about it yesterday")
}

func TestAgyDetector_acceptFileEdit_pattern(t *testing.T) {
	t.Parallel()
	var accept string
	for _, sp := range NewAgyDetector().Patterns().NeedsApproval {
		if sp.Name == "agy_accept_file_edit" {
			accept = sp.Pattern
		}
	}
	if accept == "" {
		t.Fatal("agy_accept_file_edit pattern not found in NeedsApproval")
	}
	mustMatch(t, accept, "Accept this file edit?")
	mustNotMatch(t, accept, "Accept this file edit") // question mark required
	mustNotMatch(t, accept, "file edited successfully")
}

func TestAgyDetector_pendingEdit_pattern(t *testing.T) {
	t.Parallel()
	var pending string
	for _, sp := range NewAgyDetector().Patterns().NeedsApproval {
		if sp.Name == "agy_pending_edit" {
			pending = sp.Pattern
		}
	}
	if pending == "" {
		t.Fatal("agy_pending_edit pattern not found in NeedsApproval")
	}
	mustMatch(t, pending, "Pending edit")
	mustNotMatch(t, pending, "No pending edits")
}

func TestAgyDetector_numberedOption_pattern(t *testing.T) {
	t.Parallel()
	input := NewAgyDetector().Patterns().InputRequired
	if len(input) == 0 {
		t.Fatal("InputRequired patterns empty — agy approval dialog options would not parse")
	}
	var numbered string
	for _, sp := range input {
		if sp.Name == "agy_numbered_option" {
			numbered = sp.Pattern
		}
	}
	if numbered == "" {
		t.Fatal("agy_numbered_option pattern not found in InputRequired")
	}
	mustMatch(t, numbered, "> 1. Yes, accept this change")
	mustMatch(t, numbered, "> 2. No, reject this change")
	mustNotMatch(t, numbered, "2. No, reject this change") // unselected option has no > cursor
	mustNotMatch(t, numbered, "> some text here")
}

func TestAgyDetector_escToCancel_pattern(t *testing.T) {
	t.Parallel()
	var esc string
	for _, sp := range NewAgyDetector().Patterns().Active {
		if sp.Name == "agy_esc_to_cancel" {
			esc = sp.Pattern
		}
	}
	if esc == "" {
		t.Fatal("agy_esc_to_cancel pattern not found in Active")
	}
	mustMatch(t, esc, "esc to cancel")
	mustMatch(t, esc, "esc to interrupt")
	mustNotMatch(t, esc, "press enter to continue")
}
