package session

import (
	"errors"
	"testing"
	"time"
)

// fakeInstanceStore is a minimal InstanceStore fake for domain-level tests
// that only need ListInstanceData (collision checks) or DeleteInstance
// (compensating-delete / cancel-kill paths). Unused methods panic loudly
// rather than silently no-opping, so a test that accidentally exercises an
// unwired path fails fast instead of masking a bug.
type fakeInstanceStore struct {
	instances []InstanceData

	added     []*Instance
	addErr    error
	deleteErr error

	deletedTitle    string
	deletedInstance *Instance
}

func (f *fakeInstanceStore) LoadInstances() ([]*Instance, error) {
	panic("fakeInstanceStore: LoadInstances not implemented")
}

func (f *fakeInstanceStore) ListInstanceData() ([]InstanceData, error) {
	return f.instances, nil
}

func (f *fakeInstanceStore) SaveInstances([]*Instance) error {
	panic("fakeInstanceStore: SaveInstances not implemented")
}

func (f *fakeInstanceStore) AddInstance(inst *Instance) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, inst)
	return nil
}

func (f *fakeInstanceStore) DeleteInstance(title string) error {
	f.deletedTitle = title
	if f.deleteErr != nil {
		return f.deleteErr
	}
	kept := make([]*Instance, 0, len(f.added))
	for _, inst := range f.added {
		if inst.Title == title {
			f.deletedInstance = inst
		} else {
			kept = append(kept, inst)
		}
	}
	f.added = kept
	return nil
}

// hasInstance reports whether an instance with the given title is currently
// tracked as added (i.e. not yet deleted).
func (f *fakeInstanceStore) hasInstance(title string) bool {
	for _, inst := range f.added {
		if inst.Title == title {
			return true
		}
	}
	return false
}

func (f *fakeInstanceStore) UpdateInstanceLastUserResponse(string, time.Time) error {
	panic("fakeInstanceStore: UpdateInstanceLastUserResponse not implemented")
}

func TestCheckPathNotAlreadyManaged_ReturnsErrPathAlreadyManaged_When_ExactPathCollides(t *testing.T) {
	store := &fakeInstanceStore{
		instances: []InstanceData{
			{Title: "existing", Path: "/tmp/import-collision/repo"},
		},
	}

	err := CheckPathNotAlreadyManaged("/tmp/import-collision/repo", store)

	if !errors.Is(err, ErrPathAlreadyManaged) {
		t.Fatalf("expected ErrPathAlreadyManaged, got %v", err)
	}
}

func TestCheckPathNotAlreadyManaged_ReturnsErrPathAlreadyManaged_When_CandidateIsSubdirOfExistingWorktree(t *testing.T) {
	store := &fakeInstanceStore{
		instances: []InstanceData{
			{
				Title:    "existing",
				Worktree: GitWorktreeData{WorktreePath: "/tmp/import-collision/repo"},
			},
		},
	}

	err := CheckPathNotAlreadyManaged("/tmp/import-collision/repo/subdir", store)

	if !errors.Is(err, ErrPathAlreadyManaged) {
		t.Fatalf("expected ErrPathAlreadyManaged, got %v", err)
	}
}

func TestCheckPathNotAlreadyManaged_ReturnsErrPathAlreadyManaged_When_ExistingWorktreeIsSubdirOfCandidate(t *testing.T) {
	store := &fakeInstanceStore{
		instances: []InstanceData{
			{
				Title:    "existing",
				Worktree: GitWorktreeData{WorktreePath: "/tmp/import-collision/repo/nested"},
			},
		},
	}

	err := CheckPathNotAlreadyManaged("/tmp/import-collision/repo", store)

	if !errors.Is(err, ErrPathAlreadyManaged) {
		t.Fatalf("expected ErrPathAlreadyManaged, got %v", err)
	}
}

func TestCheckPathNotAlreadyManaged_ReturnsNil_When_NoCollision(t *testing.T) {
	store := &fakeInstanceStore{
		instances: []InstanceData{
			{Title: "existing", Path: "/tmp/import-collision/other-repo"},
		},
	}

	err := CheckPathNotAlreadyManaged("/tmp/import-collision/repo", store)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCheckPathNotAlreadyManaged_ReturnsNil_When_NoExistingInstances(t *testing.T) {
	store := &fakeInstanceStore{instances: nil}

	err := CheckPathNotAlreadyManaged("/tmp/import-collision/repo", store)

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCheckPathNotAlreadyManaged_ReturnsError_When_RegistryNil(t *testing.T) {
	err := CheckPathNotAlreadyManaged("/tmp/import-collision/repo", nil)

	if err == nil {
		t.Fatal("expected error when registry is nil, got nil")
	}
}
