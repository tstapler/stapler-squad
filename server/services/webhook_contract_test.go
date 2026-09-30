package services

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"

	"github.com/tstapler/stapler-squad/server/integrations/contractbundle"
)

type httpRecorder = httptest.ResponseRecorder

// These tests bind the checked-in contract (contracts/webhook-management/v1) to the real
// handler: the schemas must accept the fixtures and the live responses, and the golden
// fixtures must equal what the server actually returns, so the contract cannot drift
// from the implementation unnoticed.

const contractDir = "../../contracts/webhook-management/v1"

func readContract(t *testing.T, rel string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(contractDir, rel))
	require.NoError(t, err)
	return raw
}

func schemaResult(t *testing.T, schemaFile string, doc []byte) *gojsonschema.Result {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join(contractDir, "schemas", schemaFile))
	require.NoError(t, err)
	schema, err := gojsonschema.NewSchema(gojsonschema.NewReferenceLoader("file://" + abs))
	require.NoError(t, err)
	res, err := schema.Validate(gojsonschema.NewBytesLoader(doc))
	require.NoError(t, err)
	return res
}

func requireValid(t *testing.T, schemaFile string, doc []byte) {
	t.Helper()
	res := schemaResult(t, schemaFile, doc)
	assert.True(t, res.Valid(), "%s rejected: %v\nbody: %s", schemaFile, res.Errors(), doc)
}

func requireInvalid(t *testing.T, schemaFile string, doc []byte) {
	t.Helper()
	assert.False(t, schemaResult(t, schemaFile, doc).Valid(), "%s should have rejected: %s", schemaFile, doc)
}

func asMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func TestContractFixtures_should_ValidateAgainstTheirSchemas(t *testing.T) {
	requireValid(t, "capability-response.schema.json", readContract(t, "fixtures/capability-response.json"))
	requireValid(t, "reconcile-request.schema.json", readContract(t, "fixtures/reconcile-request.create.json"))
	requireValid(t, "reconcile-response.schema.json", readContract(t, "fixtures/reconcile-response.created.json"))
	requireValid(t, "error-response.schema.json", readContract(t, "fixtures/error.version-conflict.json"))
	requireValid(t, "error-response.schema.json", readContract(t, "fixtures/error.unauthenticated.json"))
	requireValid(t, "lifecycle-request.schema.json", readContract(t, "fixtures/lifecycle-request.disable.json"))
	requireValid(t, "emergency-cleanup-request.schema.json", readContract(t, "fixtures/emergency-cleanup-request.delete.json"))
}

func TestContractSchemas_should_RejectUnsafeLifecycleRequests(t *testing.T) {
	clone := func(fixture string, f func(map[string]any)) []byte {
		m := asMap(t, readContract(t, fixture))
		f(m)
		raw, err := json.Marshal(m)
		require.NoError(t, err)
		return raw
	}
	const lifecycle, emergency = "fixtures/lifecycle-request.disable.json", "fixtures/emergency-cleanup-request.delete.json"

	requireInvalid(t, "lifecycle-request.schema.json", clone(lifecycle, func(m map[string]any) { m["expected_version"] = 0 }))
	requireInvalid(t, "lifecycle-request.schema.json", clone(lifecycle, func(m map[string]any) { delete(m, "compat") }))
	requireInvalid(t, "lifecycle-request.schema.json", clone(lifecycle, func(m map[string]any) { m["surprise"] = true }))
	requireInvalid(t, "emergency-cleanup-request.schema.json", clone(emergency, func(m map[string]any) { m["action"] = "reconcile" }))
	requireInvalid(t, "emergency-cleanup-request.schema.json", clone(emergency, func(m map[string]any) { delete(m, "action") }))
}

func TestContractSchemas_should_BeStrict_When_GivenUnsafeOrUnknownInput(t *testing.T) {
	mutate := func(f func(map[string]any)) []byte {
		clone := asMap(t, readContract(t, "fixtures/reconcile-request.create.json"))
		f(clone)
		raw, err := json.Marshal(clone)
		require.NoError(t, err)
		return raw
	}

	requireInvalid(t, "reconcile-request.schema.json", mutate(func(m map[string]any) { m["surprise"] = 1 }))
	requireInvalid(t, "reconcile-request.schema.json", mutate(func(m map[string]any) { m["secret"] = "short" }))
	requireInvalid(t, "reconcile-request.schema.json", mutate(func(m map[string]any) { delete(m, "compat") }))
	requireInvalid(t, "reconcile-request.schema.json", mutate(func(m map[string]any) {
		m["webhook"].(map[string]any)["target_directory"] = "relative/dir"
	}))
	requireInvalid(t, "reconcile-request.schema.json", mutate(func(m map[string]any) {
		m["webhook"].(map[string]any)["slug"] = "Not A Slug"
	}))

	resp := func(f func(map[string]any)) []byte {
		clone := asMap(t, readContract(t, "fixtures/reconcile-response.created.json"))
		f(clone)
		raw, err := json.Marshal(clone)
		require.NoError(t, err)
		return raw
	}
	requireInvalid(t, "reconcile-response.schema.json", resp(func(m map[string]any) { m["secret"] = "leaked" }))
	requireInvalid(t, "reconcile-response.schema.json", resp(func(m map[string]any) {
		m["registration"].(map[string]any)["secret_digest"] = "leaked"
	}))
	requireInvalid(t, "error-response.schema.json", []byte(`{"error":{"code":"MADE_UP","message":"x"}}`))
}

func TestContract_should_MatchLiveServer_When_ExercisedThroughTheRealHandler(t *testing.T) {
	h := newMgmtHarness(t)

	capRec := h.do(t, http.MethodGet, webhookManagementBasePath+"/capability", h.token, nil)
	require.Equal(t, http.StatusOK, capRec.Code)
	requireValid(t, "capability-response.schema.json", capRec.Body.Bytes())
	assert.Equal(t, asMap(t, readContract(t, "fixtures/capability-response.json")), asMap(t, capRec.Body.Bytes()),
		"the golden capability fixture must equal what the server returns")

	var req WebhookReconcileRequest
	require.NoError(t, json.Unmarshal(readContract(t, "fixtures/reconcile-request.create.json"), &req))
	req.Webhook.TargetDirectory = h.root // the only field that must be environment-specific
	rec := h.do(t, http.MethodPost, mgmtBase+"example-instance/reconcile", h.token, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	requireValid(t, "reconcile-response.schema.json", rec.Body.Bytes())

	want := asMap(t, readContract(t, "fixtures/reconcile-response.created.json"))
	got := asMap(t, rec.Body.Bytes())
	for _, m := range []map[string]any{want, got} {
		reg := m["registration"].(map[string]any)
		for _, volatile := range []string{"registration_id", "workflow_id", "created_at", "updated_at"} {
			reg[volatile] = "<volatile>"
		}
		reg["webhook"].(map[string]any)["target_directory"] = "<env>"
	}
	assert.Equal(t, want, got, "the golden response fixture must equal what the server returns, "+
		"including compat_tuple_sha256 (which proves a client can reproduce the canonical tuple hash)")

	insp := h.do(t, http.MethodGet, mgmtBase+"example-instance", h.token, nil)
	require.Equal(t, http.StatusOK, insp.Code)
	requireValid(t, "registration.schema.json", insp.Body.Bytes())
}

func TestContract_should_ReturnSchemaValidErrors_When_EachFailureClassOccurs(t *testing.T) {
	h := newMgmtHarness(t)
	base := func(requestID string, expected int64, secret string) WebhookReconcileRequest {
		return h.reconcileReq(requestID, expected, secret)
	}
	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, base("req-00000001", 0, testMgmtSecret)).Code)

	outside := base("req-00000002", 1, "")
	outside.Webhook.TargetDirectory = t.TempDir()
	differs := base("req-00000001", 0, testMgmtSecret)
	differs.Webhook.Name = "changed"
	unsupported := base("req-00000003", 1, "")
	unsupported.Compat.CapabilityRevision = 9

	cases := []struct {
		name   string
		rec    func() *httpRecorder
		status int
		code   string
	}{
		{"unauthenticated", func() *httpRecorder { return h.do(t, http.MethodGet, mgmtBase+testInstance, "", nil) }, 401, "UNAUTHENTICATED"},
		{"not found", func() *httpRecorder { return h.do(t, http.MethodGet, mgmtBase+"nope", h.token, nil) }, 404, "NOT_FOUND"},
		{"version conflict", func() *httpRecorder { return h.reconcile(t, h.token, base("req-00000004", 0, testMgmtSecret)) }, 409, "VERSION_CONFLICT"},
		{"idempotency key reused", func() *httpRecorder { return h.reconcile(t, h.token, differs) }, 409, "IDEMPOTENCY_KEY_REUSED"},
		{"forbidden directory", func() *httpRecorder { return h.reconcile(t, h.token, outside) }, 403, "FORBIDDEN_DIRECTORY"},
		{"unsupported capability", func() *httpRecorder { return h.reconcile(t, h.token, unsupported) }, 409, "UNSUPPORTED_CAPABILITY"},
		{"weak secret", func() *httpRecorder { return h.reconcile(t, h.token, base("req-00000005", 1, "weak")) }, 400, "WEAK_SECRET"},
		{"malformed body", func() *httpRecorder {
			return h.do(t, http.MethodPost, mgmtBase+testInstance+"/reconcile", h.token, []byte("not json"))
		}, 400, "INVALID_REQUEST"},
	}
	for _, tc := range cases {
		rec := tc.rec()
		assert.Equal(t, tc.status, rec.Code, tc.name)
		assert.Equal(t, tc.code, errCode(t, rec), tc.name)
		requireValid(t, "error-response.schema.json", rec.Body.Bytes())
	}
}

func TestContractBundle_should_SignAndVerify_When_BuiltFromTheCheckedInContract(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	staged := t.TempDir()
	require.NoError(t, copyTree(contractDir, staged))

	manifest, err := contractbundle.BuildManifest(staged)
	require.NoError(t, err)
	sig, err := contractbundle.Sign(manifest, "test-key", priv)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(staged, contractbundle.ManifestFile), manifest, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(staged, contractbundle.SignatureFile), sig, 0o644))

	rootJSON, err := json.Marshal(contractbundle.TrustRoot{Version: 1, Keys: []contractbundle.TrustKey{{
		ID: "test-key", PublicKey: base64.StdEncoding.EncodeToString(pub), NotBefore: time.Unix(0, 0).UTC(),
	}}})
	require.NoError(t, err)
	root, err := contractbundle.ParseTrustRoot(rootJSON)
	require.NoError(t, err)

	got, err := contractbundle.Verify(staged, root, time.Now())

	require.NoError(t, err)
	assert.Contains(t, got.Files, "schemas/reconcile-request.schema.json")
	assert.Contains(t, got.Files, "fixtures/capability-response.json")
	assert.Len(t, got.Files, 14, "every checked-in contract artifact must be in the bundle")

	require.NoError(t, os.WriteFile(filepath.Join(staged, "fixtures/capability-response.json"), []byte(`{}`), 0o644))
	_, err = contractbundle.Verify(staged, root, time.Now())
	assert.ErrorIs(t, err, contractbundle.ErrVerification, "altering any artifact after signing must fail verification")
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
}
