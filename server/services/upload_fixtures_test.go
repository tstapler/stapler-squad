package services

import (
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// fakeInstanceStore is a minimal session.InstanceStore stub for SessionService tests.
type fakeInstanceStore struct {
	instances []*instance.Instance
	loadErr   error
}

func (f *fakeInstanceStore) LoadInstances() ([]*instance.Instance, error) {
	return f.instances, f.loadErr
}

func (f *fakeInstanceStore) ListInstanceData() ([]domain.InstanceData, error) {
	return nil, nil
}

func (f *fakeInstanceStore) FindInstanceDataByID(id string) (*domain.InstanceData, error) {
	for _, inst := range f.instances {
		data := inst.ToInstanceData()
		if data.MatchesID(id) {
			return &data, nil
		}
	}
	return nil, domain.ErrInstanceDataNotFound
}

func (f *fakeInstanceStore) SaveInstances([]*instance.Instance) error { return nil }

func (f *fakeInstanceStore) AddInstance(*instance.Instance) error { return nil }

func (f *fakeInstanceStore) DeleteInstance(_ string) error { return nil }

func (f *fakeInstanceStore) UpdateInstanceLastUserResponse(_ string, _ time.Time) error { return nil }

func (f *fakeInstanceStore) UpdateInstanceMetadata(_ string, _, _, _, _ *string) error { return nil }

// pngMagic returns a minimal byte slice that looks like a PNG.
func pngMagic() []byte {
	return append([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 504)...)
}
