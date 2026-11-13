package artifacts

import (
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// domain cannot import log, so the skipped-record warning is registered here.
func init() {
	domain.SetRecordSkipObserver(func(err error) {
		log.Warn("[ArtifactExtractor] skipping unparseable artifact record", "err", err)
	})
}
