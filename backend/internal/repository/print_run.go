package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/constants"
	"github.com/blueship581/print-color-calibration-release/backend/internal/dto"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
	"gorm.io/gorm"
)

// PrintRunRepository owns all persistence operations for 印刷批次.
type PrintRunRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.PrintRun], error)
	Get(context.Context, uint) (model.PrintRun, error)
	GetByCode(context.Context, string) (model.PrintRun, error)
	CreateVersioned(context.Context, *model.PrintRun, string, string, string) error
	UpdateVersioned(context.Context, uint, uint, *model.PrintRun, string, string, string) error
	// AcceptProof commits the proof status and the linked run verdict in one
	// transaction. Both rows use optimistic-lock predicates, so a proof version
	// or run version that changed during the review rolls the whole operation
	// back and the proof status can never persist by itself.
	AcceptProof(context.Context, ProofAcceptance) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
}

// ProofAcceptance carries the coordinated 校样接收: the already-decided proof
// status, the already-decided run status/verdict and the reviewer evidence.
type ProofAcceptance struct {
	Run                  *model.PrintRun
	ExpectedRunVersion   uint
	Proof                *model.ColorProof
	ExpectedProofVersion uint
	Reviewer             string
	RequestID            string
	Reason               string
	Verdict              string
}

type printRunRepository struct {
	store *Store[model.PrintRun]
}

func NewPrintRunRepository(db *gorm.DB) PrintRunRepository {
	return &printRunRepository{store: NewStore[model.PrintRun](db)}
}

func (r *printRunRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.PrintRun], error) {
	return r.store.List(ctx, q)
}
func (r *printRunRepository) Get(ctx context.Context, id uint) (model.PrintRun, error) {
	var item model.PrintRun
	err := r.store.db.WithContext(ctx).
		Preload("Revisions", func(db *gorm.DB) *gorm.DB { return db.Order("version DESC") }).
		First(&item, id).Error
	return item, err
}
func (r *printRunRepository) GetByCode(ctx context.Context, code string) (model.PrintRun, error) {
	var item model.PrintRun
	err := r.store.db.WithContext(ctx).Where("code = ?", code).First(&item).Error
	return item, err
}

func (r *printRunRepository) AcceptProof(ctx context.Context, in ProofAcceptance) error {
	return r.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()

		proofResult := tx.Model(&model.ColorProof{}).
			Where("id = ? AND version = ?", in.Proof.ID, in.ExpectedProofVersion).
			Updates(map[string]any{"status": in.Proof.Status, "version": in.Proof.Version, "updated_at": now})
		if proofResult.Error != nil {
			return proofResult.Error
		}
		if proofResult.RowsAffected == 0 {
			return ErrVersionConflict
		}

		runResult := tx.Model(&model.PrintRun{}).
			Where("id = ? AND version = ?", in.Run.ID, in.ExpectedRunVersion).
			Updates(map[string]any{"status": in.Run.Status, "version": in.Run.Version, "updated_at": now})
		if runResult.Error != nil {
			return runResult.Error
		}
		if runResult.RowsAffected == 0 {
			return ErrVersionConflict
		}

		revision := printRunRevision(in.Run, in.Reviewer, in.RequestID, in.Reason)
		revision.ProofCode = in.Proof.Code
		revision.ProofValue = in.Proof.MetricValue
		revision.ProofVerdict = in.Verdict
		if err := tx.Create(revision).Error; err != nil {
			return err
		}

		if err := tx.Create(&model.AuditLog{
			Actor: in.Reviewer, RequestID: in.RequestID, Action: "transition", EntityType: "ColorProof",
			EntityID: in.Proof.ID, BeforeState: "review", AfterState: in.Proof.Status,
			Detail: in.Reason, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&model.AuditLog{
			Actor: in.Reviewer, RequestID: in.RequestID, Action: "proof-judgement", EntityType: "PrintRun",
			EntityID: in.Run.ID, BeforeState: string(constants.RunStateProofing), AfterState: in.Run.Status,
			Detail:    fmt.Sprintf("proof %s measured %.4f vs tolerance %.4f: %s", in.Proof.Code, in.Proof.MetricValue, in.Run.Tolerance, in.Verdict),
			CreatedAt: now,
		}).Error
	})
}
func (r *printRunRepository) CreateVersioned(ctx context.Context, item *model.PrintRun, actor, requestID, reason string) error {
	return r.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Revisions").Create(item).Error; err != nil {
			return err
		}
		return tx.Create(printRunRevision(item, actor, requestID, reason)).Error
	})
}
func (r *printRunRepository) UpdateVersioned(ctx context.Context, id, version uint, item *model.PrintRun, actor, requestID, reason string) error {
	return r.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.PrintRun{}).Where("id = ? AND version = ?", id, version).
			Select("*").Omit("id", "code", "created_at", "deleted_at", "Revisions").Updates(item)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		return tx.Create(printRunRevision(item, actor, requestID, reason)).Error
	})
}

func printRunRevision(item *model.PrintRun, actor, requestID, reason string) *model.PrintRunRevision {
	return &model.PrintRunRevision{
		PrintRunID: item.ID, Version: item.Version, Status: item.Status, Name: item.Name,
		Facility: item.Facility, Owner: item.Owner, Category: item.Category,
		RiskLevel: item.RiskLevel, MetricValue: item.MetricValue, MetricUnit: item.MetricUnit,
		Tolerance: item.Tolerance,
		Evidence:  item.Evidence, RelatedCode: item.RelatedCode,
		Actor: actor, RequestID: requestID, Reason: reason,
	}
}
func (r *printRunRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *printRunRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}
