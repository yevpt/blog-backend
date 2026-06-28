package moderation

import (
	"context"

	"github.com/vpt/blog-backend/internal/model"
)

func (r *repository) LoadEnabledRules(ctx context.Context) ([]RuleRecord, error) {
	var rows []model.ModerationRule
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("priority ASC,id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	rules := make([]RuleRecord, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, RuleRecord{
			ID: row.ID, Name: row.Name, RuleType: row.RuleType, Pattern: row.Pattern,
			RiskLevel: RiskLevel(row.RiskLevel), Priority: row.Priority, RulesetVersion: row.RulesetVersion,
		})
	}
	return rules, nil
}
