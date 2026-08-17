package sqlitedsn

import (
	"testing"
	"time"
)

func TestBuild_should_ReturnBarePath_When_NoParamsAdded(t *testing.T) {
	got := New("/tmp/test.db").Build()
	want := "/tmp/test.db"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuild_should_JoinParamsWithQuestionMark_When_PathHasNoExistingQuery(t *testing.T) {
	got := New("/tmp/test.db").WithWAL().WithBusyTimeout(5000 * time.Millisecond).Build()
	want := "/tmp/test.db?_journal_mode=WAL&_timeout=5000"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuild_should_JoinParamsWithAmpersand_When_PathAlreadyHasQueryString(t *testing.T) {
	got := New("file:test?cache=shared").WithBusyTimeout(5000 * time.Millisecond).Build()
	want := "file:test?cache=shared&_timeout=5000"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWithWAL_should_BeNoOp_When_PathAlreadyHasQueryString(t *testing.T) {
	got := New("file:test?cache=shared").WithWAL().Build()
	want := "file:test?cache=shared"
	if got != want {
		t.Errorf("got %q, want %q; WithWAL should be a no-op on shared-cache DSNs", got, want)
	}
}

func TestWithForeignKeys_should_SetLongForm(t *testing.T) {
	got := New("/tmp/test.db").WithForeignKeys().Build()
	want := "/tmp/test.db?_foreign_keys=on"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWithForeignKeysShort_should_SetShortForm(t *testing.T) {
	got := New("/tmp/test.db").WithForeignKeysShort().Build()
	want := "/tmp/test.db?_fk=1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWithEntTimeCompat_should_SetAllThreeParams(t *testing.T) {
	got := New("/tmp/test.db").WithEntTimeCompat().Build()
	want := "/tmp/test.db?_texttotime=1&_time_format=sqlite&_timezone=UTC"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWithPragma_should_UseGenericPragmaSyntax(t *testing.T) {
	got := New("/tmp/test.db").WithPragma("wal_autocheckpoint", "1000").Build()
	want := "/tmp/test.db?_pragma=wal_autocheckpoint(1000)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuild_should_ChainMultipleParamsInCallOrder(t *testing.T) {
	got := New("/tmp/test.db").
		WithQueryOnly().
		WithWAL().
		WithBusyTimeout(5000 * time.Millisecond).
		Build()
	want := "/tmp/test.db?_query_only=1&_journal_mode=WAL&_timeout=5000"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
