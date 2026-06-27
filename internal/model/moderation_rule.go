package model

import "time"

const (
	ModerationRuleKeyword   = "keyword"
	ModerationRuleRegexp    = "regexp"
	ModerationRuleComposite = "composite"
)

// ModerationRule 是不可原地覆盖的文本审核规则版本。
type ModerationRule struct {
	ID             uint64    `gorm:"primaryKey"`
	Name           string    `gorm:"size:100;not null"`
	RuleType       string    `gorm:"size:16;not null;check:chk_moderation_rule_type,rule_type IN ('keyword','regexp','composite')"`
	Pattern        string    `gorm:"size:500;not null"`
	RiskLevel      string    `gorm:"size:16;not null;check:chk_moderation_rule_risk,risk_level IN ('low','medium','high')"`
	Priority       int       `gorm:"not null;default:100;index:idx_moderation_rule_snapshot,priority:3"`
	Enabled        bool      `gorm:"type:tinyint(1);not null;default:0;index:idx_moderation_rule_snapshot,priority:2"`
	RulesetVersion uint64    `gorm:"not null;index:idx_moderation_rule_snapshot,priority:1"`
	CreatedAt      time.Time `gorm:"type:datetime(3);not null"`
	UpdatedAt      time.Time `gorm:"type:datetime(3);not null"`
}

func (ModerationRule) TableName() string { return "moderation_rule" }
