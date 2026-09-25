package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/blueship581/print-color-calibration-release/backend/internal/config"
	"github.com/blueship581/print-color-calibration-release/backend/internal/constants"
	"github.com/blueship581/print-color-calibration-release/backend/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(ctx context.Context, cfg config.Config, log *slog.Logger) (*gorm.DB, *redis.Client, error) {
	var dialector gorm.Dialector
	switch cfg.DatabaseDriver {
	case "postgres":
		dialector = postgres.Open(cfg.DatabaseDSN)
	case "mysql":
		dialector = mysql.Open(cfg.DatabaseDSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DatabaseDSN)
	default:
		return nil, nil, fmt.Errorf("unsupported database driver %q", cfg.DatabaseDriver)
	}
	logLevel := logger.Warn
	if cfg.Environment == "development" {
		logLevel = logger.Info
	}
	var db *gorm.DB
	var err error
	for attempt := 1; attempt <= 20; attempt++ {
		db, err = gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logLevel)})
		if err == nil {
			sqlDB, dbErr := db.DB()
			if dbErr == nil && sqlDB.PingContext(ctx) == nil {
				break
			}
			if dbErr != nil {
				err = dbErr
			} else {
				err = sqlDB.PingContext(ctx)
			}
		}
		log.Warn("database not ready", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("connect database: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, nil, err
	}
	if err := Seed(ctx, db); err != nil {
		return nil, nil, err
	}
	var redisClient *redis.Client
	if cfg.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return nil, nil, fmt.Errorf("connect redis: %w", err)
		}
	}
	return db, redisClient, nil
}

func migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{}, &model.AuditLog{},
		&model.PressUnit{},
		&model.PrintRun{}, &model.PrintRunRevision{},
		&model.ColorProof{},
		&model.ReleaseDecision{}, &model.ReleaseDecisionRevision{},
	)
}

func Seed(ctx context.Context, db *gorm.DB) error {
	var users int64
	if err := db.WithContext(ctx).Model(&model.User{}).Count(&users).Error; err != nil {
		return err
	}
	if users == 0 {
		password, err := bcrypt.GenerateFromPassword([]byte("Admin123!"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		seedUsers := []model.User{
			{Username: "admin", DisplayName: "系统管理员", PasswordHash: string(password), Role: model.RoleAdmin, Active: true},
			{Username: "reviewer", DisplayName: "质量复核员", PasswordHash: string(password), Role: model.RoleReviewer, Active: true},
			{Username: "operator", DisplayName: "现场操作员", PasswordHash: string(password), Role: model.RoleOperator, Active: true},
			{Username: "viewer", DisplayName: "只读观察员", PasswordHash: string(password), Role: model.RoleViewer, Active: true},
		}
		if err := db.WithContext(ctx).Create(&seedUsers).Error; err != nil {
			return err
		}
	}

	if err := seedPressUnit(ctx, db); err != nil {
		return err
	}

	if err := seedPrintRun(ctx, db); err != nil {
		return err
	}

	if err := seedColorProof(ctx, db); err != nil {
		return err
	}

	if err := seedReleaseDecision(ctx, db); err != nil {
		return err
	}

	return nil
}

func seedPressUnit(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.PressUnit{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	items := []model.PressUnit{

		{BaseModel: model.BaseModel{Code: "PU-001", Name: "印刷设备示例一", Status: "ready", Version: 1,
			Description: "用于启动验证和主要流程演示的印刷设备记录"}, Facility: "印刷色彩批次校准放行区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 12.5, MetricUnit: "unit",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-517-01"},

		{BaseModel: model.BaseModel{Code: "PU-002", Name: "印刷设备示例二", Status: "setup", Version: 1,
			Description: "用于启动验证和主要流程演示的印刷设备记录"}, Facility: "印刷色彩批次校准放行区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 25.0, MetricUnit: "%",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-517-02"},

		{BaseModel: model.BaseModel{Code: "PU-003", Name: "印刷设备示例三", Status: "printing", Version: 1,
			Description: "用于启动验证和主要流程演示的印刷设备记录"}, Facility: "印刷色彩批次校准放行区域3", Owner: "安全主管组",
			Category: "复核", RiskLevel: "high", MetricValue: 37.5, MetricUnit: "score",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-517-03"},
	}
	return db.WithContext(ctx).Create(&items).Error
}

func seedPrintRun(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.PrintRun{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	proofPassActor := "reviewer"
	items := []model.PrintRun{

		{BaseModel: model.BaseModel{Code: "PR-001", Name: "印刷批次示例一", Status: "setup", Version: 1,
			Description: "用于启动验证和主要流程演示的印刷批次记录"}, Facility: "印刷色彩批次校准放行区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 12.5, MetricUnit: "unit",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-517-01",
			ColorTolerance: 3.0},

		{BaseModel: model.BaseModel{Code: "PR-002", Name: "印刷批次示例二", Status: "printing", Version: 1,
			Description: "用于启动验证和主要流程演示的印刷批次记录"}, Facility: "印刷色彩批次校准放行区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 25.0, MetricUnit: "%",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-517-02",
			ColorTolerance: 3.0},

		{BaseModel: model.BaseModel{Code: "PR-003", Name: "印刷批次示例三", Status: "proofing", Version: 1,
			Description: "校样已合格、等待建立放行决定的批次"}, Facility: "印刷色彩批次校准放行区域3", Owner: "安全主管组",
			Category: "复核", RiskLevel: "high", MetricValue: 37.5, MetricUnit: "score",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "校样 ΔE 在容差内", RelatedCode: "REL-517-03",
			ColorTolerance: 3.0, ProofVerdict: model.ProofVerdictPass, ProofMeasuredDeltaE: 1.8,
			ProofTolerance: 3.0, ProofActor: proofPassActor, ProofDecidedAt: now.Add(6 * time.Hour)},

		{BaseModel: model.BaseModel{Code: "PR-004", Name: "印刷批次示例四", Status: "hold", Version: 2,
			Description: "校样色差超容差、已被转入等待的批次"}, Facility: "印刷色彩批次校准放行区域4", Owner: "质量复核组",
			Category: "复核", RiskLevel: "critical", MetricValue: 42.0, MetricUnit: "score",
			EffectiveAt: now.Add(9 * time.Hour), Evidence: "校样 ΔE 超容差，等待返修后重新校样", RelatedCode: "REL-517-04",
			ColorTolerance: 3.0, ProofVerdict: model.ProofVerdictFail, ProofMeasuredDeltaE: 4.6,
			ProofTolerance: 3.0, ProofActor: proofPassActor, ProofDecidedAt: now.Add(9 * time.Hour)},

		{BaseModel: model.BaseModel{Code: "PR-005", Name: "印刷批次示例五", Status: "released", Version: 2,
			Description: "校样合格并完成放行的批次"}, Facility: "印刷色彩批次校准放行区域5", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 18.0, MetricUnit: "unit",
			EffectiveAt: now.Add(12 * time.Hour), Evidence: "校样合格，批次已放行", RelatedCode: "REL-517-05",
			ColorTolerance: 3.0, ProofVerdict: model.ProofVerdictPass, ProofMeasuredDeltaE: 2.0,
			ProofTolerance: 3.0, ProofActor: proofPassActor, ProofDecidedAt: now.Add(12 * time.Hour)},
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Revisions").Create(&items).Error; err != nil {
			return err
		}
		revisions := make([]model.PrintRunRevision, 0, len(items))
		for _, item := range items {
			revision := model.PrintRunRevision{
				PrintRunID: item.ID, Version: item.Version, Status: item.Status, Name: item.Name,
				Facility: item.Facility, Owner: item.Owner, Category: item.Category,
				RiskLevel: item.RiskLevel, MetricValue: item.MetricValue, MetricUnit: item.MetricUnit,
				Evidence: item.Evidence, RelatedCode: item.RelatedCode,
				ColorTolerance: item.ColorTolerance, ProofVerdict: item.ProofVerdict,
				ProofMeasuredDeltaE: item.ProofMeasuredDeltaE, ProofTolerance: item.ProofTolerance,
				ProofActor: item.ProofActor, ProofDecidedAt: item.ProofDecidedAt,
				Actor: "seed", RequestID: "startup-seed", Reason: "initial colour configuration",
			}
			revisions = append(revisions, revision)
		}
		// Gate verdicts for the demo runs are historical: add the v1 snapshot
		// for the runs that now sit at v2 so the revision chain stays complete.
		for idx := range items {
			item := items[idx]
			if item.Version <= 1 {
				continue
			}
			v1Status := string(constants.RunStateProofing)
			reason := "proof pass kept batch in proofing"
			if item.ProofVerdict == model.ProofVerdictFail {
				reason = "proof fail parked batch in hold"
			}
			revisions = append(revisions, model.PrintRunRevision{
				PrintRunID: item.ID, Version: 1, Status: v1Status, Name: item.Name,
				Facility: item.Facility, Owner: item.Owner, Category: item.Category,
				RiskLevel: item.RiskLevel, MetricValue: item.MetricValue, MetricUnit: item.MetricUnit,
				Evidence: item.Evidence, RelatedCode: item.RelatedCode, ColorTolerance: item.ColorTolerance,
				Actor: "seed", RequestID: "startup-seed", Reason: reason,
			})
		}
		return tx.Create(&revisions).Error
	})
}

func seedColorProof(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.ColorProof{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	var runs []model.PrintRun
	if err := db.WithContext(ctx).Order("code ASC").Find(&runs).Error; err != nil {
		return err
	}
	runID := func(code string) *uint {
		for i := range runs {
			if runs[i].Code == code {
				id := runs[i].ID
				return &id
			}
		}
		return nil
	}
	items := []model.ColorProof{

		{BaseModel: model.BaseModel{Code: "CP-001", Name: "色彩校样示例一", Status: "captured", Version: 1,
			Description: "已采集、待提交复核的校样"}, Facility: "印刷色彩批次校准放行区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 1.4, MetricUnit: "ΔE",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "分光密度仪首件读数", RelatedCode: "REL-517-01",
			PrintRunID: runID("PR-001")},

		{BaseModel: model.BaseModel{Code: "CP-002", Name: "色彩校样示例二", Status: "review", Version: 1,
			Description: "已提交、等待复核员接收的校样"}, Facility: "印刷色彩批次校准放行区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 2.3, MetricUnit: "ΔE",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "印刷中抽样读数", RelatedCode: "REL-517-02",
			PrintRunID: runID("PR-002")},

		{BaseModel: model.BaseModel{Code: "CP-003", Name: "色彩校样示例三", Status: "accepted", Version: 1,
			Description: "色差合格、批次保持校样中并允许放行"}, Facility: "印刷色彩批次校准放行区域3", Owner: "安全主管组",
			Category: "复核", RiskLevel: "high", MetricValue: 1.8, MetricUnit: "ΔE",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "ΔE 1.8 在容差 3.0 内", RelatedCode: "REL-517-03",
			PrintRunID: runID("PR-003")},

		{BaseModel: model.BaseModel{Code: "CP-004", Name: "色彩校样示例四", Status: "accepted", Version: 1,
			Description: "色差超容差、批次已转入等待"}, Facility: "印刷色彩批次校准放行区域4", Owner: "质量复核组",
			Category: "复核", RiskLevel: "critical", MetricValue: 4.6, MetricUnit: "ΔE",
			EffectiveAt: now.Add(9 * time.Hour), Evidence: "ΔE 4.6 超出容差 3.0", RelatedCode: "REL-517-04",
			PrintRunID: runID("PR-004")},

		{BaseModel: model.BaseModel{Code: "CP-005", Name: "色彩校样示例五", Status: "accepted", Version: 1,
			Description: "色差合格、支撑批次放行"}, Facility: "印刷色彩批次校准放行区域5", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 2.0, MetricUnit: "ΔE",
			EffectiveAt: now.Add(12 * time.Hour), Evidence: "ΔE 2.0 在容差 3.0 内", RelatedCode: "REL-517-05",
			PrintRunID: runID("PR-005")},
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		// Backfill the gate evidence's proof pointer on the affected batches.
		proofByRun := map[uint]uint{}
		for _, p := range items {
			if p.PrintRunID != nil {
				proofByRun[*p.PrintRunID] = p.ID
			}
		}
		for i := range runs {
			pid, ok := proofByRun[runs[i].ID]
			if !ok {
				continue
			}
			if err := tx.Model(&model.PrintRun{}).Where("id = ?", runs[i].ID).
				Update("proof_id", pid).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func seedReleaseDecision(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.ReleaseDecision{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	var runs []model.PrintRun
	if err := db.WithContext(ctx).Order("code ASC").Find(&runs).Error; err != nil {
		return err
	}
	runID := func(code string) *uint {
		for i := range runs {
			if runs[i].Code == code {
				id := runs[i].ID
				return &id
			}
		}
		return nil
	}
	items := []model.ReleaseDecision{

		{BaseModel: model.BaseModel{Code: "RD-001", Name: "放行决定示例一", Status: "draft", Version: 1,
			Description: "校样合格、待复核员放行的草稿"}, Facility: "印刷色彩批次校准放行区域3", Owner: "运行一组",
			Category: "常规", RiskLevel: "high", MetricValue: 1.8, MetricUnit: "ΔE",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "依据 CP-003 的合格校样", RelatedCode: "REL-517-03",
			PrintRunID: runID("PR-003")},

		{BaseModel: model.BaseModel{Code: "RD-002", Name: "放行决定示例二", Status: "release", Version: 1,
			Description: "校样合格、批次已完成放行"}, Facility: "印刷色彩批次校准放行区域5", Owner: "质量复核组",
			Category: "重点", RiskLevel: "low", MetricValue: 2.0, MetricUnit: "ΔE",
			EffectiveAt: now.Add(12 * time.Hour), Evidence: "依据 CP-005 的合格校样放行", RelatedCode: "REL-517-05",
			PrintRunID: runID("PR-005")},

		{BaseModel: model.BaseModel{Code: "RD-003", Name: "放行决定示例三", Status: "rework", Version: 1,
			Description: "色差超容差、批次转等待并要求返修"}, Facility: "印刷色彩批次校准放行区域4", Owner: "安全主管组",
			Category: "复核", RiskLevel: "critical", MetricValue: 4.6, MetricUnit: "ΔE",
			EffectiveAt: now.Add(9 * time.Hour), Evidence: "依据 CP-004 的不合格校样返修", RelatedCode: "REL-517-04",
			PrintRunID: runID("PR-004")},
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Revisions").Create(&items).Error; err != nil {
			return err
		}
		revisions := make([]model.ReleaseDecisionRevision, 0, len(items))
		for _, item := range items {
			revisions = append(revisions, model.ReleaseDecisionRevision{
				ReleaseDecisionID: item.ID, Version: item.Version, Status: item.Status, Name: item.Name,
				RiskLevel: item.RiskLevel, MetricValue: item.MetricValue, MetricUnit: item.MetricUnit,
				Evidence: item.Evidence, RelatedCode: item.RelatedCode,
				Actor: "seed", RequestID: "startup-seed", Reason: "initial release decision",
			})
		}
		return tx.Create(&revisions).Error
	})
}
