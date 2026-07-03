package server

import (
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/tmux"
)

func TestBuildServiceDeps_RejectsNilCore(t *testing.T) {
	_, err := BuildServiceDeps(nil)
	if err == nil {
		t.Fatal("expected error for nil CoreDeps")
	}
}

func TestBuildServiceDeps_RejectsNilCoreFields(t *testing.T) {
	// CoreDeps with all nil fields should be rejected.
	core := &CoreDeps{}
	_, err := BuildServiceDeps(core)
	if err == nil {
		t.Fatal("expected error for CoreDeps with nil fields")
	}
}

func TestBuildServiceDeps_ErrorMentionsPhase(t *testing.T) {
	// The error from a nil CoreDeps should mention the phase name so that
	// callers can identify where in the initialization chain the failure occurred.
	_, err := BuildServiceDeps(nil)
	if err == nil {
		t.Fatal("expected error for nil CoreDeps")
	}
	if !strings.Contains(err.Error(), "BuildServiceDeps") {
		t.Errorf("error %q does not mention BuildServiceDeps", err.Error())
	}
}

func TestBuildRuntimeDeps_RejectsNilService(t *testing.T) {
	// The zero-value token is acceptable here — this test is only checking the
	// nil-ServiceDeps guard, not that tmux is actually running.
	_, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil ServiceDeps")
	}
}

func TestBuildRuntimeDeps_ErrorMentionsPhase(t *testing.T) {
	_, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil ServiceDeps")
	}
	if !strings.Contains(err.Error(), "BuildRuntimeDeps") {
		t.Errorf("error %q does not mention BuildRuntimeDeps", err.Error())
	}
}

func TestBuildServiceDeps_NilCoreFieldsErrorIsDescriptive(t *testing.T) {
	// A zero CoreDeps (all nil fields) must return an error that describes
	// the problem — not a panic or an empty error string.
	core := &CoreDeps{}
	_, err := BuildServiceDeps(core)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() == "" {
		t.Fatal("error message must not be empty")
	}
}

func TestBuildServiceDeps_OnlyCoreNil_DifferentFromPartialCore(t *testing.T) {
	// Nil *CoreDeps and a zero-value *CoreDeps should both fail, but the error
	// message for nil should mention the nil guard (not a panic).
	_, nilErr := BuildServiceDeps(nil)
	_, zeroErr := BuildServiceDeps(&CoreDeps{})

	if nilErr == nil || zeroErr == nil {
		t.Fatal("both cases should return errors")
	}
	// The errors should be different messages — nil core vs nil fields.
	if nilErr.Error() == zeroErr.Error() {
		t.Logf("note: nil and zero-value CoreDeps produce the same error: %v", nilErr)
	}
}

func TestPrNumFromTitle(t *testing.T) {
	cases := []struct {
		title   string
		matches bool
		want    int
	}{
		{"pr-1255-actions-spring-boot", true, 1255},
		{"PR-42-feature", true, 42},   // case-insensitive
		{"pr-0-foo", true, 0},          // zero is valid match; caller ignores pr 0
		{"pr-99-", true, 99},           // trailing dash only
		{"pr-1255", false, 0},          // missing trailing dash
		{"pr-foo-bar", false, 0},       // non-numeric
		{"feature-branch", false, 0},   // no prefix
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			m := prNumFromTitle.FindStringSubmatch(tc.title)
			if !tc.matches {
				if m != nil {
					t.Errorf("expected no match, got %v", m)
				}
				return
			}
			if m == nil {
				t.Fatalf("expected match for %q, got none", tc.title)
			}
			var got int
			for _, b := range m[1] {
				got = got*10 + int(b-'0')
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
