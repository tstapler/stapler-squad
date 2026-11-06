package gitwiring

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/native/capability"
)

// NewCapabilityProbe builds the capability preflight (ADR-006) for backend.RouterConfig.Capabilities.
// The system config files are learned from git once, here; the probe itself never starts a
// process. When that discovery fails the probe fails closed for every operation that consults it
// (so those calls run on the CLI), rather than guessing which system file git reads.
func NewCapabilityProbe(ctx context.Context) backend.CapabilityProbe {
	files, err := DiscoverSystemConfig(ctx)
	return newCapabilityProbe(files, err)
}

func newCapabilityProbe(files SystemConfigFiles, discoveryErr error) backend.CapabilityProbe {
	if discoveryErr != nil {
		log.Warn("git system config discovery failed; capability preflight routes to the cli", "err", discoveryErr)
		return unknownSystemConfigProbe{err: discoveryErr}
	}
	return capability.New(capability.Config{SystemDefault: files.Default, SystemVendor: files.Vendor})
}

type unknownSystemConfigProbe struct{ err error }

func (p unknownSystemConfigProbe) Capabilities(context.Context, backend.Local, backend.OperationName) ([]backend.Capability, error) {
	return nil, &backend.DetectError{Sub: backend.DetectIO, Err: p.err}
}
