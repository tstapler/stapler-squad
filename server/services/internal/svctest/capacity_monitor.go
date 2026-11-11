package svctest

import (
	"github.com/tstapler/stapler-squad/session"
)

type MockInstancePoller struct {
	Instances []*session.Instance
}

func (m *MockInstancePoller) GetInstances() []*session.Instance {
	return m.Instances
}
