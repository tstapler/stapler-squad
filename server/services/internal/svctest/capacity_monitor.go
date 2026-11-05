package services

import (
	"github.com/tstapler/stapler-squad/session"
)

type mockInstancePoller struct {
	instances []*session.Instance
}

func (m *mockInstancePoller) GetInstances() []*session.Instance {
	return m.instances
}
