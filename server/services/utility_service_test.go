package services

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func setupUtilityService() *UtilityService {
	return NewUtilityService(NewApprovalStore(""))
}

func setupUtilityServiceWithPollerFixture() (*UtilityService, *session.ReviewQueuePoller) {
	svc := setupUtilityService()

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	svc.SetReviewQueuePoller(poller)

	return svc, poller
}

// strPtr returns a pointer to s; used to set optional proto string fields.
func strPtr(s string) *string { return &s }

// --------------------------------------------------------------------------
// GetLogs – nil poller (no session ID resolution available)
// --------------------------------------------------------------------------

// TestGetLogs_NoSessionID_NilPoller verifies that GetLogs with no session ID
// and no poller returns an empty log list rather than an error.
// (The global app log file may or may not exist; both outcomes are valid.)
func TestGetLogs_NoSessionID_NilPoller(t *testing.T) {
	t.Parallel()
	svc := setupUtilityService()

	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{}))

	// Either returns empty logs or errors if the log file is inaccessible.
	// The important invariant: no panic, and if it errors it's not CodeNotFound.
	if err != nil {
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		assert.NotEqual(t, connect.CodeNotFound, connectErr.Code(),
			"no-session-id call must not return CodeNotFound")
	} else {
		require.NotNil(t, resp)
	}
}

// --------------------------------------------------------------------------
// GetLogs – UUID resolution via ReviewQueuePoller
// --------------------------------------------------------------------------

// TestGetLogs_WithUUID_NilPoller verifies that GetLogs does not crash or
// return an unexpected error when the poller is nil and a UUID is passed.
// The log file for that UUID won't exist, so the response is empty.
func TestGetLogs_WithUUID_NilPoller(t *testing.T) {
	t.Parallel()
	svc := setupUtilityService()
	// No poller wired — UUID is used as-is to look up the log file.

	sid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{
		SessionId: &sid,
	}))

	// No log file exists for this UUID → empty response, not an error.
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Entries)
}

// TestGetLogs_WithUUID_MatchingInstance verifies that GetLogs exercises the
// UUID→Title resolution path when the poller has a matching instance.
// The session's log file does not exist on disk, so the response is empty —
// but the call must succeed (no crash, no error from the resolution logic).
func TestGetLogs_WithUUID_MatchingInstance(t *testing.T) {
	t.Parallel()
	svc, poller := setupUtilityServiceWithPollerFixture()

	const testUUID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	poller.SetInstances([]*session.Instance{
		{
			UUID:    testUUID,
			Title:   "my-resolved-session",
			Path:    "/tmp/test",
			Status:  session.Active,
			Program: "claude",
		},
	})

	// The log file for "my-resolved-session" does not exist → empty response.
	// The call must succeed: UUID is resolved to Title before path lookup.
	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{
		SessionId: strPtr(testUUID),
	}))

	require.NoError(t, err, "GetLogs should succeed when UUID resolves to a known instance")
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Entries)
}

// TestGetLogs_WithUUID_NoMatchingInstance verifies that when no instance in
// the poller matches the UUID, GetLogs falls back gracefully (uses the UUID
// as-is for the log file path, returning empty logs rather than an error).
func TestGetLogs_WithUUID_NoMatchingInstance(t *testing.T) {
	t.Parallel()
	svc, _ := setupUtilityServiceWithPollerFixture()
	// Poller is wired but has no instances.

	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{
		SessionId: strPtr("cccccccc-cccc-cccc-cccc-cccccccccccc"),
	}))

	require.NoError(t, err, "GetLogs should not error when UUID has no matching instance")
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Entries)
}

// TestGetLogs_WithTitle_FindsLogByTitle verifies the baseline: passing a Title
// (legacy behaviour) still works correctly after the UUID migration.
func TestGetLogs_WithTitle_FindsLogByTitle(t *testing.T) {
	t.Parallel()
	svc, poller := setupUtilityServiceWithPollerFixture()

	poller.SetInstances([]*session.Instance{
		{
			Title:   "title-session",
			Path:    "/tmp/test",
			Status:  session.Active,
			Program: "claude",
		},
	})

	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{
		SessionId: strPtr("title-session"),
	}))

	require.NoError(t, err, "GetLogs should accept a Title as session ID")
	require.NotNil(t, resp)
	assert.Empty(t, resp.Msg.Entries)
}

// --------------------------------------------------------------------------
// parseLogLine / parseLogs — JSON (slog) and legacy plain-text formats
// --------------------------------------------------------------------------

// TestParseLogLine_JSON_should_ExtractTimeLevelAndMessage_When_LineIsSlogJSON
// guards the actual bug this test file otherwise had zero coverage for:
// the live log file (log/log.go's JSON handler chain) writes lines like
// {"time":"2026-08-25T11:25:03.12598-07:00","level":"WARN","msg":"..."} —
// before this fix, logLineRegex (the plain-text format) never matched these,
// so parseLogs silently dropped nearly every real log line.
func TestParseLogLine_JSON_should_ExtractTimeLevelAndMessage_When_LineIsSlogJSON(t *testing.T) {
	t.Parallel()
	line := `{"time":"2026-08-25T11:25:03.12598-07:00","level":"WARN","msg":"worktree directory missing, marking as paused","path":"/tmp/wt"}`

	parsed, ok := parseLogLine(line)

	require.True(t, ok, "a well-formed slog JSON line must parse")
	assert.Equal(t, "WARN", parsed.Level)
	assert.Equal(t, 2026, parsed.Timestamp.Year())
	assert.Contains(t, parsed.Message, "worktree directory missing, marking as paused")
	// Extra JSON attributes beyond time/level/msg fold into Message as
	// sorted key=value pairs so they remain visible/searchable.
	assert.Contains(t, parsed.Message, "path=/tmp/wt")
}

// TestParseLogLine_JSON_should_SortExtraFieldsByKey_When_MultipleFieldsPresent
// pins the deterministic ordering of folded-in JSON attributes.
func TestParseLogLine_JSON_should_SortExtraFieldsByKey_When_MultipleFieldsPresent(t *testing.T) {
	t.Parallel()
	line := `{"time":"2026-08-25T11:25:03Z","level":"INFO","msg":"batch flushed","zebra":"1","alpha":"2"}`

	parsed, ok := parseLogLine(line)

	require.True(t, ok)
	alphaIdx := strings.Index(parsed.Message, "alpha=2")
	zebraIdx := strings.Index(parsed.Message, "zebra=1")
	require.NotEqual(t, -1, alphaIdx)
	require.NotEqual(t, -1, zebraIdx)
	assert.Less(t, alphaIdx, zebraIdx, "extra fields must be sorted by key (alpha before zebra)")
}

// TestParseLogLine_JSON_should_DefaultLevelToInfo_When_LevelFieldMissing
// covers a JSON line with no "level" key (defensive — slog always sets one
// in practice, but a hand-written or malformed line shouldn't be dropped).
func TestParseLogLine_JSON_should_DefaultLevelToInfo_When_LevelFieldMissing(t *testing.T) {
	t.Parallel()
	line := `{"time":"2026-08-25T11:25:03Z","msg":"no level field"}`

	parsed, ok := parseLogLine(line)

	require.True(t, ok)
	assert.Equal(t, "INFO", parsed.Level)
}

// TestParseLogLine_JSON_should_ExtractSource_When_SourceIsSlogAddSourceObject
// covers the day log/log.go's JSONHandler gets HandlerOptions.AddSource:
// true — slog's built-in shape is a nested object, not a plain string.
func TestParseLogLine_JSON_should_ExtractSource_When_SourceIsSlogAddSourceObject(t *testing.T) {
	t.Parallel()
	line := `{"time":"2026-08-25T11:25:03Z","level":"INFO","msg":"started","source":{"function":"main.run","file":"/app/main.go","line":42}}`

	parsed, ok := parseLogLine(line)

	require.True(t, ok)
	assert.Equal(t, "/app/main.go:42", parsed.Source)
	assert.NotContains(t, parsed.Message, "source=", "the structural AddSource object must not also be folded into Message as a generic attribute")
}

// TestParseLogLine_JSON_should_LeaveSourceEmpty_When_SourceIsAPlainStringAttribute
// guards against the wrong fix: several call sites (credentials.go,
// database_service.go) already pass a plain-string "source" attribute for
// unrelated domain data — treating that as a caller location would show a
// credential name or directory path as if it were "file.go:42".
func TestParseLogLine_JSON_should_LeaveSourceEmpty_When_SourceIsAPlainStringAttribute(t *testing.T) {
	t.Parallel()
	line := `{"time":"2026-08-25T11:25:03Z","level":"DEBUG","msg":"resolved credential","source":"keychain"}`

	parsed, ok := parseLogLine(line)

	require.True(t, ok)
	assert.Empty(t, parsed.Source, "a plain-string source attribute is domain data, not a caller location")
	assert.Contains(t, parsed.Message, "source=keychain", "the domain-specific value must still be visible via the generic attribute fold")
}

// TestParseLogLine_JSON_should_RedactSensitiveAttributeKeys_When_KnownSensitiveNamePresent
// guards the widened exposure surface this parser introduces: before this
// fix, JSON log lines (and any secret-shaped attribute on them) never
// reached GetLogs at all. Now that they do, a known-sensitive attribute
// name must be redacted rather than surfaced verbatim to anyone viewing
// the Logs/Patterns page.
func TestParseLogLine_JSON_should_RedactSensitiveAttributeKeys_When_KnownSensitiveNamePresent(t *testing.T) {
	t.Parallel()
	line := `{"time":"2026-08-25T11:25:03Z","level":"ERROR","msg":"auth failed","token":"sk-live-abc123","user":"alice"}`

	parsed, ok := parseLogLine(line)

	require.True(t, ok)
	assert.Contains(t, parsed.Message, "token=<redacted>")
	assert.NotContains(t, parsed.Message, "sk-live-abc123")
	assert.Contains(t, parsed.Message, "user=alice", "non-sensitive attributes are unaffected")
}

// TestFormatLogFieldValue_should_RenderWholeNumberFloatsAsPlainIntegers_When_ValueIsLarge
// guards against strconv.FormatFloat's 'g' format switching to scientific
// notation at 1e6 (e.g. FormatFloat(1_000_000, 'g', -1, 64) == "1e+06") —
// misleading for the common case of byte counts, durations, and PIDs
// logged as JSON numbers.
func TestFormatLogFieldValue_should_RenderWholeNumberFloatsAsPlainIntegers_When_ValueIsLarge(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "1000000", formatLogFieldValue(float64(1_000_000)))
	assert.Equal(t, "21474836480", formatLogFieldValue(float64(21_474_836_480)))
	assert.Equal(t, "1048576", formatLogFieldValue(float64(1_048_576)))
	assert.Equal(t, "100000", formatLogFieldValue(float64(100_000)))
}

// TestParseLogLine_Legacy_should_StillParse_When_LineIsOldPlainTextFormat
// guards backward compatibility: lines from call sites still on
// log.InfoLog().Printf (or older rotated log segments) must keep parsing.
func TestParseLogLine_Legacy_should_StillParse_When_LineIsOldPlainTextFormat(t *testing.T) {
	t.Parallel()
	line := `[main] INFO:2026/08/25 11:25:03 backlog_service.go:42: triage queued for item abc123`

	parsed, ok := parseLogLine(line)

	require.True(t, ok, "the legacy plain-text format must still parse")
	assert.Equal(t, "INFO", parsed.Level)
	assert.Equal(t, "triage queued for item abc123", parsed.Message)
	assert.Equal(t, "backlog_service.go:42", parsed.Source)
}

// TestParseLegacyLogLine_should_RejectSlogJSONLine_When_LineIsNotLegacyFormat
// pins the actual regression this PR fixes: before parseJSONLogLine existed,
// parseLogs's only parser was logLineRegex, and this exact production-shaped
// slog JSON line silently failed to match it — meaning nearly every real log
// line was dropped. Without this test, "parseJSONLogLine succeeds" alone
// doesn't prove the old code path actually had this defect.
func TestParseLegacyLogLine_should_RejectSlogJSONLine_When_LineIsNotLegacyFormat(t *testing.T) {
	t.Parallel()
	line := `{"time":"2026-08-25T11:25:03.12598-07:00","level":"WARN","msg":"worktree directory missing, marking as paused","path":"/tmp/wt"}`

	_, ok := parseLegacyLogLine(line)

	assert.False(t, ok, "the legacy regex must not match a slog JSON line — this is the exact defect parseJSONLogLine fixes")
}

// TestParseLogLine_should_RejectGarbage_When_LineMatchesNeitherFormat ensures
// a line that is neither valid JSON nor the legacy format is skipped, not
// mis-parsed into a zero-value entry.
func TestParseLogLine_should_RejectGarbage_When_LineMatchesNeitherFormat(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		"",
		"not a log line at all",
		`{"just": "an unrelated json object"}`,
		`{"time":"not-a-timestamp","msg":"x"}`,
	} {
		_, ok := parseLogLine(line)
		assert.False(t, ok, "line %q must not parse", line)
	}
}

// TestParseLogs_should_ParseMixedJSONAndLegacyLines_When_FileHasBothFormats
// exercises the full parseLogs pipeline (not just the line parser) against a
// file mixing both formats — the real shape of the on-disk log file today,
// since some call sites still use the legacy stdlib logger.
func TestParseLogs_should_ParseMixedJSONAndLegacyLines_When_FileHasBothFormats(t *testing.T) {
	t.Parallel()
	content := strings.Join([]string{
		`{"time":"2026-08-25T11:00:00Z","level":"INFO","msg":"json line one"}`,
		`[main] WARN:2026/08/25 11:01:00 foo.go:1: legacy line two`,
		`not a valid log line`,
		`{"time":"2026-08-25T11:02:00Z","level":"ERROR","msg":"json line three"}`,
	}, "\n")

	result, err := parseLogs(strings.NewReader(content), &sessionv1.GetLogsRequest{}, "")

	require.NoError(t, err)
	require.Len(t, result.Entries, 3, "the 3 valid lines must all parse; the garbage line is skipped")
	assert.Equal(t, 3, result.TotalCount)
	// parseLogs reverses to most-recent-first.
	assert.Equal(t, "json line three", result.Entries[0].Message)
	assert.Equal(t, "legacy line two", result.Entries[1].Message)
	assert.Equal(t, "json line one", result.Entries[2].Message)
}

// TestParseLogs_should_FilterByLevel_When_JSONLinesHaveDifferentLevels covers
// the level filter against JSON-sourced entries specifically (the previous
// test file only ever exercised this against an empty/nonexistent log file).
func TestParseLogs_should_FilterByLevel_When_JSONLinesHaveDifferentLevels(t *testing.T) {
	t.Parallel()
	content := strings.Join([]string{
		`{"time":"2026-08-25T11:00:00Z","level":"INFO","msg":"info line"}`,
		`{"time":"2026-08-25T11:01:00Z","level":"ERROR","msg":"error line"}`,
	}, "\n")

	result, err := parseLogs(strings.NewReader(content), &sessionv1.GetLogsRequest{
		Levels: []string{"ERROR"},
	}, "")

	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "error line", result.Entries[0].Message)
}

// TestParseJSONLogLine_should_ExtractSessionAttribute_When_LinePresent covers
// the "session" attribute log.ForSession tags every session-scoped entry
// with — it must be extracted into parsedLogLine.Session, not folded into
// Message like an arbitrary attribute (GetLogs' session filter reads this
// field, not the message text).
func TestParseJSONLogLine_should_ExtractSessionAttribute_When_LinePresent(t *testing.T) {
	t.Parallel()
	line := `{"time":"2026-08-25T11:00:00Z","level":"INFO","msg":"session started","session":"my-session"}`

	parsed, ok := parseJSONLogLine(line)

	require.True(t, ok)
	assert.Equal(t, "my-session", parsed.Session)
	assert.Equal(t, "session started", parsed.Message, "session must not also appear folded into Message")
}

// TestParseJSONLogLine_should_ExcludeSessionFromMessage_When_ValueIsEmptyOrNonString
// covers the edge case an all-or-nothing "consume iff non-empty" check would
// miss: an empty-string or non-string "session" attribute must still be
// treated as structural (excluded from the message fold), not leak through
// as a noisy "session=" suffix in Message.
func TestParseJSONLogLine_should_ExcludeSessionFromMessage_When_ValueIsEmptyOrNonString(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		`{"time":"2026-08-25T11:00:00Z","level":"INFO","msg":"m","session":""}`,
		`{"time":"2026-08-25T11:00:00Z","level":"INFO","msg":"m","session":123}`,
	} {
		parsed, ok := parseJSONLogLine(line)

		require.True(t, ok, "line %q must parse", line)
		assert.Empty(t, parsed.Session)
		assert.Equal(t, "m", parsed.Message, "line %q: session must not leak into Message", line)
	}
}

// TestParseLogs_should_FilterBySession_When_SessionIdSet is the regression
// test for the "Logs tab is always empty" bug: entries are written to the
// single global log file tagged with a "session" attribute (see
// log.ForSession), not to separate per-session log files, so GetLogs must
// filter the global file's entries by that attribute rather than trying to
// open a file that nothing ever writes.
func TestParseLogs_should_FilterBySession_When_SessionIdSet(t *testing.T) {
	t.Parallel()
	content := strings.Join([]string{
		`{"time":"2026-08-25T11:00:00Z","level":"INFO","msg":"line for session A","session":"session-a"}`,
		`{"time":"2026-08-25T11:01:00Z","level":"INFO","msg":"line for session B","session":"session-b"}`,
		`{"time":"2026-08-25T11:02:00Z","level":"INFO","msg":"line with no session tag"}`,
	}, "\n")

	result, err := parseLogs(strings.NewReader(content), &sessionv1.GetLogsRequest{}, "session-a")

	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "line for session A", result.Entries[0].Message)
}
