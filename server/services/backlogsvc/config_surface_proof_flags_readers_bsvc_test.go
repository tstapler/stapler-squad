package backlogsvc

import (
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/interceptors"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

func TestConfigSurfaceProof_BacklogFlag_ChangesBacklogRPCAnswer(t *testing.T) {
	set := bkfFlagWriter(t)
	svc, _, _, _ := enfService(t)
	_, handler := sessionv1connect.NewBacklogServiceHandler(svc,
		connect.WithInterceptors(interceptors.NewFeatureFlagInterceptor(services.BacklogFlagName, services.BacklogFlagEnabled)))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := sessionv1connect.NewBacklogServiceClient(srv.Client(), srv.URL)

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(services.BacklogFlagName, setting.(bool))
		_, err := client.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{}))
		return connect.CodeOf(err)
	}, false, true)
}
