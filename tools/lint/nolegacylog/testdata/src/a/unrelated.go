// This file's basename ("unrelated.go") is NOT in protectedFiles, so the
// same legacy Printf pattern here must never be flagged — proves the
// analyzer is scoped by file, not by call shape alone.
package a

import "github.com/tstapler/stapler-squad/log"

func stillLegacyButUnprotected(err error) {
	log.WarningLog().Printf("[SomeOtherFile] not yet migrated: %v", err)
}
