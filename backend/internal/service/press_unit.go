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

type PressUnitService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.PressUnit], error)
	Get(context.Context, uint) (model.PressUnit, error)
	Create(context.Context, dto.CreatePressUnit, string, string) (model.PressUnit, error)
	Update(context.Context, uint, dto.UpdatePressUnit, string, string) (model.PressUnit, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.PressUnit, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
}

type pressUnitService struct {
	repository repository.PressUnitRepository
	security   SecurityService
}

func NewPressUnitService(repo repository.PressUnitRepository, security SecurityService) PressUnitService {
	return &pressUnitService{repository: repo, security: security}
}

func (s *pressUnitService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.PressUnit], error) {
	return s.repository.List(ctx, query)
}

func (s *pressUnitService) Get(ctx context.Context, id uint) (model.PressUnit, error) {
	return s.repository.Get(ctx, id)
}

func (s *pressUnitService) Create(ctx context.Context, input dto.CreatePressUnit, actor, requestID string) (model.PressUnit, error) {
	if err := validatePressUnitBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.PressUnit{}, err
	}
	item := model.PressUnit{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.PressUnitInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.PressUnit{}, fmt.Errorf("create 印刷设备: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "PressUnit", item.ID, "", item.Status, "created 印刷设备")
	return item, nil
}

func (s *pressUnitService) Update(ctx context.Context, id uint, input dto.UpdatePressUnit, actor, requestID string) (model.PressUnit, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.PressUnit{}, err
	}
	if err := validatePressUnitBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.PressUnit{}, err
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
		return model.PressUnit{}, fmt.Errorf("update 印刷设备: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "PressUnit", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *pressUnitService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (model.PressUnit, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.PressUnit{}, err
	}
	target := strings.TrimSpace(input.Status)
	if !constants.CanTransition(constants.PressUnitTransitions, current.Status, target) {
		return model.PressUnit{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.PressUnit{}, fmt.Errorf("transition 印刷设备: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "PressUnit", id, before, target, input.Reason); err != nil {
		return model.PressUnit{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, id)
}

func (s *pressUnitService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "PressUnit", id, current.Status, "deleted", "soft deleted 印刷设备")
}

func (s *pressUnitService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validatePressUnitBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
