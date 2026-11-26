package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Not parallel: t.Setenv.
func TestServerInfo_should_OfferE2EHooksOnlyOnTheE2EInstance_When_Requested(t *testing.T) {
	for name, wantHooks := range map[string]bool{"": false, "shared": false, "e2e-local": true} {
		t.Run("instance="+name, func(t *testing.T) {
			t.Setenv("STAPLER_SQUAD_INSTANCE", name)
			srv := &Server{mux: http.NewServeMux()}
			srv.registerServerInfoHandler()

			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/server-info", nil))

			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			_, present := body["e2e_hooks"]
			assert.Equal(t, wantHooks, present)
		})
	}
}
