package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"voltforge/internal/domain"
)

func TestLoginAndLogoutRevokesSession(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "POST", "/api/v1/auth/login", `{"username":"labreviewer","password":"labreviewer-demo"}`)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rr.Code, rr.Body.String())
	}
	var session struct{ ID string }
	if err := json.Unmarshal(rr.Body.Bytes(), &session); err != nil || session.ID == "" {
		t.Fatalf("invalid session: %v %s", err, rr.Body.String())
	}
	logout := makeRequest(t, "POST", "/api/v1/auth/logout", "")
	logout.Header.Set("Authorization", "Bearer "+session.ID)
	logoutResult := httptest.NewRecorder()
	srv.Routes().ServeHTTP(logoutResult, logout)
	if logoutResult.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logoutResult.Code, logoutResult.Body.String())
	}
	repeated := makeRequest(t, "POST", "/api/v1/auth/logout", "")
	repeated.Header.Set("Authorization", "Bearer "+session.ID)
	repeatedResult := httptest.NewRecorder()
	srv.Routes().ServeHTTP(repeatedResult, repeated)
	if repeatedResult.Code != http.StatusUnauthorized {
		t.Fatalf("revoked status=%d", repeatedResult.Code)
	}
}

func TestHealthz(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "GET", "/healthz", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp["status"])
}

func TestReadyz(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "GET", "/readyz", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "ready", resp["status"])
}

func TestRegisterHandshakeAPI(t *testing.T) {
	srv, _, _, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	body := `{"form_no":"H-API-001","date":"2026-08-19","protocol_id":"R001","adapter_model":"V001","outbound_product":"JMS-01","arrival_product":"JMS-02","owner_id":"tester","session_items":[{"session_no":"M-API-1","vendor_id":"S","lab_id":"R"}]}`
	req := makeRequest(t, "POST", "/api/v1/handshakes", body)
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	_ = ctx
	var form map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &form))
	assert.Equal(t, "H-API-001", form["form_no"])
	assert.Equal(t, "draft", form["state"])
}

func TestGetHandshakeNotFound(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "GET", "/api/v1/handshakes/nonexistent", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "not_found", errResp.Error.Code)
}

func TestPaginationAPI(t *testing.T) {
	srv, store, clock, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	for i := 0; i < 15; i++ {
		form := &domain.ProtocolHandshake{
			ID: "h-page-" + itoa(i), FormNo: "HP" + itoa(i), Date: "2026-08-19",
			ProtocolID: "R001", AdapterModel: "V001", State: domain.HandshakeStateDraft,
			ChargeSessionCount: 1, OwnerID: "test",
			RegisteredAt: clock.Now(), UpdatedAt: clock.Now(),
			Version: 1, ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
		}
		require.NoError(t, store.HandshakeRepo().Save(ctx, form))
		clock.Advance(1e9)
	}
	req := makeRequest(t, "GET", "/api/v1/handshakes?page_size=10&page_offset=0", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp PaginatedResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 15, resp.Total)
	assert.Equal(t, 10, resp.PageSize)
	assert.True(t, resp.HasNext)
}

func TestImportHandshakesCSV(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	csvBody := "form_no,date,protocol_id,adapter_model,outbound_product,arrival_product,owner_id,session_no,vendor_id,cable_id,lab_id,firmware_version\n" +
		"H-IMP-1,2026-08-19,R001,V001,JMS-01,JMS-02,tester,M-IMP-1,S1,addr1,R1,addr2\n" +
		"H-IMP-2,2026-08-19,R001,V001,JMS-01,JMS-02,tester,M-IMP-2,S2,addr1,R2,addr2\n" +
		",2026-08-19,R001,V001,JMS-01,JMS-02,tester,M-IMP-3,S3,addr1,R3,addr2\n"
	req := makeRequest(t, "POST", "/api/v1/imports/handshakes", csvBody)
	req.Header.Set("Content-Type", "text/csv")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var result map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result))
	assert.Equal(t, float64(3), result["total_rows"])
	assert.Equal(t, float64(2), result["succeeded"])
	assert.Equal(t, float64(1), result["failed"])
}

func TestExportTelemetryAPI(t *testing.T) {
	srv, store, clock, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	entry := &domain.TelemetryEntry{
		ID: "le-1", Date: "2026-08-19", ProtocolID: "R001", VolumeNo: "2026-08-19_R001",
		FormNo: "H-EXP-1", EntryType: domain.TelemetryEntryTypeRegistration,
		SessionNo: "M-EXP-1", OwnerID: "tester", Description: "requested",
		CreatedAt: clock.Now(), ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
	}
	require.NoError(t, store.TelemetryRepo().Save(ctx, entry))
	req := makeRequest(t, "GET", "/api/v1/telemetry/export?date=2026-08-19&protocol_id=R001", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "H-EXP-1")
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/csv")
}

func TestSubmitMitigationAPI(t *testing.T) {
	srv, store, clock, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	session := &domain.ChargeSession{
		ID: "session-api-1", SessionNo: "MN-API-1", ProtocolID: "R001", AdapterModel: "V001",
		State: domain.SessionStateNegotiating, DeviceModel: "JMS-01", ChargerModel: "JMS-02",
		VendorID: "S", LabID: "R", OwnerID: "test",
		RegisteredAt: clock.Now(), Version: 1,
		ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
	}
	require.NoError(t, store.ChargeSessionRepo().Save(ctx, session))
	body := `{"session_id":"session-api-1","type":"thermal_throttle","submitted_by":"product-op"}`
	req := makeRequest(t, "POST", "/api/v1/mitigations", body)
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var disp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &disp))
	assert.Equal(t, "pending", disp["state"])
}

func TestOverduesAPI(t *testing.T) {
	srv, store, clock, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	session := &domain.ChargeSession{
		ID: "session-od-1", SessionNo: "MN-OD-1", ProtocolID: "R001", AdapterModel: "V001",
		State: domain.SessionStateNegotiating, DeviceModel: "JMS-01", ChargerModel: "JMS-02",
		VendorID: "S", LabID: "R", OwnerID: "test",
		RegisteredAt: clock.Now().Add(-72 * 3600e9), Version: 1,
		ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
	}
	require.NoError(t, store.ChargeSessionRepo().Save(ctx, session))
	req := makeRequest(t, "GET", "/api/v1/overdues", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var report map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &report))
	assert.NotNil(t, report["overdue_sessions"])
}

func TestCertifyDataAPI(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "POST", "/api/v1/maintenance/certify", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var report map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &report))
	assert.Contains(t, report, "total_shards")
}
