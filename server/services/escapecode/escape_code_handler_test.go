package escapecode

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeCodeHandler_WriteJSON_EncodeFailureIs500(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	(&EscapeCodeHandler{}).writeJSON(rec, make(chan int))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "Failed to encode response")
}
