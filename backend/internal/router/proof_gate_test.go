package router_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/config"
	"github.com/blueship581/print-color-calibration-release/backend/internal/database"
	"github.com/blueship581/print-color-calibration-release/backend/internal/router"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func uniqueDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "gb517-gate.db")
}

func newTestEngine(t *testing.T, cfg config.Config) (*gorm.DB, *gin.Engine) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, redisClient, err := database.Open(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	return db, router.New(cfg, db, redisClient, logger)
}

// TestProofAcceptanceDrivesBatchGate exercises the requirement that accepting
// a proof directly decides the linked print run's fate: the measured value is
// checked against the same-batch tolerance, an out-of-tolerance proof parks
// the run in hold and blocks release decisions, the judgment evidence is kept
// on the run, and a batch version that changed during review prevents the
// proof status from persisting on its own.
func TestProofAcceptanceDrivesBatchGate(t *testing.T) {
	cfg := testConfig(uniqueDB(t))
	_, engine := newTestEngine(t, cfg)
	tokens := map[string]string{}
	for _, role := range []string{"operator", "reviewer"} {
		tokens[role] = loginToken(t, engine, role)
	}

	suffix := time.Now().UTC().Format("150405.000000000")
	runCode := "PR-GATE-" + suffix

	// A batch with a ΔE tolerance of 3.0, advanced setup -> printing -> proofing.
	runPayload := recordPayload(runCode, "校样联动批次")
	runPayload["metricUnit"] = "ΔE"
	runPayload["tolerance"] = 3.0
	status, body := perform(t, engine, http.MethodPost, "/api/runs", tokens["operator"], "gate-run-create", runPayload)
	if status != http.StatusCreated {
		t.Fatalf("create run status = %d body=%s", status, body)
	}
	run := decodeData[struct {
		ID      uint   `json:"id"`
		Version uint   `json:"version"`
		Status  string `json:"status"`
	}](t, body)
	for _, target := range []string{"printing", "proofing"} {
		status, body = perform(t, engine, http.MethodPost, "/api/runs/"+uintString(run.ID)+"/transition", tokens["operator"], "gate-run-"+target,
			map[string]any{"status": target, "expectedVersion": run.Version, "reason": "moving batch toward proofing gate"})
		if status != http.StatusOK {
			t.Fatalf("run -> %s status = %d body=%s", target, status, body)
		}
		run.Version++
	}

	// Reviewer alone may accept a proof.
	proofPayload := recordPayload("CP-GATE-BAD-"+suffix, "超限校样")
	proofPayload["metricUnit"] = "ΔE"
	proofPayload["metricValue"] = 4.6
	proofPayload["relatedCode"] = runCode
	status, body = perform(t, engine, http.MethodPost, "/api/proofs", tokens["operator"], "gate-proof-bad-create", proofPayload)
	if status != http.StatusCreated {
		t.Fatalf("create bad proof status = %d body=%s", status, body)
	}
	badProof := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	if status, _ = perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(badProof.ID)+"/transition", tokens["operator"], "gate-proof-bad-review",
		map[string]any{"status": "review", "expectedVersion": badProof.Version, "reason": "sending proof to review"}); status != http.StatusOK {
		t.Fatalf("proof -> review status = %d", status)
	}
	badProof.Version++
	if status, _ = perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(badProof.ID)+"/transition", tokens["operator"], "gate-proof-bad-accept-forbidden",
		map[string]any{"status": "accepted", "expectedVersion": badProof.Version, "reason": "operator must not accept", "expectedRunVersion": run.Version}); status != http.StatusForbidden {
		t.Fatalf("operator accept status = %d, want 403", status)
	}

	// Out-of-tolerance acceptance: proof accepted, batch parked in hold and the
	// judgment evidence is frozen on the batch revision chain.
	status, body = perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(badProof.ID)+"/transition", tokens["reviewer"], "gate-proof-bad-accept",
		map[string]any{"status": "accepted", "expectedVersion": badProof.Version, "reason": "reviewer accepts over-tolerance proof", "expectedRunVersion": run.Version})
	if status != http.StatusOK {
		t.Fatalf("accept bad proof status = %d body=%s", status, body)
	}
	status, body = perform(t, engine, http.MethodGet, "/api/runs/"+uintString(run.ID), tokens["reviewer"], "gate-run-held-read", nil)
	heldRun := decodeData[struct {
		Status    string `json:"status"`
		Version   uint   `json:"version"`
		Revisions []struct {
			Version      uint    `json:"version"`
			Status       string  `json:"status"`
			Tolerance    float64 `json:"tolerance"`
			ProofCode    string  `json:"proofCode"`
			ProofValue   float64 `json:"proofValue"`
			ProofVerdict string  `json:"proofVerdict"`
			Actor        string  `json:"actor"`
		} `json:"revisions"`
	}](t, body)
	if status != http.StatusOK || heldRun.Status != "hold" {
		t.Fatalf("run after bad proof: status=%d detail=%+v", status, heldRun)
	}
	judgement := heldRun.Revisions[0]
	if judgement.ProofVerdict != "out-of-tolerance" || judgement.ProofCode == "" || judgement.ProofValue != 4.6 ||
		judgement.Tolerance != 3.0 || judgement.Actor != "reviewer" {
		t.Fatalf("batch revision missing judgment evidence: %+v", judgement)
	}

	// A held batch blocks both creating a release decision and moving one out.
	blocked := recordPayload("RD-GATE-"+suffix, "被挡住的放行决定")
	blocked["relatedCode"] = runCode
	if status, _ := perform(t, engine, http.MethodPost, "/api/release", tokens["operator"], "gate-decision-create-blocked", blocked); status != http.StatusConflict {
		t.Fatalf("create decision on held run status = %d, want 409", status)
	}

	// Within-tolerance acceptance keeps the batch in proofing and allows a
	// release decision to be created against it. Use a fresh batch + proof.
	goodRunCode := "PR-GOOD-" + suffix
	goodPayload := recordPayload(goodRunCode, "合格校样批次")
	goodPayload["metricUnit"] = "ΔE"
	goodPayload["tolerance"] = 3.0
	status, body = perform(t, engine, http.MethodPost, "/api/runs", tokens["operator"], "gate-good-run-create", goodPayload)
	goodRun := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	for _, target := range []string{"printing", "proofing"} {
		if status, _ = perform(t, engine, http.MethodPost, "/api/runs/"+uintString(goodRun.ID)+"/transition", tokens["operator"], "gate-good-run-"+target,
			map[string]any{"status": target, "expectedVersion": goodRun.Version, "reason": "advancing good batch"}); status != http.StatusOK {
			t.Fatalf("good run -> %s status = %d", target, status)
		}
		goodRun.Version++
	}
	goodProofPayload := recordPayload("CP-GOOD-"+suffix, "合格校样")
	goodProofPayload["metricUnit"] = "ΔE"
	goodProofPayload["metricValue"] = 2.1
	goodProofPayload["relatedCode"] = goodRunCode
	status, body = perform(t, engine, http.MethodPost, "/api/proofs", tokens["operator"], "gate-good-proof-create", goodProofPayload)
	goodProof := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	if status, _ = perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(goodProof.ID)+"/transition", tokens["operator"], "gate-good-proof-review",
		map[string]any{"status": "review", "expectedVersion": goodProof.Version, "reason": "good proof to review"}); status != http.StatusOK {
		t.Fatalf("good proof -> review status = %d", status)
	}
	goodProof.Version++
	if status, body = perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(goodProof.ID)+"/transition", tokens["reviewer"], "gate-good-proof-accept",
		map[string]any{"status": "accepted", "expectedVersion": goodProof.Version, "reason": "within tolerance", "expectedRunVersion": goodRun.Version}); status != http.StatusOK {
		t.Fatalf("accept good proof status = %d body=%s", status, body)
	}
	status, body = perform(t, engine, http.MethodGet, "/api/runs/"+uintString(goodRun.ID), tokens["reviewer"], "gate-good-run-read", nil)
	goodRunDetail := decodeData[struct {
		Status    string `json:"status"`
		Revisions []struct {
			ProofVerdict string  `json:"proofVerdict"`
			ProofValue   float64 `json:"proofValue"`
		} `json:"revisions"`
	}](t, body)
	if status != http.StatusOK || goodRunDetail.Status != "proofing" || goodRunDetail.Revisions[0].ProofVerdict != "within-tolerance" {
		t.Fatalf("good run detail = %+v", goodRunDetail)
	}
	allowed := recordPayload("RD-GOOD-"+suffix, "合格批次放行决定")
	allowed["relatedCode"] = goodRunCode
	if status, _ := perform(t, engine, http.MethodPost, "/api/release", tokens["operator"], "gate-good-decision-create", allowed); status != http.StatusCreated {
		t.Fatalf("create decision on proofing run status = %d, want 201", status)
	}

	// A draft decision created while the batch was proofing must still be
	// blocked at release time once the batch is held by another proof.
	draftCode := "RD-DRAFT-" + suffix
	draftPayload := recordPayload(draftCode, "先行建立的草稿决定")
	draftPayload["relatedCode"] = goodRunCode
	status, body = perform(t, engine, http.MethodPost, "/api/release", tokens["operator"], "gate-draft-create", draftPayload)
	draft := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	if status != http.StatusCreated {
		t.Fatalf("create draft status = %d body=%s", status, body)
	}
	badProofTwo := recordPayload("CP-GOOD-BAD-"+suffix, "后来的超限校样")
	badProofTwo["metricUnit"] = "ΔE"
	badProofTwo["metricValue"] = 5.8
	badProofTwo["relatedCode"] = goodRunCode
	status, body = perform(t, engine, http.MethodPost, "/api/proofs", tokens["operator"], "gate-proof2-create", badProofTwo)
	proofTwo := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	goodRun.Version++ // acceptance bumped the good run once already
	if status, _ = perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(proofTwo.ID)+"/transition", tokens["operator"], "gate-proof2-review",
		map[string]any{"status": "review", "expectedVersion": proofTwo.Version, "reason": "second proof review"}); status != http.StatusOK {
		t.Fatalf("proof2 -> review status = %d", status)
	}
	proofTwo.Version++
	if status, _ = perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(proofTwo.ID)+"/transition", tokens["reviewer"], "gate-proof2-accept",
		map[string]any{"status": "accepted", "expectedVersion": proofTwo.Version, "reason": "now out of tolerance", "expectedRunVersion": goodRun.Version}); status != http.StatusOK {
		t.Fatalf("accept second bad proof status = %d", status)
	}
	if status, _ := perform(t, engine, http.MethodPost, "/api/release/"+uintString(draft.ID)+"/transition", tokens["reviewer"], "gate-draft-release-blocked",
		map[string]any{"status": "release", "expectedVersion": draft.Version, "reason": "must be blocked by held run"}); status != http.StatusConflict {
		t.Fatalf("release decision status = %d, want 409", status)
	}

	// Stale batch version during one review: the whole request fails and the
	// proof status must not land on its own.
	staleRunCode := "PR-STALE-" + suffix
	staleRunPayload := recordPayload(staleRunCode, "版本竞争批次")
	staleRunPayload["tolerance"] = 3.0
	status, body = perform(t, engine, http.MethodPost, "/api/runs", tokens["operator"], "gate-stale-run-create", staleRunPayload)
	staleRun := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	for _, target := range []string{"printing", "proofing"} {
		if status, _ = perform(t, engine, http.MethodPost, "/api/runs/"+uintString(staleRun.ID)+"/transition", tokens["operator"], "gate-stale-run-"+target,
			map[string]any{"status": target, "expectedVersion": staleRun.Version, "reason": "advancing stale batch"}); status != http.StatusOK {
			t.Fatalf("stale run -> %s status = %d", target, status)
		}
		staleRun.Version++
	}
	staleProofPayload := recordPayload("CP-STALE-"+suffix, "版本竞争校样")
	staleProofPayload["metricValue"] = 1.5
	staleProofPayload["relatedCode"] = staleRunCode
	status, body = perform(t, engine, http.MethodPost, "/api/proofs", tokens["operator"], "gate-stale-proof-create", staleProofPayload)
	staleProof := decodeData[struct {
		ID      uint `json:"id"`
		Version uint `json:"version"`
	}](t, body)
	if status, _ = perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(staleProof.ID)+"/transition", tokens["operator"], "gate-stale-proof-review",
		map[string]any{"status": "review", "expectedVersion": staleProof.Version, "reason": "stale proof review"}); status != http.StatusOK {
		t.Fatalf("stale proof -> review status = %d", status)
	}
	staleProof.Version++
	seenVersion := staleRun.Version

	// Someone else edits the batch (evidence/tolerance), bumping its version.
	updatePayload := recordPayload("ignored", "版本竞争批次")
	updatePayload["tolerance"] = 2.5
	updatePayload["expectedVersion"] = staleRun.Version
	if status, _ := perform(t, engine, http.MethodPut, "/api/runs/"+uintString(staleRun.ID), tokens["operator"], "gate-stale-run-update", updatePayload); status != http.StatusOK {
		t.Fatalf("run update status = %d", status)
	}
	staleRun.Version++

	if status, _ := perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(staleProof.ID)+"/transition", tokens["reviewer"], "gate-stale-accept-conflict",
		map[string]any{"status": "accepted", "expectedVersion": staleProof.Version, "reason": "reviewer saw old batch version", "expectedRunVersion": seenVersion}); status != http.StatusConflict {
		t.Fatalf("accept with stale run version status = %d, want 409", status)
	}
	status, body = perform(t, engine, http.MethodGet, "/api/proofs/"+uintString(staleProof.ID), tokens["reviewer"], "gate-stale-proof-read", nil)
	proofAfter := decodeData[struct {
		Status  string `json:"status"`
		Version uint   `json:"version"`
	}](t, body)
	if status != http.StatusOK || proofAfter.Status != "review" || proofAfter.Version != staleProof.Version {
		t.Fatalf("proof persisted despite batch version change: %+v", proofAfter)
	}

	// Retrying with the current batch version succeeds atomically.
	if status, _ = perform(t, engine, http.MethodPost, "/api/proofs/"+uintString(staleProof.ID)+"/transition", tokens["reviewer"], "gate-stale-accept-retry",
		map[string]any{"status": "accepted", "expectedVersion": staleProof.Version, "reason": "retry with current batch version", "expectedRunVersion": staleRun.Version}); status != http.StatusOK {
		t.Fatalf("accept retry status = %d", status)
	}
}
