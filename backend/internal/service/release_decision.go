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

type ReleaseDecisionService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.ReleaseDecision], error)
	Get(context.Context, uint) (model.ReleaseDecision, error)
	Create(context.Context, dto.CreateReleaseDecision, string, string) (model.ReleaseDecision, error)
	Update(context.Context, uint, dto.UpdateReleaseDecision, string, string) (model.ReleaseDecision, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string, string) (model.ReleaseDecision, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
}

type releaseDecisionService struct {
	repository repository.ReleaseDecisionRepository
	security   SecurityService
}

func NewReleaseDecisionService(repo repository.ReleaseDecisionRepository, security SecurityService) ReleaseDecisionService {
	return &releaseDecisionService{repository: repo, security: security}
}

func (s *releaseDecisionService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.ReleaseDecision], error) {
	return s.repository.List(ctx, query)
}

func (s *releaseDecisionService) Get(ctx context.Context, id uint) (model.ReleaseDecision, error) {
	return s.repository.Get(ctx, id)
}

func (s *releaseDecisionService) Create(ctx context.Context, input dto.CreateReleaseDecision, actor, requestID string) (model.ReleaseDecision, error) {
	if err := validateReleaseDecisionBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ReleaseDecision{}, err
	}
	item := model.ReleaseDecision{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.ReleaseDecisionInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.CreateVersioned(ctx, &item, actor, requestID, "created release decision"); err != nil {
		return model.ReleaseDecision{}, fmt.Errorf("create 放行决定: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "ReleaseDecision", item.ID, "", item.Status, "created 放行决定")
	return item, nil
}

func (s *releaseDecisionService) Update(ctx context.Context, id uint, input dto.UpdateReleaseDecision, actor, requestID string) (model.ReleaseDecision, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ReleaseDecision{}, err
	}
	if current.Status == string(constants.DecisionTypeRelease) || current.Status == string(constants.DecisionTypeQuarantine) {
		return model.ReleaseDecision{}, ErrLocked
	}
	if err := validateReleaseDecisionBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ReleaseDecision{}, err
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
	if err := s.repository.UpdateVersioned(ctx, id, input.ExpectedVersion, &current, actor, requestID, "updated decision evidence"); err != nil {
		return model.ReleaseDecision{}, fmt.Errorf("update 放行决定: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "ReleaseDecision", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *releaseDecisionService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, role, requestID string) (model.ReleaseDecision, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ReleaseDecision{}, err
	}
	target := strings.TrimSpace(input.Status)
	if (target == string(constants.DecisionTypeRelease) || target == string(constants.DecisionTypeQuarantine) ||
		current.Status == string(constants.DecisionTypeRelease) || current.Status == string(constants.DecisionTypeQuarantine)) && !canReview(role) {
		return model.ReleaseDecision{}, ErrForbidden
	}
	if !constants.CanTransition(constants.ReleaseDecisionTransitions, current.Status, target) {
		return model.ReleaseDecision{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateVersioned(ctx, id, input.ExpectedVersion, &current, actor, requestID, input.Reason); err != nil {
		return model.ReleaseDecision{}, fmt.Errorf("transition 放行决定: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "ReleaseDecision", id, before, target, input.Reason); err != nil {
		return model.ReleaseDecision{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, id)
}

func (s *releaseDecisionService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.Status != model.ReleaseDecisionInitialStatus {
		return ErrLocked
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "ReleaseDecision", id, current.Status, "deleted", "soft deleted 放行决定")
}

func (s *releaseDecisionService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateReleaseDecisionBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
