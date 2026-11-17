package svctest

import (
	"github.com/tstapler/stapler-squad/session/instance"
)

type MockInstancePoller struct {
	Instances []*instance.Instance
}

func (m *MockInstancePoller) GetInstances() []*instance.Instance {
	return m.Instances
}
