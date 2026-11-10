package session

import (
	"strings"
	"testing"
)

func allPromptSections() map[string]string {
	return map[string]string{
		"SecWorkDiscipline": SecWorkDiscipline, "SecBlockerRouting": SecBlockerRouting,
		"SecContextLost": SecContextLost, "SecCommandsFail": SecCommandsFail, "SecNeverEnd": SecNeverEnd,
		"SecAwaitVerdict": SecAwaitVerdict, "SecOnPass": SecOnPass, "SecOnFail": OnFailSection(),
		"SecDoneNote": SecDoneNote, "SecFailNote": SecFailNote, "SecShipIntro": SecShipIntro,
		"SecShipSteps": SecShipSteps, "SecTriageCriteriaCheck": SecTriageCriteriaCheck, "SecReviewFailureClass": SecReviewFailureClass, "SecReviewScope": SecReviewScope,
		"TriageOutputContract": TriageOutputContract(), "RetriageOutputContract": RetriageOutputContract(), "HelpOwnerActionLine": HelpOwnerActionLine,
	}
}

// Seeded templates are validated on write, so a section that fails here would
// make the sdd seed (or an operator edit built from it) unsavable.
func TestPromptSections_should_PassSeedValidation(t *testing.T) {
	t.Parallel()
	for name, section := range allPromptSections() {
		if err := validateNoShellMetacharacters(name, ForSeed(section)); err != nil {
			t.Errorf("%v", err)
		}
		if err := validateRecognizedPlaceholders(name, ForSeed(section)); err != nil {
			t.Errorf("%v", err)
		}
		if strings.Contains(ForSeed(section), CodeMark) {
			t.Errorf("%s: CodeMark left in seed text", name)
		}
	}
}

// The sdd ship template once drifted from the default and lost the
// report_pr_created step; both now render the same sections.
func TestShipTemplates_should_BothRequireReportPRCreated(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{"sdd": sddShipCommandTemplate, "default": ForDefault(SecShipSteps)} {
		if !strings.Contains(text, "report_pr_created") {
			t.Errorf("%s ship text must require report_pr_created", name)
		}
	}
}

func TestWorkDiscipline_should_TellWorkerToSyncMain(t *testing.T) {
	t.Parallel()
	seed := ForSeed(SecWorkDiscipline)
	if !strings.Contains(seed, "git merge main") || strings.Contains(seed, CodeMark) {
		t.Errorf("work discipline should name git merge main with the code mark stripped: %q", seed)
	}
}
