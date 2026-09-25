package repository

import (
	"context"

	"github.com/blueship581/print-color-calibration-release/backend/internal/dto"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
	"gorm.io/gorm"
)

// ReleaseDecisionRepository owns all persistence operations for 放行决定.
type ReleaseDecisionRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.ReleaseDecision], error)
	Get(context.Context, uint) (model.ReleaseDecision, error)
	CreateVersioned(context.Context, *model.ReleaseDecision, string, string, string) error
	UpdateVersioned(context.Context, uint, uint, *model.ReleaseDecision, string, string, string) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
}

type releaseDecisionRepository struct {
	store *Store[model.ReleaseDecision]
}

func NewReleaseDecisionRepository(db *gorm.DB) ReleaseDecisionRepository {
	return &releaseDecisionRepository{store: NewStore[model.ReleaseDecision](db)}
}

func (r *releaseDecisionRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.ReleaseDecision], error) {
	return r.store.List(ctx, q)
}
func (r *releaseDecisionRepository) Get(ctx context.Context, id uint) (model.ReleaseDecision, error) {
	var item model.ReleaseDecision
	err := r.store.db.WithContext(ctx).
		Preload("Revisions", func(db *gorm.DB) *gorm.DB { return db.Order("version DESC") }).
		First(&item, id).Error
	return item, err
}
func (r *releaseDecisionRepository) CreateVersioned(ctx context.Context, item *model.ReleaseDecision, actor, requestID, reason string) error {
	return r.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Revisions").Create(item).Error; err != nil {
			return err
		}
		return tx.Create(releaseDecisionRevision(item, actor, requestID, reason)).Error
	})
}
func (r *releaseDecisionRepository) UpdateVersioned(ctx context.Context, id, version uint, item *model.ReleaseDecision, actor, requestID, reason string) error {
	return r.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ReleaseDecision{}).Where("id = ? AND version = ?", id, version).
			Select("*").Omit("id", "code", "created_at", "deleted_at", "Revisions").Updates(item)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		return tx.Create(releaseDecisionRevision(item, actor, requestID, reason)).Error
	})
}

func releaseDecisionRevision(item *model.ReleaseDecision, actor, requestID, reason string) *model.ReleaseDecisionRevision {
	return &model.ReleaseDecisionRevision{
		ReleaseDecisionID: item.ID, Version: item.Version, Status: item.Status, Name: item.Name,
		RiskLevel: item.RiskLevel, MetricValue: item.MetricValue, MetricUnit: item.MetricUnit,
		Evidence: item.Evidence, RelatedCode: item.RelatedCode,
		Actor: actor, RequestID: requestID, Reason: reason,
	}
}
func (r *releaseDecisionRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *releaseDecisionRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}
