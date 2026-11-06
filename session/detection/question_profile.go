package detection

import (
	"sort"

	"github.com/tstapler/stapler-squad/session/detection/dtypes"
)

// QuestionProfileRegistry holds the replyable harnesses. A harness absent from
// it is read_only: no ANSWER_QUESTION action and no replyable row is created.
// It is built once at startup and read-only afterwards.
type QuestionProfileRegistry struct {
	profiles map[string]dtypes.QuestionDialogProfile
}

// NewQuestionProfileRegistry returns an empty registry.
func NewQuestionProfileRegistry() *QuestionProfileRegistry {
	return &QuestionProfileRegistry{profiles: map[string]dtypes.QuestionDialogProfile{}}
}

// Register adds p under its name, replacing an earlier profile of that name.
func (r *QuestionProfileRegistry) Register(p dtypes.QuestionDialogProfile) {
	r.profiles[p.Name()] = p
}

// Lookup returns the profile named name.
func (r *QuestionProfileRegistry) Lookup(name string) (dtypes.QuestionDialogProfile, bool) {
	p, ok := r.profiles[name]
	return p, ok
}

// Names lists the replyable profile names, sorted.
func (r *QuestionProfileRegistry) Names() []string {
	names := make([]string, 0, len(r.profiles))
	for n := range r.profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Extract tries every profile and returns the first that matches, in name order.
func (r *QuestionProfileRegistry) Extract(capture string) (dtypes.ProfileDialog, bool) {
	for _, n := range r.Names() {
		if d, ok := r.profiles[n].Extract(capture); ok {
			return d, true
		}
	}
	return dtypes.ProfileDialog{}, false
}

// Has reports whether name is replyable.
func (r *QuestionProfileRegistry) Has(name string) bool {
	_, ok := r.profiles[name]
	return ok
}
