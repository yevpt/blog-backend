package model

import "time"

// ModerationRuleset 保存候选或已发布规则集的构建元数据。
type ModerationRuleset struct {
	ID                 uint64    `gorm:"primaryKey"`
	BaseRulesetID      *uint64   `gorm:"index:idx_moderation_ruleset_base"`
	Status             string    `gorm:"size:16;not null;index:idx_moderation_ruleset_status"`
	RuleCount          uint64    `gorm:"not null;default:0"`
	KeywordCount       uint64    `gorm:"not null;default:0"`
	RegexpCount        uint64    `gorm:"not null;default:0"`
	CompositeCount     uint64    `gorm:"not null;default:0"`
	IndexBytes         uint64    `gorm:"not null;default:0"`
	BuildPeakBytes     uint64    `gorm:"not null;default:0"`
	BuildDurationMS    uint64    `gorm:"not null;default:0"`
	IndexObjectKey     *string   `gorm:"size:500"`
	IndexFormatVersion uint32    `gorm:"not null;default:1"`
	IndexSHA256        *string   `gorm:"size:64"`
	OperatorID         *uint64   `gorm:"index:idx_moderation_ruleset_operator"`
	FailureCode        *string   `gorm:"size:64"`
	CreatedAt          time.Time `gorm:"type:datetime(3);not null"`
	UpdatedAt          time.Time `gorm:"type:datetime(3);not null"`
}

func (ModerationRuleset) TableName() string { return "moderation_ruleset" }

// ModerationRulesetRemoval 保存候选规则集待停用的规则。
type ModerationRulesetRemoval struct {
	RulesetID uint64    `gorm:"primaryKey;autoIncrement:false"`
	RuleID    uint64    `gorm:"primaryKey;autoIncrement:false;index:idx_moderation_ruleset_removal_rule"`
	CreatedAt time.Time `gorm:"type:datetime(3);not null"`
}

func (ModerationRulesetRemoval) TableName() string { return "moderation_ruleset_removal" }

// ModerationRuleImport 保存规则文件导入任务的校验摘要。
type ModerationRuleImport struct {
	ID               uint64    `gorm:"primaryKey"`
	FileName         string    `gorm:"size:255;not null"`
	Format           string    `gorm:"size:8;not null"`
	FileSize         uint64    `gorm:"not null"`
	ObjectKey        string    `gorm:"size:500;not null"`
	SourceID         uint64    `gorm:"not null;index:idx_moderation_rule_import_source"`
	DefaultCategory  string    `gorm:"size:24;not null"`
	DefaultEffect    string    `gorm:"size:16;not null"`
	DefaultRiskLevel string    `gorm:"size:16;not null"`
	DefaultPriority  int32     `gorm:"not null"`
	ValidationStatus string    `gorm:"size:16;not null;index:idx_moderation_rule_import_status"`
	TotalRows        uint64    `gorm:"not null;default:0"`
	ValidRows        uint64    `gorm:"not null;default:0"`
	DuplicateRows    uint64    `gorm:"not null;default:0"`
	ErrorRows        uint64    `gorm:"not null;default:0"`
	ErrorObjectKey   *string   `gorm:"size:500"`
	RulesetID        *uint64   `gorm:"index:idx_moderation_rule_import_ruleset"`
	OperatorID       uint64    `gorm:"not null;index:idx_moderation_rule_import_operator"`
	CreatedAt        time.Time `gorm:"type:datetime(3);not null"`
	UpdatedAt        time.Time `gorm:"type:datetime(3);not null"`
}

func (ModerationRuleImport) TableName() string { return "moderation_rule_import" }
