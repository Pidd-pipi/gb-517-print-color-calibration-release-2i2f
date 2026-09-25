package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/constants"
	"github.com/blueship581/print-color-calibration-release/backend/internal/dto"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
	"github.com/blueship581/print-color-calibration-release/backend/internal/repository"
	"gorm.io/gorm"
)

type ColorProofService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.ColorProof], error)
	Get(context.Context, uint) (model.ColorProof, error)
	Create(context.Context, dto.CreateColorProof, string, string) (model.ColorProof, error)
	Update(context.Context, uint, dto.UpdateColorProof, string, string) (model.ColorProof, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string, string) (model.ColorProof, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
}

// Proof judgement verdicts frozen into PrintRunRevision.ProofVerdict.
const (
	ProofVerdictWithinTolerance = "within-tolerance"
	ProofVerdictOutOfTolerance  = "out-of-tolerance"
)

type colorProofService struct {
	repository repository.ColorProofRepository
	runs       repository.PrintRunRepository
	security   SecurityService
}

func NewColorProofService(repo repository.ColorProofRepository, runs repository.PrintRunRepository, security SecurityService) ColorProofService {
	return &colorProofService{repository: repo, runs: runs, security: security}
}

func (s *colorProofService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.ColorProof], error) {
	return s.repository.List(ctx, query)
}

func (s *colorProofService) Get(ctx context.Context, id uint) (model.ColorProof, error) {
	return s.repository.Get(ctx, id)
}

func (s *colorProofService) Create(ctx context.Context, input dto.CreateColorProof, actor, requestID string) (model.ColorProof, error) {
	if err := validateColorProofBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ColorProof{}, err
	}
	item := model.ColorProof{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.ColorProofInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.ColorProof{}, fmt.Errorf("create 色彩校样: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "ColorProof", item.ID, "", item.Status, "created 色彩校样")
	return item, nil
}

func (s *colorProofService) Update(ctx context.Context, id uint, input dto.UpdateColorProof, actor, requestID string) (model.ColorProof, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ColorProof{}, err
	}
	if err := validateColorProofBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ColorProof{}, err
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = strings.TrimSpace(input.Description)
	current.Facility = strings.TrimSpace(input.Facility)
	current.Owner = strings.TrimSpace(input.Owner)
	current.Category = strings.TrimSpace(input.Category)
	current.RiskLevel = input.RiskLevel
	current.MetricValue = input.MetricValue
	current.MetricUnit = strings.TrimSpace(input.MetricUnit)
	current.EffectiveAt = input.EffectiveAt.UTC()
	current.Evidence = strings.TrimSpace(input.Evidence)
	current.RelatedCode = strings.ToUpper(strings.TrimSpace(input.RelatedCode))
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ColorProof{}, fmt.Errorf("update 色彩校样: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "ColorProof", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *colorProofService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, role, requestID string) (model.ColorProof, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ColorProof{}, err
	}
	target := strings.TrimSpace(input.Status)
	if (target == "accepted" || target == "rejected" || current.Status == "accepted" || current.Status == "rejected") && !canReview(role) {
		return model.ColorProof{}, ErrForbidden
	}
	if !constants.CanTransition(constants.ColorProofTransitions, current.Status, target) {
		return model.ColorProof{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	if target != "accepted" {
		before := current.Status
		current.Status = target
		current.Version = input.ExpectedVersion + 1
		current.UpdatedAt = time.Now().UTC()
		if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
			return model.ColorProof{}, fmt.Errorf("transition 色彩校样: %w", err)
		}
		if err := s.security.Audit(ctx, actor, requestID, "transition", "ColorProof", id, before, target, input.Reason); err != nil {
			return model.ColorProof{}, fmt.Errorf("persist transition audit: %w", err)
		}
		return s.repository.Get(ctx, id)
	}

	// Acceptance is the quality gate: read the linked batch at the version the
	// reviewer saw, compare the proof measurement against the batch tolerance
	// and commit proof status, batch state and evidence in one transaction.
	runCode := strings.ToUpper(strings.TrimSpace(current.RelatedCode))
	if runCode == "" {
		return model.ColorProof{}, ErrBatchNotLinked
	}
	run, err := s.runs.GetByCode(ctx, runCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ColorProof{}, fmt.Errorf("%w: %s", ErrBatchNotLinked, runCode)
		}
		return model.ColorProof{}, err
	}
	if run.Status != string(constants.RunStateProofing) {
		return model.ColorProof{}, fmt.Errorf("%w: %s 当前 %s", ErrBatchNotProofing, run.Code, run.Status)
	}
	if run.Tolerance <= 0 {
		return model.ColorProof{}, fmt.Errorf("%w: %s", ErrToleranceMissing, run.Code)
	}
	expectedRunVersion := input.ExpectedRunVersion
	if expectedRunVersion == 0 {
		expectedRunVersion = run.Version
	}
	if expectedRunVersion != run.Version {
		return model.ColorProof{}, fmt.Errorf("%w: 批次 %s 已变更至 v%d", repository.ErrVersionConflict, run.Code, run.Version)
	}

	deviation := current.MetricValue
	within := deviation <= run.Tolerance
	verdict := ProofVerdictOutOfTolerance
	if within {
		verdict = ProofVerdictWithinTolerance
	} else {
		run.Status = string(constants.RunStateHold)
	}
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	run.Version = expectedRunVersion + 1
	now := time.Now().UTC()
	current.UpdatedAt = now
	run.UpdatedAt = now

	reason := fmt.Sprintf("校样 %s 实测色差 %.4f%s，同批次容差 %.4f%s：%s（%s）",
		current.Code, deviation, current.MetricUnit, run.Tolerance, run.MetricUnit, verdict, strings.TrimSpace(input.Reason))
	reason = truncateRunes(reason, 500)
	if err := s.runs.AcceptProof(ctx, repository.ProofAcceptance{
		Run: &run, ExpectedRunVersion: expectedRunVersion,
		Proof: &current, ExpectedProofVersion: input.ExpectedVersion,
		Reviewer: actor, RequestID: requestID, Reason: reason, Verdict: verdict,
	}); err != nil {
		return model.ColorProof{}, fmt.Errorf("accept 色彩校样: %w", err)
	}
	return s.repository.Get(ctx, id)
}

func (s *colorProofService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "ColorProof", id, current.Status, "deleted", "soft deleted 色彩校样")
}

func (s *colorProofService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateColorProofBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
