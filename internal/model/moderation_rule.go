package model

import "time"

const (
	ModerationRuleKeyword   = "keyword"
	ModerationRuleRegexp    = "regexp"
	ModerationRuleComposite = "composite"
)

// ModerationRule 是不可原地覆盖的文本审核规则版本。
type ModerationRule struct {
	ID                   uint64    `gorm:"primaryKey"`
	Name                 *string   `gorm:"size:100"`
	RuleType             string    `gorm:"size:16;not null"`
	Pattern              string    `gorm:"size:500;not null"`
	DedupeHash           []byte    `gorm:"type:binary(32);not null;index:idx_moderation_rule_dedupe"`
	Category             string    `gorm:"size:24;not null;index:idx_moderation_rule_filter,priority:1"`
	Effect               string    `gorm:"size:16;not null"`
	RiskLevel            string    `gorm:"size:16;not null;index:idx_moderation_rule_filter,priority:2"`
	Priority             int32     `gorm:"not null;default:100"`
	SourceID             uint64    `gorm:"not null;index:idx_moderation_rule_source"`
	ActivatedRulesetID   uint64    `gorm:"not null;index:idx_moderation_rule_interval,priority:1"`
	DeactivatedRulesetID *uint64   `gorm:"index:idx_moderation_rule_interval,priority:2"`
	ReplacesRuleID       *uint64   `gorm:"index:idx_moderation_rule_replaces"`
	CreatedAt            time.Time `gorm:"type:datetime(3);not null"`
	UpdatedAt            time.Time `gorm:"type:datetime(3);not null"`
}

func (ModerationRule) TableName() string { return "moderation_rule" }

// ModerationRuleSource 保存规则来源目录。
type ModerationRuleSource struct {
	ID        uint64    `gorm:"primaryKey"`
	Name      string    `gorm:"size:100;not null;uniqueIndex:uk_moderation_rule_source_name"`
	CreatedAt time.Time `gorm:"type:datetime(3);not null"`
	UpdatedAt time.Time `gorm:"type:datetime(3);not null"`
}

func (ModerationRuleSource) TableName() string { return "moderation_rule_source" }
