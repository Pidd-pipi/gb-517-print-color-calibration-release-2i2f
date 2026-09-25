package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/config"
	"github.com/blueship581/print-color-calibration-release/backend/internal/database"
	"github.com/blueship581/print-color-calibration-release/backend/internal/router"
	"github.com/gin-gonic/gin"
)

type apiEnvelope struct {
	Data json.RawMessage `json:"data"`
}

func TestRBACAndImmutableRevisionFlows(t *testing.T) {
	cfg := testConfig(filepath.Join(t.TempDir(), "gb517.db"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, redisClient, err := database.Open(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	engine := router.New(cfg, db, redisClient, logger)
	tokens := map[string]string{}
	for _, role := range []string{"viewer", "operator", "reviewer", "admin"} {
		tokens[role] = loginToken(t, engine, role)
	}

	// --- Build a batch that will be driven by its proof gate ---------------
	runPayload := recordPayload("PR-TEST-001", "测试色彩配置")
	runPayload["colorTolerance"] = 3.0
	status, body := perform(t, engine, http.MethodPost, "/api/runs", tokens["operator"], "run-create", runPayload)
	run := decodeData[struct {
		ID      uint    `json:"id"`
		Version uint    `json:"version"`
		Tol     float64 `json:"colorTolerance"`
	}](t, body)
	if status != http.StatusCreated {
		t.Fatalf("create run status = %d body=%s", status, body)
	}
	if run.Tol != 3.0 {
		t.Fatalf("batch tolerance = %v, want 3.0", run.Tol)
	}
	runPath := "/api/runs/" + uintString(run.ID) + "/transition"
	if status, _ := perform(t, engine, http.MethodPost, runPath, tokens["operator"], "run-printing",
		map[string]any{"status": "printing", "expectedVersion": run.Version, "reason": "plates and ink verified"}); status != http.StatusOK {
		t.Fatalf("run transition status = %d", status)
	}
	// printing -> proofing (operator may advance to the gate)
	status, body = perform(t, engine, http.MethodPost, runPath, tokens["operator"], "run-proofing",
		map[string]any{"status": "proofing", "expectedVersion": 2, "reason": "ready for colour proof"})
	if status != http.StatusOK {
		t.Fatalf("run proofing transition status = %d body=%s", status, body)
	}
	run.Version = 3

	// A batch without a passing proof cannot be released directly.
	if status, _ := perform(t, engine, http.MethodPost, runPath, tokens["reviewer"], "run-release-blocked",
		map[string]any{"status": "released", "expectedVersion": run.Version, "reason": "trying to skip proof"}); status != http.StatusConflict {
		t.Fatalf("release before proof status = %d, want 409", status)
	}

	// --- Capture and submit a passing proof --------------------------------
	proofPayload := recordPayload("CP-TEST-PASS", "合格校样")
	proofPayload["metricValue"] = 1.8
	proofPayload["metricUnit"] = "ΔE"
	proofPayload["printRunId"] = run.ID
	status, body = perform(t, engine, http.MethodPost, "/api/proofs", tokens["operator"], "proof-create-pass", proofPayload)
	if status != http.StatusCreated {
		t.Fatalf("create proof status = %d body=%s", status, body)
	}
	proof := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	proofPath := "/api/proofs/" + uintString(proof.ID) + "/transition"
	if status, _ := perform(t, engine, http.MethodPost, proofPath, tokens["operator"], "proof-review",
		map[string]any{"status": "review", "expectedVersion": proof.Version, "reason": "measurement captured"}); status != http.StatusOK {
		t.Fatalf("proof review status = %d", status)
	}
	proof.Version = 2
	// Operator may never accept.
	if status, _ := perform(t, engine, http.MethodPost, proofPath, tokens["operator"], "proof-accept-forbidden",
		map[string]any{"status": "accepted", "expectedVersion": proof.Version, "expectedRunVersion": run.Version, "reason": "operator cannot accept"}); status != http.StatusForbidden {
		t.Fatalf("operator accept status = %d, want 403", status)
	}
	// Accepting without the batch version is rejected (no partial write).
	if status, _ := perform(t, engine, http.MethodPost, proofPath, tokens["reviewer"], "proof-accept-nover",
		map[string]any{"status": "accepted", "expectedVersion": proof.Version, "reason": "missing run version"}); status != http.StatusConflict {
		t.Fatalf("accept without run version status = %d, want 409", status)
	}
	// Reviewer accepts with the in-tolerance measurement: batch stays proofing.
	status, body = perform(t, engine, http.MethodPost, proofPath, tokens["reviewer"], "proof-accept-pass",
		map[string]any{"status": "accepted", "expectedVersion": proof.Version, "expectedRunVersion": run.Version, "reason": "colour tolerance verified"})
	if status != http.StatusOK {
		t.Fatalf("reviewer accept pass status = %d body=%s", status, body)
	}

	// Batch detail now carries the gate evidence and a new revision.
	_, body = perform(t, engine, http.MethodGet, "/api/runs/"+uintString(run.ID), tokens["reviewer"], "run-read-pass", nil)
	runAfter := decodeData[struct {
		Status       string  `json:"status"`
		Version      uint    `json:"version"`
		ProofVerdict string  `json:"proofVerdict"`
		Measured     float64 `json:"proofMeasuredDeltaE"`
		Tolerance    float64 `json:"proofTolerance"`
		Actor        string  `json:"proofActor"`
		ProofID      *uint   `json:"proofId"`
		Revisions    []struct {
			Version uint   `json:"version"`
			Actor   string `json:"actor"`
		} `json:"revisions"`
	}](t, body)
	if runAfter.Status != "proofing" || runAfter.ProofVerdict != "pass" || runAfter.Measured != 1.8 ||
		runAfter.Tolerance != 3.0 || runAfter.Actor != "reviewer" || runAfter.ProofID == nil || *runAfter.ProofID != proof.ID {
		t.Fatalf("unexpected batch gate evidence after pass: %+v", runAfter)
	}
	if runAfter.Version != run.Version+1 {
		t.Fatalf("batch version = %d, want %d", runAfter.Version, run.Version+1)
	}
	if len(runAfter.Revisions) < 4 {
		t.Fatalf("expected >=4 batch revisions, got %d", len(runAfter.Revisions))
	}
	run.Version = runAfter.Version

	// Now the batch can be released by the reviewer.
	if status, _ := perform(t, engine, http.MethodPost, runPath, tokens["reviewer"], "run-release-ok",
		map[string]any{"status": "released", "expectedVersion": run.Version, "reason": "proof passed, release"}); status != http.StatusOK {
		t.Fatalf("release after pass status = %d", status)
	}
	run.Version++

	// --- Release decisions are gated to the released, passed batch ---------
	decisionPayload := recordPayload("RD-TEST-001", "测试放行决定")
	decisionPayload["printRunId"] = run.ID
	if status, _ := perform(t, engine, http.MethodPost, "/api/release", tokens["viewer"], "viewer-create", decisionPayload); status != http.StatusForbidden {
		t.Fatalf("viewer create status = %d, want 403", status)
	}
	status, body = perform(t, engine, http.MethodPost, "/api/release", tokens["operator"], "decision-create", decisionPayload)
	if status != http.StatusCreated {
		t.Fatalf("operator create decision status = %d body=%s", status, body)
	}
	decision := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	transition := map[string]any{"status": "release", "expectedVersion": decision.Version, "reason": "quality gate accepted"}
	dpath := "/api/release/" + uintString(decision.ID) + "/transition"
	if status, _ := perform(t, engine, http.MethodPost, dpath, tokens["operator"], "operator-release", transition); status != http.StatusForbidden {
		t.Fatalf("operator release status = %d, want 403", status)
	}
	if status, body := perform(t, engine, http.MethodPost, dpath, tokens["reviewer"], "reviewer-release", transition); status != http.StatusOK {
		t.Fatalf("reviewer release status = %d body=%s", status, body)
	}
	status, body = perform(t, engine, http.MethodGet, "/api/release/"+uintString(decision.ID), tokens["reviewer"], "decision-read", nil)
	detail := decodeData[struct {
		Version   uint `json:"version"`
		Revisions []struct {
			Version   uint   `json:"version"`
			RequestID string `json:"requestId"`
		} `json:"revisions"`
	}](t, body)
	if status != http.StatusOK || detail.Version != 2 || len(detail.Revisions) != 2 || detail.Revisions[0].RequestID != "reviewer-release" {
		t.Fatalf("unexpected decision revision chain: status=%d detail=%+v", status, detail)
	}
	update := recordPayload("ignored", "不得覆盖的决定")
	update["printRunId"] = run.ID
	update["expectedVersion"] = detail.Version
	if status, _ := perform(t, engine, http.MethodPut, "/api/release/"+uintString(decision.ID), tokens["operator"], "locked-update", update); status != http.StatusConflict {
		t.Fatalf("resolved decision update status = %d, want 409", status)
	}
	if status, _ := perform(t, engine, http.MethodDelete, "/api/release/"+uintString(decision.ID), tokens["admin"], "locked-delete", nil); status != http.StatusConflict {
		t.Fatalf("resolved decision delete status = %d, want 409", status)
	}
	if status, _ := perform(t, engine, http.MethodDelete, "/api/runs/"+uintString(run.ID), tokens["admin"], "locked-run-delete", nil); status != http.StatusConflict {
		t.Fatalf("active run delete status = %d, want 409", status)
	}

	if status, _ := perform(t, engine, http.MethodGet, "/api/audits", tokens["viewer"], "viewer-audit", nil); status != http.StatusForbidden {
		t.Fatalf("viewer audit status = %d, want 403", status)
	}
	if status, _ := perform(t, engine, http.MethodGet, "/api/audits", tokens["reviewer"], "reviewer-audit", nil); status != http.StatusOK {
		t.Fatalf("reviewer audit status = %d, want 200", status)
	}
}

func TestProofFailParksBatchAndBlocksRelease(t *testing.T) {
	cfg := testConfig(filepath.Join(t.TempDir(), "gb517-fail.db"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, redisClient, err := database.Open(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	engine := router.New(cfg, db, redisClient, logger)
	operator := loginToken(t, engine, "operator")
	reviewer := loginToken(t, engine, "reviewer")

	runPayload := recordPayload("PR-FAIL-001", "超容差批次")
	runPayload["colorTolerance"] = 3.0
	_, body := perform(t, engine, http.MethodPost, "/api/runs", operator, "f-run-create", runPayload)
	run := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	runPath := "/api/runs/" + uintString(run.ID) + "/transition"
	for _, step := range []struct {
		status  string
		version uint
		reason  string
		req     string
	}{
		{"printing", 1, "ink ready", "f-run-printing"},
		{"proofing", 2, "proof ready", "f-run-proofing"},
	} {
		if status, _ := perform(t, engine, http.MethodPost, runPath, operator, step.req,
			map[string]any{"status": step.status, "expectedVersion": step.version, "reason": step.reason}); status != http.StatusOK {
			t.Fatalf("run -> %s status = %d", step.status, status)
		}
	}
	run.Version = 3

	proofPayload := recordPayload("CP-FAIL-001", "超容差校样")
	proofPayload["metricValue"] = 4.6
	proofPayload["metricUnit"] = "ΔE"
	proofPayload["printRunId"] = run.ID
	_, body = perform(t, engine, http.MethodPost, "/api/proofs", operator, "f-proof-create", proofPayload)
	proof := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	proofPath := "/api/proofs/" + uintString(proof.ID) + "/transition"
	if status, _ := perform(t, engine, http.MethodPost, proofPath, operator, "f-proof-review",
		map[string]any{"status": "review", "expectedVersion": proof.Version, "reason": "captured over limit sample"}); status != http.StatusOK {
		t.Fatalf("proof review status = %d", status)
	}
	proof.Version = 2
	// Acceptance is allowed, but the over-tolerance reading parks the batch.
	if status, body := perform(t, engine, http.MethodPost, proofPath, reviewer, "f-proof-accept",
		map[string]any{"status": "accepted", "expectedVersion": proof.Version, "expectedRunVersion": run.Version, "reason": "delta E over tolerance"}); status != http.StatusOK {
		t.Fatalf("accept over-tolerance proof status = %d body=%s", status, body)
	}
	_, body = perform(t, engine, http.MethodGet, "/api/runs/"+uintString(run.ID), reviewer, "f-run-read", nil)
	held := decodeData[struct {
		Status       string  `json:"status"`
		ProofVerdict string  `json:"proofVerdict"`
		Measured     float64 `json:"proofMeasuredDeltaE"`
		Tolerance    float64 `json:"proofTolerance"`
		Actor        string  `json:"proofActor"`
		Version      uint    `json:"version"`
	}](t, body)
	if held.Status != "hold" || held.ProofVerdict != "fail" || held.Measured != 4.6 || held.Tolerance != 3.0 || held.Actor != "reviewer" {
		t.Fatalf("batch should be parked in hold with fail evidence: %+v", held)
	}
	run.Version = held.Version

	// A held batch blocks release decisions even though the proof is accepted.
	decisionPayload := recordPayload("RD-FAIL-001", "不得放行决定")
	decisionPayload["printRunId"] = run.ID
	if status, _ := perform(t, engine, http.MethodPost, "/api/release", operator, "f-decision-blocked", decisionPayload); status != http.StatusConflict {
		t.Fatalf("draft decision on held batch status = %d, want 409", status)
	}
	// Direct batch release stays blocked.
	if status, _ := perform(t, engine, http.MethodPost, runPath, reviewer, "f-run-release-blocked",
		map[string]any{"status": "released", "expectedVersion": run.Version, "reason": "attempt release while held"}); status != http.StatusConflict {
		t.Fatalf("release held batch status = %d, want 409", status)
	}

	// Re-proof within tolerance lifts the hold back to proofing.
	second := recordPayload("CP-FAIL-002", "返修后合格校样")
	second["metricValue"] = 1.2
	second["metricUnit"] = "ΔE"
	second["printRunId"] = run.ID
	_, body = perform(t, engine, http.MethodPost, "/api/proofs", operator, "f-proof2-create", second)
	p2 := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	p2Path := "/api/proofs/" + uintString(p2.ID) + "/transition"
	if status, _ := perform(t, engine, http.MethodPost, p2Path, operator, "f-proof2-review",
		map[string]any{"status": "review", "expectedVersion": p2.Version, "reason": "rework sample captured"}); status != http.StatusOK {
		t.Fatalf("second proof review status = %d", status)
	}
	p2.Version = 2
	if status, _ := perform(t, engine, http.MethodPost, p2Path, reviewer, "f-proof2-accept",
		map[string]any{"status": "accepted", "expectedVersion": p2.Version, "expectedRunVersion": run.Version, "reason": "rework within tolerance"}); status != http.StatusOK {
		t.Fatalf("accept reworked proof status = %d", status)
	}
	_, body = perform(t, engine, http.MethodGet, "/api/runs/"+uintString(run.ID), reviewer, "f-run-lifted", nil)
	lifted := decodeData[struct {
		Status       string `json:"status"`
		ProofVerdict string `json:"proofVerdict"`
	}](t, body)
	if lifted.Status != "proofing" || lifted.ProofVerdict != "pass" {
		t.Fatalf("batch should return to proofing/pass after rework: %+v", lifted)
	}
}

func TestProofAcceptVersionConflictRollsBack(t *testing.T) {
	cfg := testConfig(filepath.Join(t.TempDir(), "gb517-conflict.db"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, redisClient, err := database.Open(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	engine := router.New(cfg, db, redisClient, logger)
	operator := loginToken(t, engine, "operator")
	reviewer := loginToken(t, engine, "reviewer")

	runPayload := recordPayload("PR-CONF-001", "版本冲突批次")
	runPayload["colorTolerance"] = 3.0
	_, body := perform(t, engine, http.MethodPost, "/api/runs", operator, "c-run-create", runPayload)
	run := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	runPath := "/api/runs/" + uintString(run.ID) + "/transition"
	for _, step := range []struct {
		status  string
		version uint
	}{
		{"printing", 1}, {"proofing", 2},
	} {
		if status, _ := perform(t, engine, http.MethodPost, runPath, operator, "c-run-"+step.status,
			map[string]any{"status": step.status, "expectedVersion": step.version, "reason": "advance batch"}); status != http.StatusOK {
			t.Fatalf("run -> %s status = %d", step.status, status)
		}
	}
	// Batch is now at version 3. Capture two proofs, both based on v3.
	makeProof := func(code string) struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	} {
		payload := recordPayload(code, "冲突校样 "+code)
		payload["metricValue"] = 1.1
		payload["metricUnit"] = "ΔE"
		payload["printRunId"] = run.ID
		_, b := perform(t, engine, http.MethodPost, "/api/proofs", operator, "c-"+code+"-create", payload)
		p := decodeData[struct {
			ID      uint `json:"id"`
			Version uint `json:"version"`
		}](t, b)
		pp := "/api/proofs/" + uintString(p.ID) + "/transition"
		if status, _ := perform(t, engine, http.MethodPost, pp, operator, "c-"+code+"-review",
			map[string]any{"status": "review", "expectedVersion": p.Version, "reason": "submitted"}); status != http.StatusOK {
			t.Fatalf("proof %s review status = %d", code, status)
		}
		p.Version = 2
		return p
	}
	first := makeProof("CP-CONF-001")
	second := makeProof("CP-CONF-002")

	// First acceptance moves run 3 -> 4 and proof first 2 -> 3.
	firstPath := "/api/proofs/" + uintString(first.ID) + "/transition"
	if status, _ := perform(t, engine, http.MethodPost, firstPath, reviewer, "c-accept-first",
		map[string]any{"status": "accepted", "expectedVersion": first.Version, "expectedRunVersion": 3, "reason": "first reviewer pass"}); status != http.StatusOK {
		t.Fatalf("first accept status = %d", status)
	}
	// Second review still carries the stale run version 3: optimistic lock must
	// abort the whole transaction, so the second proof must remain in review.
	secondPath := "/api/proofs/" + uintString(second.ID) + "/transition"
	if status, _ := perform(t, engine, http.MethodPost, secondPath, reviewer, "c-accept-stale",
		map[string]any{"status": "accepted", "expectedVersion": second.Version, "expectedRunVersion": 3, "reason": "stale batch version"}); status != http.StatusConflict {
		t.Fatalf("stale accept status = %d, want 409", status)
	}
	_, body = perform(t, engine, http.MethodGet, "/api/proofs/"+uintString(second.ID), reviewer, "c-proof2-read", nil)
	staleProof := decodeData[struct {
		Status  string `json:"status"`
		Version uint   `json:"version"`
	}](t, body)
	if staleProof.Status != "review" || staleProof.Version != second.Version {
		t.Fatalf("stale proof must stay in review at v%d, got status=%s v%d", second.Version, staleProof.Status, staleProof.Version)
	}
	// Retrying the second review with the current batch version succeeds.
	if status, _ := perform(t, engine, http.MethodPost, secondPath, reviewer, "c-accept-retry",
		map[string]any{"status": "accepted", "expectedVersion": second.Version, "expectedRunVersion": 4, "reason": "refreshed batch version"}); status != http.StatusOK {
		t.Fatalf("refreshed accept status = %d", status)
	}
}

func testConfig(dsn string) config.Config {
	return config.Config{
		AppName: "print-color-calibration-release", Environment: "test", Port: "0",
		DatabaseDriver: "sqlite", DatabaseDSN: dsn, JWTSecret: "gb517-router-tests-secret",
		TokenTTL: time.Hour, RequestLimit: 1000, StartupTimeout: time.Second,
		ShutdownTimeout: time.Second, ReadHeaderTimeout: time.Second, ReadTimeout: time.Second,
		WriteTimeout: time.Second, IdleTimeout: time.Second,
	}
}

func loginToken(t *testing.T, engine *gin.Engine, username string) string {
	t.Helper()
	status, body := perform(t, engine, http.MethodPost, "/api/auth/login", "", "login-"+username, map[string]any{"username": username, "password": "Admin123!"})
	if status != http.StatusOK {
		t.Fatalf("login %s status = %d body=%s", username, status, body)
	}
	return decodeData[struct {
		Token string `json:"token"`
	}](t, body).Token
}

func recordPayload(code, name string) map[string]any {
	return map[string]any{
		"code": code, "name": name, "description": "router integration test",
		"facility": "测试印刷区", "owner": "operator", "category": "校准",
		"riskLevel": "medium", "metricValue": 2.1, "metricUnit": "dE",
		"effectiveAt": time.Now().UTC().Format(time.RFC3339), "evidence": "spectrophotometer evidence", "relatedCode": "PR-001",
	}
}

func perform(t *testing.T, engine *gin.Engine, method, path, token, requestID string, payload any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("X-Request-ID", requestID)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response.Code, response.Body.Bytes()
}

func decodeData[T any](t *testing.T, body []byte) T {
	t.Helper()
	var envelope apiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode envelope %s: %v", body, err)
	}
	var value T
	if err := json.Unmarshal(envelope.Data, &value); err != nil {
		t.Fatalf("decode data %s: %v", envelope.Data, err)
	}
	return value
}

func uintString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
