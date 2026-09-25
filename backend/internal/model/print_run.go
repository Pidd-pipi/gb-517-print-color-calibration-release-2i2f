package model

import "time"

// PrintRun models 印刷批次 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
type PrintRun struct {
	BaseModel
	Facility    string             `json:"facility" gorm:"size:120;index"`
	Owner       string             `json:"owner" gorm:"size:120;index"`
	Category    string             `json:"category" gorm:"size:80;index"`
	RiskLevel   string             `json:"riskLevel" gorm:"size:32;index"`
	MetricValue float64            `json:"metricValue"`
	MetricUnit  string             `json:"metricUnit" gorm:"size:24"`
	EffectiveAt time.Time          `json:"effectiveAt"`
	Evidence    string             `json:"evidence" gorm:"size:2000"`
	RelatedCode string             `json:"relatedCode" gorm:"size:64;index"`
	Revisions   []PrintRunRevision `json:"revisions,omitempty" gorm:"foreignKey:PrintRunID"`
}

func (item *PrintRun) GetBase() *BaseModel { return &item.BaseModel }

func (item PrintRun) TableName() string { return "print_runs" }

var PrintRunInitialStatus = "setup"

// PrintRunRevision is an append-only snapshot of the run's colour
// configuration. It is deliberately separate from optimistic locking so an
// operator can never overwrite the evidence used by an earlier decision.
type PrintRunRevision struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	PrintRunID  uint      `json:"printRunId" gorm:"not null;uniqueIndex:idx_print_run_revision"`
	Version     uint      `json:"version" gorm:"not null;uniqueIndex:idx_print_run_revision"`
	Status      string    `json:"status" gorm:"size:40;not null"`
	Name        string    `json:"name" gorm:"size:160;not null"`
	Facility    string    `json:"facility" gorm:"size:120"`
	Owner       string    `json:"owner" gorm:"size:120"`
	Category    string    `json:"category" gorm:"size:80"`
	RiskLevel   string    `json:"riskLevel" gorm:"size:32"`
	MetricValue float64   `json:"metricValue"`
	MetricUnit  string    `json:"metricUnit" gorm:"size:24"`
	Evidence    string    `json:"evidence" gorm:"size:2000"`
	RelatedCode string    `json:"relatedCode" gorm:"size:64"`
	Actor       string    `json:"actor" gorm:"size:80;not null"`
	RequestID   string    `json:"requestId" gorm:"size:80;not null"`
	Reason      string    `json:"reason" gorm:"size:500;not null"`
	CreatedAt   time.Time `json:"createdAt"`
}
