package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/pipelinemode"
)

func TestSDDTemplates_should_ShareTheDefaultProtocolRules(t *testing.T) {
	t.Parallel()
	shared := map[string]string{
		"work discipline": pipelinemode.ForSeed(pipelinemode.SecWorkDiscipline), "blocker routing": pipelinemode.ForSeed(pipelinemode.SecBlockerRouting),
		"never end": pipelinemode.ForSeed(pipelinemode.SecNeverEnd), "on pass": pipelinemode.ForSeed(pipelinemode.SecOnPass), "on fail": pipelinemode.ForSeed(pipelinemode.OnFailSection()),
	}
	for name, want := range shared {
		if !strings.Contains(pipelinemode.SddInitialPromptTemplate, want) {
			t.Errorf("sdd initial prompt is missing the shared %s section", name)
		}
		if !strings.Contains(strings.ReplaceAll(taskProtocolBlock, "`", ""), want) {
			t.Errorf("default protocol is missing the shared %s section", name)
		}
	}
}

func TestReworkContextFor_should_CarryPriorAttemptsAndFixesLine(t *testing.T) {
	t.Parallel()
	if got := reworkContextFor(&backlogdto.BacklogItemData{ID: "i"}, nil); got != "" {
		t.Errorf("first attempt without an issue should add nothing, got %q", got)
	}
	item := &backlogdto.BacklogItemData{ID: "i", ExternalURL: "https://github.com/o/r/issues/7"}
	ended := time.Now()
	got := reworkContextFor(item, []backlogdto.ItemSessionSummary{{Role: "work", EndedAt: &ended, CommitCountSinceSpawn: 2}})
	for _, want := range []string{"Prior Attempts", "Fixes https://github.com/o/r/issues/7"} {
		if !strings.Contains(got, want) {
			t.Errorf("rework context missing %q in %q", want, got)
		}
	}
}
