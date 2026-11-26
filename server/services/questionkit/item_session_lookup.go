package questionkit

import (
	"context"

	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

type ItemSessionLookup interface {
	GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (backlogdto.ItemSessionSummary, error)
}
