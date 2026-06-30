package moderationrule

import (
	"context"
	"errors"
	"fmt"
)

func (r *repository) CurrentRuleset(ctx context.Context) (RulesetRecord, error) {
	if r == nil || r.db == nil {
		return RulesetRecord{}, errors.New("规则快照仓库未初始化")
	}
	var record RulesetRecord
	err := r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Select("id", "status", "index_object_key", "index_format_version", "index_sha256", "index_bytes").
		Where("status = ?", "published").
		Order("id DESC").
		Limit(1).
		Take(&record).Error
	if err != nil {
		return RulesetRecord{}, fmt.Errorf("读取当前审核规则集: %w", err)
	}
	return record, nil
}

func (r *repository) StreamRules(ctx context.Context, version uint64, visit func(RuleRecord) error) error {
	if r == nil || r.db == nil {
		return errors.New("规则快照仓库未初始化")
	}
	if version == 0 || visit == nil {
		return errors.New("规则行流参数无效")
	}

	rows, err := r.db.WithContext(ctx).
		Table("moderation_rule AS rule").
		Select("rule.id", "rule.rule_type", "rule.pattern", "rule.risk_level", "rule.effect", "rule.priority").
		Joins("JOIN moderation_ruleset AS activation ON activation.id = rule.activated_ruleset_id AND activation.status IN ?", []string{"published", "superseded"}).
		Where("rule.activated_ruleset_id <= ?", version).
		Where("rule.deactivated_ruleset_id IS NULL OR rule.deactivated_ruleset_id > ?", version).
		Order("rule.id ASC").
		Rows()
	if err != nil {
		return fmt.Errorf("打开审核规则行流: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var record RuleRecord
		if err := rows.Scan(
			&record.ID,
			&record.RuleType,
			&record.Pattern,
			&record.RiskLevel,
			&record.Effect,
			&record.Priority,
		); err != nil {
			return fmt.Errorf("扫描审核规则行: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取审核规则行流: %w", err)
	}
	return nil
}
