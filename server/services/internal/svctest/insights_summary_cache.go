package services

import (
	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func summaryReq(orphans bool) *connect.Request[sessionv1.GetInsightsSummaryRequest] {
	return connect.NewRequest(&sessionv1.GetInsightsSummaryRequest{IncludeOrphans: orphans})
}
