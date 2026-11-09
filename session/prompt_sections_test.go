package session

import (
	"strings"
	"testing"
)

func allPromptSections() map[string]string {
	return map[string]string{
		"secWorkDiscipline": secWorkDiscipline, "secBlockerRouting": secBlockerRouting,
		"secContextLost": secContextLost, "secCommandsFail": secCommandsFail, "secNeverEnd": secNeverEnd,
		"secAwaitVerdict": secAwaitVerdict, "secOnPass": secOnPass, "secOnFail": onFailSection(),
		"secDoneNote": secDoneNote, "secFailNote": secFailNote, "secShipIntro": secShipIntro,
		"secShipSteps": secShipSteps, "secTriageCriteriaCheck": secTriageCriteriaCheck,
		"triageOutputContract": triageOutputContract(), "retriageOutputContract": retriageOutputContract(), "helpOwnerActionLine": helpOwnerActionLine,
	}
}

// Seeded templates are validated on write, so a section that fails here would
// make the sdd seed (or an operator edit built from it) unsavable.
func TestPromptSections_should_PassSeedValidation(t *testing.T) {
	t.Parallel()
	for name, section := range allPromptSections() {
		if err := validateNoShellMetacharacters(name, forSeed(section)); err != nil {
			t.Errorf("%v", err)
		}
		if err := validateRecognizedPlaceholders(name, forSeed(section)); err != nil {
			t.Errorf("%v", err)
		}
		if strings.Contains(forSeed(section), codeMark) {
			t.Errorf("%s: codeMark left in seed text", name)
		}
	}
}

// The sdd ship template once drifted from the default and lost the
// report_pr_created step; both now render the same sections.
func TestShipTemplates_should_BothRequireReportPRCreated(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{"sdd": sddShipCommandTemplate, "default": forDefault(secShipSteps)} {
		if !strings.Contains(text, "report_pr_created") {
			t.Errorf("%s ship text must require report_pr_created", name)
		}
	}
}

func TestSDDTemplates_should_ShareTheDefaultProtocolRules(t *testing.T) {
	t.Parallel()
	shared := map[string]string{
		"work discipline": forSeed(secWorkDiscipline), "blocker routing": forSeed(secBlockerRouting),
		"never end": forSeed(secNeverEnd), "on pass": forSeed(secOnPass), "on fail": forSeed(onFailSection()),
	}
	for name, want := range shared {
		if !strings.Contains(sddInitialPromptTemplate, want) {
			t.Errorf("sdd initial prompt is missing the shared %s section", name)
		}
		if !strings.Contains(strings.ReplaceAll(taskProtocolBlock, "`", ""), want) {
			t.Errorf("default protocol is missing the shared %s section", name)
		}
	}
}
