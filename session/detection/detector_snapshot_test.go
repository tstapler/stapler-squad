package detection

import (
	"context"
	"fmt"
	"maps"
	"os"
	"testing"
)

// resetSnapshotAfterTest restores the built-ins-only snapshot once t
// completes. activeSnapshot is package-global; every test that calls
// rebuildSnapshot must call this so it doesn't leak state into other tests
// in this package (including ones in other files, e.g. bug_regression_test.go).
func resetSnapshotAfterTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		activeSnapshot.Store(buildSnapshot(DefaultRegistry(), nil))
	})
}

// TestRebuildSnapshot_should_makePluginDetectable_When_directoryHasValidPlugin
// covers AC1: a rebuild picking up a valid plugin file makes its binary name
// detectable via DetectForProgram and reports the file's path as provenance.
func TestRebuildSnapshot_should_makePluginDetectable_When_directoryHasValidPlugin(t *testing.T) {
	resetSnapshotAfterTest(t)

	dir := t.TempDir()
	path := writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))

	if err := rebuildSnapshot(context.Background(), dir); err != nil {
		t.Fatalf("rebuildSnapshot() = %v, want nil", err)
	}

	sd := NewStatusDetector()
	status := sd.DetectForProgram([]byte("Thinking..."), "my-agent")
	if status != StatusProcessing {
		t.Errorf("DetectForProgram(%q, %q) = %v, want %v", "Thinking...", "my-agent", status, StatusProcessing)
	}

	prov := DetectorProvenance()
	if got := prov["my-agent"]; got != path {
		t.Errorf("DetectorProvenance()[%q] = %q, want %q", "my-agent", got, path)
	}
}

// TestRebuildSnapshot_should_keepPreviousSnapshot_When_directoryScanFails
// covers AC2: once a directory has been replaced by a regular file (making
// os.ReadDir fail with ENOTDIR, the LoadPluginDir "directory"-field fatal
// case), a subsequent rebuild must return an error and must NOT clobber the
// previously published snapshot.
func TestRebuildSnapshot_should_keepPreviousSnapshot_When_directoryScanFails(t *testing.T) {
	resetSnapshotAfterTest(t)

	dir := t.TempDir()
	path := writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))

	if err := rebuildSnapshot(context.Background(), dir); err != nil {
		t.Fatalf("initial rebuildSnapshot() = %v, want nil", err)
	}
	if got := DetectorProvenance()["my-agent"]; got != path {
		t.Fatalf("precondition failed: DetectorProvenance()[%q] = %q, want %q", "my-agent", got, path)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("failed to remove dir %s: %v", dir, err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("failed to replace dir with regular file %s: %v", dir, err)
	}

	if err := rebuildSnapshot(context.Background(), dir); err == nil {
		t.Fatal("rebuildSnapshot() = nil, want a directory-scan error")
	}

	prov := DetectorProvenance()
	if got := prov["my-agent"]; got != path {
		t.Errorf("DetectorProvenance()[%q] = %q after failed rebuild, want unchanged %q (previous snapshot must stay live)", "my-agent", got, path)
	}
}

// TestRebuildSnapshot_should_loadValidFileAndKeepBuiltins_When_directoryHasInvalidFileToo
// covers AC3: a per-file rejection (bad regex) does not block the rest of
// the rebuild — the valid plugin still loads, and built-ins are unaffected.
func TestRebuildSnapshot_should_loadValidFileAndKeepBuiltins_When_directoryHasInvalidFileToo(t *testing.T) {
	resetSnapshotAfterTest(t)

	dir := t.TempDir()
	writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))
	writePluginFile(t, dir, "broken.toml", `id = "broken"
binary_names = ["broken"]

[[patterns]]
name = "thinking"
regex = "Thinking(\\.\\.\\."
status = "processing"
`)

	if err := rebuildSnapshot(context.Background(), dir); err != nil {
		t.Fatalf("rebuildSnapshot() = %v, want nil", err)
	}

	sd := NewStatusDetector()

	status := sd.DetectForProgram([]byte("Thinking..."), "my-agent")
	if status != StatusProcessing {
		t.Errorf("DetectForProgram(%q, %q) = %v, want %v", "Thinking...", "my-agent", status, StatusProcessing)
	}

	claudeStatus := sd.DetectForProgram([]byte("esc to interrupt"), "claude")
	if claudeStatus != StatusExecuting {
		t.Errorf("DetectForProgram(%q, %q) = %v, want %v (built-in claude detector must still resolve)", "esc to interrupt", "claude", claudeStatus, StatusExecuting)
	}

	prov := DetectorProvenance()
	if _, ok := prov["my-agent"]; !ok {
		t.Errorf("DetectorProvenance() = %+v, want an entry for %q", prov, "my-agent")
	}
	if _, ok := prov["broken"]; ok {
		t.Errorf("DetectorProvenance() = %+v, want no entry for rejected plugin %q", prov, "broken")
	}
}

// TestRebuildSnapshot_should_returnCtxErrAndSkipStore_When_contextAlreadyCancelled
// covers AC4: an already-cancelled context short-circuits before any work,
// including before the snapshot store.
func TestRebuildSnapshot_should_returnCtxErrAndSkipStore_When_contextAlreadyCancelled(t *testing.T) {
	resetSnapshotAfterTest(t)

	dir := t.TempDir()
	writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))

	before := DetectorProvenance()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rebuildSnapshot(ctx, dir)
	if err != ctx.Err() {
		t.Errorf("rebuildSnapshot(cancelled ctx, dir) = %v, want %v", err, ctx.Err())
	}

	after := DetectorProvenance()
	if !maps.Equal(before, after) {
		t.Errorf("DetectorProvenance() changed after cancelled-context rebuildSnapshot(): before=%v after=%v (no Store should have occurred)", before, after)
	}
}

// TestRebuildSnapshot_should_publishPartialSet_When_fileCountCapExceeded
// covers AC5: the non-fatal "file_count" cap error must not trip the
// directory-level fatal path — the rebuild still succeeds and publishes the
// 200 detectors that did load.
func TestRebuildSnapshot_should_publishPartialSet_When_fileCountCapExceeded(t *testing.T) {
	resetSnapshotAfterTest(t)

	dir := t.TempDir()
	const total = maxPluginFiles + 1
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("agent-%03d", i)
		writePluginFile(t, dir, fmt.Sprintf("p%03d.toml", i), validPluginTOML(id, []string{id}))
	}

	if err := rebuildSnapshot(context.Background(), dir); err != nil {
		t.Fatalf("rebuildSnapshot() = %v, want nil", err)
	}

	prov := DetectorProvenance()
	builtinCount := DefaultRegistry().Len()
	want := builtinCount + maxPluginFiles
	if len(prov) != want {
		t.Errorf("len(DetectorProvenance()) = %d, want %d (%d built-ins + %d capped plugins)", len(prov), want, builtinCount, maxPluginFiles)
	}
}

// DetectForProgram_should_matchBuiltinPattern_When_noPluginsLoaded verifies
// that with no plugins loaded (package init's built-ins-only snapshot is
// still the active one), DetectForProgram behaves exactly as it did before
// the cutover to the atomic detectorSnapshot: the built-in claude
// "esc to interrupt" pattern still resolves to StatusExecuting.
func TestDetectForProgram_should_matchBuiltinPattern_When_noPluginsLoaded(t *testing.T) {
	sd := NewStatusDetector()
	status := sd.DetectForProgram([]byte("esc to interrupt"), "claude")
	if status != StatusExecuting {
		t.Errorf("DetectForProgram(%q, %q) = %v, want %v", "esc to interrupt", "claude", status, StatusExecuting)
	}
}

// DetectorProvenance_should_returnBuiltinsOnlyMap_When_noPluginsLoaded verifies
// the built-ins-only snapshot reports all 5 built-in binaries with an empty
// provenance value, and that mutating the returned map cannot affect the
// live snapshot (defensive copy).
func TestDetectorProvenance_should_returnBuiltinsOnlyMap_When_noPluginsLoaded(t *testing.T) {
	prov := DetectorProvenance()

	wantNames := []string{"claude", "gemini", "aider", "opencode", "agy"}
	if len(prov) != len(wantNames) {
		t.Fatalf("DetectorProvenance() returned %d entries, want %d: %v", len(prov), len(wantNames), prov)
	}
	for _, name := range wantNames {
		v, ok := prov[name]
		if !ok {
			t.Errorf("DetectorProvenance() missing entry for %q", name)
			continue
		}
		if v != "" {
			t.Errorf("DetectorProvenance()[%q] = %q, want empty string (built-in)", name, v)
		}
	}

	// Mutate the returned map and confirm a fresh call is unaffected.
	prov["claude"] = "mutated"
	prov["injected"] = "should-not-persist"

	prov2 := DetectorProvenance()
	if prov2["claude"] != "" {
		t.Errorf("DetectorProvenance() after mutating a prior returned map: [\"claude\"] = %q, want empty string — snapshot provenance map was not defensively copied", prov2["claude"])
	}
	if _, ok := prov2["injected"]; ok {
		t.Errorf("DetectorProvenance() after mutating a prior returned map: unexpected \"injected\" key leaked into snapshot")
	}
}

// TestLookupBinaryDetector_should_findBuiltins_When_noPluginsLoaded is a more
// direct unit test of the accessor DetectForProgram now uses in place of the
// old package-level built-in-detectors map index.
func TestLookupBinaryDetector_should_findBuiltins_When_noPluginsLoaded(t *testing.T) {
	for _, name := range []string{"claude", "gemini", "aider", "opencode", "agy"} {
		if _, ok := lookupBinaryDetector(name); !ok {
			t.Errorf("lookupBinaryDetector(%q) = _, false; want true", name)
		}
	}
	if _, ok := lookupBinaryDetector("not-a-real-binary"); ok {
		t.Errorf("lookupBinaryDetector(%q) = _, true; want false", "not-a-real-binary")
	}
}

// TestDetectorForProgram_should_returnDetector_When_programRegistered is a
// direct unit test of the exported wrapper session.ClaudeController.Start
// uses in place of the old unconditional NewStatusDetector() call.
func TestDetectorForProgram_should_returnDetector_When_programRegistered(t *testing.T) {
	sd, ok := ResolveDetectorForProgram("claude")
	if !ok {
		t.Fatal(`ResolveDetectorForProgram("claude") = _, false; want true`)
	}
	if sd == nil {
		t.Fatal(`ResolveDetectorForProgram("claude") returned ok=true but a nil detector`)
	}
	status := sd.Detect([]byte("esc to interrupt"))
	if status != StatusExecuting {
		t.Errorf(`Detect("esc to interrupt") on the returned detector = %v, want %v`, status, StatusExecuting)
	}
}

// TestDetectorForProgram_should_returnMiss_When_programUnregistered covers
// the fallback signal callers (ClaudeController.Start) depend on to know
// when to construct NewStatusDetector() instead.
func TestDetectorForProgram_should_returnMiss_When_programUnregistered(t *testing.T) {
	if sd, ok := ResolveDetectorForProgram("not-a-real-binary"); ok || sd != nil {
		t.Errorf(`ResolveDetectorForProgram("not-a-real-binary") = %v, %v; want nil, false`, sd, ok)
	}
}

// TestDetectorForProgram_should_returnIndependentDetector_When_calledTwice is
// a regression guard for the concurrency hazard ResolveDetectorForProgram exists to
// avoid: returning the snapshot's own *StatusDetector pointer directly would
// let two callers' SetSessionID calls (one per concurrent session running
// the same program) stomp each other's DetectionEventSink.sessionID, since
// they'd be mutating the exact same shared object. Each call must return an
// independent detector instance.
func TestDetectorForProgram_should_returnIndependentDetector_When_calledTwice(t *testing.T) {
	sd1, ok := ResolveDetectorForProgram("claude")
	if !ok {
		t.Fatal(`ResolveDetectorForProgram("claude") = _, false; want true`)
	}
	sd2, ok := ResolveDetectorForProgram("claude")
	if !ok {
		t.Fatal(`ResolveDetectorForProgram("claude") = _, false; want true`)
	}

	sd1.SetSessionID("session-a")
	sd2.SetSessionID("session-b")
	sd1.Detect([]byte("esc to interrupt"))
	sd2.Detect([]byte("esc to interrupt"))

	events1 := sd1.RecentEvents(1)
	events2 := sd2.RecentEvents(1)
	if len(events1) != 1 || len(events2) != 1 {
		t.Fatalf("expected 1 recorded event on each detector, got %d and %d", len(events1), len(events2))
	}
	if events1[0].SessionID != "session-a" {
		t.Errorf("sd1's recorded event SessionID = %q, want %q (sd2.SetSessionID must not affect sd1)", events1[0].SessionID, "session-a")
	}
	if events2[0].SessionID != "session-b" {
		t.Errorf("sd2's recorded event SessionID = %q, want %q (sd1.SetSessionID must not affect sd2)", events2[0].SessionID, "session-b")
	}
}

// TestResolveDetectorForProgram_should_normalizeToBinaryName_When_programIsFullCommandOrPath
// is the regression guard for the BLOCKER found in pre-ship review: Instance.Program
// is frequently not a bare binary name — it can be a resolved absolute path
// (config.GetClaudeCommand()) or a full command string with arguments — and an
// unnormalized exact-string map lookup silently missed the registered "claude"
// entry for both shapes, falling back to the (formerly-identical, now-guaranteed
// worse if this regresses) generic detector.
func TestResolveDetectorForProgram_should_normalizeToBinaryName_When_programIsFullCommandOrPath(t *testing.T) {
	cases := []string{
		"claude",
		"/usr/local/bin/claude",
		"claude --dangerously-skip-permissions",
		"  claude --foo bar  ",
	}
	for _, program := range cases {
		sd, ok := ResolveDetectorForProgram(program)
		if !ok {
			t.Errorf("ResolveDetectorForProgram(%q) = _, false; want true", program)
			continue
		}
		status := sd.Detect([]byte("esc to interrupt"))
		if status != StatusExecuting {
			t.Errorf("ResolveDetectorForProgram(%q): Detect(\"esc to interrupt\") = %v, want %v", program, status, StatusExecuting)
		}
	}
}

// TestProgramBinaryName_should_extractBareName_When_programHasArgsOrIsAbsolutePath
// is a direct unit test of the normalization helper.
func TestProgramBinaryName_should_extractBareName_When_programHasArgsOrIsAbsolutePath(t *testing.T) {
	cases := []struct {
		program string
		want    string
	}{
		{"claude", "claude"},
		{"/usr/local/bin/claude", "claude"},
		{"claude --dangerously-skip-permissions", "claude"},
		{"aider --model ollama_chat/gemma3:1b", "aider"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := programBinaryName(c.program); got != c.want {
			t.Errorf("programBinaryName(%q) = %q, want %q", c.program, got, c.want)
		}
	}
}

// TestClaudeBuiltinDetector_should_matchGetDefaultPatterns_When_resolvedFromSnapshot
// is the regression guard for the second half of the BLOCKER: the built-in
// "claude" registry entry (binaries.ClaudeDetector) must stay at parity with
// getDefaultPatterns(), the generic fallback — historically it was a stale,
// hand-copied, materially thinner subset (missing the entire WaitingForAgent
// category, among others). Spot-checks the categories/patterns that were
// previously missing, plus an aggregate pattern count so any future drift
// (in either direction) fails loudly instead of silently.
func TestClaudeBuiltinDetector_should_matchGetDefaultPatterns_When_resolvedFromSnapshot(t *testing.T) {
	sd, ok := ResolveDetectorForProgram("claude")
	if !ok {
		t.Fatal(`ResolveDetectorForProgram("claude") = _, false; want true`)
	}

	countPatterns := func(p StatusPatterns) int {
		return len(p.Ready) + len(p.Processing) + len(p.NeedsApproval) + len(p.InputRequired) +
			len(p.Error) + len(p.TestsFailing) + len(p.Idle) + len(p.Active) + len(p.Success) + len(p.WaitingForAgent)
	}

	builtin := sd.patternSet.Load().Patterns()
	def := getDefaultPatterns()

	if got, want := countPatterns(builtin), countPatterns(def); got != want {
		t.Errorf("built-in claude detector has %d total patterns, want %d (parity with getDefaultPatterns())", got, want)
	}

	if len(builtin.WaitingForAgent) == 0 {
		t.Error("built-in claude detector has no WaitingForAgent patterns — must have parity with getDefaultPatterns()")
	}

	hasName := func(patterns []StatusPattern, name string) bool {
		for _, p := range patterns {
			if p.Name == name {
				return true
			}
		}
		return false
	}
	if !hasName(builtin.Idle, "claude_accept_edits") {
		t.Error("built-in claude detector missing Idle pattern \"claude_accept_edits\"")
	}
	if !hasName(builtin.Ready, "gemini_ready") {
		t.Error("built-in claude detector missing cross-tool Ready pattern \"gemini_ready\"")
	}
}

// TestBuildSnapshot_should_skipUnresolvableEntry_When_registryLookupMiss is a
// defensive-path regression guard: buildSnapshot must not panic or drop the
// whole snapshot if a name in reg.Names() somehow fails reg.Lookup (can't
// happen via the public DetectorRegistry API today, but the nil-guard shape
// in buildSnapshot/lookupBinaryDetector is exactly what NilAway flags as
// required — see .claude/rules/interface-pollution-checklist.md).
func TestBuildSnapshot_should_produceUsableSnapshot_When_givenDefaultRegistry(t *testing.T) {
	snap := buildSnapshot(DefaultRegistry(), nil)
	if len(snap.byBinary) != 5 {
		t.Errorf("buildSnapshot(DefaultRegistry(), nil) produced %d detectors, want 5", len(snap.byBinary))
	}
	if len(snap.provenance) != 5 {
		t.Errorf("buildSnapshot(DefaultRegistry(), nil) produced %d provenance entries, want 5", len(snap.provenance))
	}
	for name, src := range snap.provenance {
		if src != "" {
			t.Errorf("buildSnapshot(DefaultRegistry(), nil) provenance[%q] = %q, want empty string", name, src)
		}
	}
}
