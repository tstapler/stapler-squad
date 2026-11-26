package taggingrules

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

func TestTaggingRuleToSpec_should_CarryNamePattern_When_RuleHasOne(t *testing.T) {
	t.Parallel()
	spec := taggingRuleToSpec(classifier.TaggingRule{NamePattern: regexp.MustCompile("^n$")})
	assert.Equal(t, "^n$", spec.NamePattern)
}

func TestProtoToTaggingRuleSpec_should_LeaveCreatedAtZero_When_ProtoHasNone(t *testing.T) {
	t.Parallel()
	spec := protoToTaggingRuleSpec(&sessionv1.TaggingRuleProto{OutputTag: "o"})
	assert.True(t, spec.CreatedAt.IsZero(), "got %s", spec.CreatedAt)
}
