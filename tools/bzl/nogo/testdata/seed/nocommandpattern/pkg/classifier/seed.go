// Package classifier is a deliberate nocommandpattern violation (its import path ends in pkg/classifier); never import it.
package classifier

import "regexp"

type Rule struct {
	ID             string
	CommandPattern *regexp.Regexp
}

// Violation sets CommandPattern with no //nolint:commandpattern justification.
var Violation = Rule{
	ID:             "seed",
	CommandPattern: regexp.MustCompile(`curl\s+.*`),
}
