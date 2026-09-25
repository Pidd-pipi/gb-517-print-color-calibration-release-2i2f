package repository

import (
	"context"

	"github.com/blueship581/print-color-calibration-release/backend/internal/dto"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
	"gorm.io/gorm"
)

// PressUnitRepository owns all persistence operations for 印刷设备.
type PressUnitRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.PressUnit], error)
	Get(context.Context, uint) (model.PressUnit, error)
	Create(context.Context, *model.PressUnit) error
	Update(context.Context, uint, uint, *model.PressUnit) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
}

type pressUnitRepository struct {
	store *Store[model.PressUnit]
}

func NewPressUnitRepository(db *gorm.DB) PressUnitRepository {
	return &pressUnitRepository{store: NewStore[model.PressUnit](db)}
}

func (r *pressUnitRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.PressUnit], error) {
	return r.store.List(ctx, q)
}
func (r *pressUnitRepository) Get(ctx context.Context, id uint) (model.PressUnit, error) {
	return r.store.Get(ctx, id)
}
func (r *pressUnitRepository) Create(ctx context.Context, item *model.PressUnit) error {
	return r.store.Create(ctx, item)
}
func (r *pressUnitRepository) Update(ctx context.Context, id, version uint, item *model.PressUnit) error {
	return r.store.Update(ctx, id, version, item)
}
func (r *pressUnitRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *pressUnitRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}
