// Package a contains test fixtures for the nocommandpattern analyzer.
package a

import "regexp"

// Rule mirrors the minimal shape of pkg/classifier.Rule for testing.
type Rule struct {
	ID             string
	Name           string
	CommandPattern *regexp.Regexp
	Criteria       *CommandCriteria
}

// CommandCriteria mirrors pkg/classifier.CommandCriteria for testing.
type CommandCriteria struct {
	Programs    []string
	Subcommands []string
}

// BAD1: CommandPattern set without any justification comment.
var bad1 = Rule{
	ID:             "bad-rule-1",
	CommandPattern: regexp.MustCompile(`curl\s+.*`), // want `CommandPattern set without a //nolint:commandpattern justification comment`
}

// BAD2: CommandPattern set; justification comment is on an unrelated line.
var bad2 = Rule{
	ID: "bad-rule-2",
	// This comment does not justify the regex below.
	CommandPattern: regexp.MustCompile(`wget\s+.*`), // want `CommandPattern set without a //nolint:commandpattern justification comment`
}

// GOOD1: CommandPattern justified with nolint comment on the same line.
var good1 = Rule{
	ID:             "good-rule-1",
	CommandPattern: regexp.MustCompile(`source\s+.*/activate`), //nolint:commandpattern Criteria cannot match argument path content
}

// GOOD2: CommandPattern justified with nolint comment on the preceding line.
var good2 = Rule{
	ID: "good-rule-2",
	//nolint:commandpattern Criteria cannot express flag-value matching; must grep for --output=
	CommandPattern: regexp.MustCompile(`--output=\S+`),
}

// GOOD3: Uses Criteria instead — no CommandPattern, no diagnostic.
var good3 = Rule{
	ID: "good-rule-3",
	Criteria: &CommandCriteria{
		Programs:    []string{"git"},
		Subcommands: []string{"status", "log"},
	},
}
