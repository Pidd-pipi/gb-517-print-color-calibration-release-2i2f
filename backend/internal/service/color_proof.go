package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/constants"
	"github.com/blueship581/print-color-calibration-release/backend/internal/dto"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
	"github.com/blueship581/print-color-calibration-release/backend/internal/repository"
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
	if err := s.validateLinkedRun(ctx, input.PrintRunID); err != nil {
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
		PrintRunID:  input.PrintRunID,
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.ColorProof{}, fmt.Errorf("create 色彩校样: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "ColorProof", item.ID, "", item.Status, "created 色彩校样")
	return item, nil
}

func (s *colorProofService) validateLinkedRun(ctx context.Context, runID *uint) error {
	if runID == nil || *runID == 0 {
		return nil // linkage optional until review, where it becomes mandatory
	}
	if _, err := s.runs.Get(ctx, *runID); err != nil {
		return fmt.Errorf("%w: linked print run %d not found", ErrInvalidInput, *runID)
	}
	return nil
}

func (s *colorProofService) Update(ctx context.Context, id uint, input dto.UpdateColorProof, actor, requestID string) (model.ColorProof, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ColorProof{}, err
	}
	if err := validateColorProofBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ColorProof{}, err
	}
	if err := s.validateLinkedRun(ctx, input.PrintRunID); err != nil {
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
	current.PrintRunID = input.PrintRunID
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
	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()

	if target != "accepted" {
		// Non-acceptance transitions (submit/reject/re-review) touch only the
		// proof and never move the batch gate.
		if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
			return model.ColorProof{}, fmt.Errorf("transition 色彩校样: %w", err)
		}
		if err := s.security.Audit(ctx, actor, requestID, "transition", "ColorProof", id, before, target, input.Reason); err != nil {
			return model.ColorProof{}, fmt.Errorf("persist transition audit: %w", err)
		}
		return s.repository.Get(ctx, id)
	}

	// Acceptance drives the linked batch. Everything below shares one
	// transaction and one optimistic-lock view: if the batch version changed
	// during this review, UpdateVersioned returns ErrVersionConflict, the
	// transaction rolls back and the proof status is never stored on its own.
	if current.PrintRunID == nil || *current.PrintRunID == 0 {
		return model.ColorProof{}, ErrProofNotLinked
	}
	err = repository.WithTransaction(ctx, s.repository.DB(), func(txCtx context.Context) error {
		run, err := s.runs.Get(txCtx, *current.PrintRunID)
		if err != nil {
			return err
		}
		if run.Status != string(constants.RunStateProofing) && run.Status != string(constants.RunStateHold) {
			return ErrRunNotGateable
		}
		if err := validateColorTolerance(run.ColorTolerance); err != nil {
			return err
		}
		if input.ExpectedRunVersion == 0 || run.Version != input.ExpectedRunVersion {
			return repository.ErrVersionConflict
		}

		measured := current.MetricValue
		verdict := model.ProofVerdictPass
		runTarget := string(constants.RunStateProofing)
		if measured > run.ColorTolerance {
			verdict = model.ProofVerdictFail
			runTarget = string(constants.RunStateHold)
		}
		runBefore := run.Status
		run.Status = runTarget
		run.Version = input.ExpectedRunVersion + 1
		run.UpdatedAt = time.Now().UTC()
		run.ProofVerdict = verdict
		run.ProofMeasuredDeltaE = measured
		run.ProofTolerance = run.ColorTolerance
		run.ProofActor = actor
		run.ProofID = &current.ID
		run.ProofDecidedAt = time.Now().UTC()

		// Persist the batch (with its immutable revision) first so the
		// optimistic lock protects the whole review before the proof moves.
		runReason := fmt.Sprintf("proof %s: measured ΔE %.3f vs tolerance %.3f (%s)", verdict, measured, run.ColorTolerance, input.Reason)
		if err := s.runs.UpdateVersioned(txCtx, run.ID, input.ExpectedRunVersion, &run, actor, requestID, runReason); err != nil {
			return err
		}
		if err := s.repository.Update(txCtx, id, input.ExpectedVersion, &current); err != nil {
			return err
		}
		if err := s.security.Audit(txCtx, actor, requestID, "transition", "ColorProof", id, before, target, input.Reason); err != nil {
			return err
		}
		if runBefore != runTarget {
			if err := s.security.Audit(txCtx, actor, requestID, "transition", "PrintRun", run.ID, runBefore, runTarget, runReason); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
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
