package backlog

import (
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func TestSDDTemplates_should_ShareTheDefaultProtocolRules(t *testing.T) {
	t.Parallel()
	shared := map[string]string{
		"work discipline": session.ForSeed(session.SecWorkDiscipline), "blocker routing": session.ForSeed(session.SecBlockerRouting),
		"never end": session.ForSeed(session.SecNeverEnd), "on pass": session.ForSeed(session.SecOnPass), "on fail": session.ForSeed(session.OnFailSection()),
	}
	for name, want := range shared {
		if !strings.Contains(session.SddInitialPromptTemplate, want) {
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
