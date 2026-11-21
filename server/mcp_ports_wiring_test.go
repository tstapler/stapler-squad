package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
)

func TestMCPAutoReopener_should_ReturnNilInterface_When_NoBacklogService(t *testing.T) {
	t.Parallel()
	assert.True(t, mcpAutoReopener(nil) == nil, "a nil service must not become a typed-nil interface")
}

func TestMCPAutoReopener_should_ReturnTheService_When_Present(t *testing.T) {
	t.Parallel()
	svc := &backlogsvc.BacklogService{}
	assert.Same(t, svc, mcpAutoReopener(svc))
}
