package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/ent"
)

// pinScheduleWake sets the flag state for one test. Tests using it must not be
// parallel: the seam is a package variable.
func pinScheduleWake(t *testing.T, on bool) {
	t.Helper()
	prev := scheduleWakeAdvertised
	scheduleWakeAdvertised = func() bool { return on }
	t.Cleanup(func() { scheduleWakeAdvertised = prev })
}

func wakePromptItem() *BacklogItemData {
	return &BacklogItemData{
		ID: "11111111-1111-1111-1111-111111111111", Title: "Add retry", RepoPath: "/repo/path",
		AcceptanceCriteria: `[{"index":0,"text":"retries three times"}]`,
	}
}

func goldenText(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "wake_prompt", name+".golden"))
	require.NoError(t, err, "run with UPDATE_PIPELINE_GOLDEN=1 to create it")
	return string(b)
}

func assertWakeGolden(t *testing.T, name, got string) {
	t.Helper()
	if os.Getenv("UPDATE_PIPELINE_GOLDEN") == "1" {
		path := filepath.Join("testdata", "wake_prompt", name+".golden")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	assert.Equal(t, goldenText(t, name), got, name)
}

func requireForbidsPollingButNamesTheTool(t *testing.T, text, name string) {
	t.Helper()
	assert.Contains(t, text, "schedule_wake", name)
	assert.Contains(t, text, "do NOT use "+scheduleWakeupToken, name, "the polling ban stays")
	assert.Contains(t, text, "/loop", name)
}

func TestInitialPrompt_should_MatchGolden_When_ScheduleWakeIsOff(t *testing.T) {
	pinScheduleWake(t, false)
	assertWakeGolden(t, "initial_prompt_off", BuildSessionInitialPrompt(wakePromptItem(), nil))
	files, err := buildDefaultSlashCommandSet(wakePromptItem())
	require.NoError(t, err)
	assertWakeGolden(t, "review_command_off", files["review.md"])
}

func TestInitialPrompt_should_NameScheduleWakeAndStillForbidPolling_When_ScheduleWakeIsOn(t *testing.T) {
	pinScheduleWake(t, false)
	off := BuildSessionInitialPrompt(wakePromptItem(), nil)
	pinScheduleWake(t, true)

	on := BuildSessionInitialPrompt(wakePromptItem(), nil)

	requireForbidsPollingButNamesTheTool(t, on, "initial prompt")
	assert.Equal(t, strings.TrimSuffix(off, "\n")+"\n10. "+scheduleWakeGuidance+"\n", on, "only the new step is added")
}

func TestReviewCommand_should_NameScheduleWakeAndStillForbidPolling_When_ScheduleWakeIsOn(t *testing.T) {
	pinScheduleWake(t, false)
	offFiles, err := buildDefaultSlashCommandSet(wakePromptItem())
	require.NoError(t, err)
	pinScheduleWake(t, true)

	onFiles, err := buildDefaultSlashCommandSet(wakePromptItem())

	require.NoError(t, err)
	requireForbidsPollingButNamesTheTool(t, onFiles["review.md"], "review.md")
	assert.Equal(t, offFiles["review.md"]+"\n"+scheduleWakeGuidance+"\n", onFiles["review.md"])
	for name, text := range offFiles {
		if name != "review.md" {
			assert.Equal(t, text, onFiles[name], name+" is untouched")
		}
	}
}

func TestPipelineMode_should_AppendScheduleWakeToReviewAndInitialPrompts_OnlyWhenOn(t *testing.T) {
	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return []*ent.PipelineMode{{
				Slug: "sdd", Name: "SDD", ReviewCommandTemplate: "Call request_review for {{item_id}}. Do NOT use " + scheduleWakeupToken + " or /loop to wait.",
				InitialPromptTemplate: "Work on {{item_title}}. Do NOT use " + scheduleWakeupToken + " or /loop.",
				StatusCommandTemplate: "s", DoneCommandTemplate: "d", FailCommandTemplate: "f", ShipCommandTemplate: "sh", HelpCommandTemplate: "h",
			}}, nil
		},
	}
	cache := &pipelineModeCache{}
	require.NoError(t, cache.Load(context.Background(), repo))
	engine := &CachingPipelineEngine{repo: repo, cache: cache}
	item := wakePromptItem()
	item.PipelineMode = "sdd"

	pinScheduleWake(t, false)
	offFiles, err := engine.SlashCommandSet(item)
	require.NoError(t, err)
	offInitial := engine.InitialPromptFor(item, nil)
	assert.NotContains(t, offFiles["review.md"], "schedule_wake")
	assert.NotContains(t, offInitial, "schedule_wake")

	pinScheduleWake(t, true)
	onFiles, err := engine.SlashCommandSet(item)
	require.NoError(t, err)
	onInitial := engine.InitialPromptFor(item, nil)

	assert.Contains(t, onFiles["review.md"], "schedule_wake")
	assert.Contains(t, onInitial, "schedule_wake")
	assert.True(t, strings.HasPrefix(onFiles["review.md"], offFiles["review.md"]), "the stored template text is untouched")
	assert.True(t, strings.HasPrefix(onInitial, offInitial))
}
