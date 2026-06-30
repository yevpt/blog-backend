package moderationrule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateCandidate 在单个事务中创建 building 规则集、规则事实和停用关系。
func (r *repository) CreateCandidate(ctx context.Context, cmd CreateCandidateCommand) (CandidateRecord, error) {
	if r == nil || r.db == nil {
		return CandidateRecord{}, errors.New("规则管理仓库未初始化")
	}
	if cmd.BaseRulesetID == 0 {
		return CandidateRecord{}, errors.New("候选基础规则集 ID 不能为空")
	}

	var candidate CandidateRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		baseID := cmd.BaseRulesetID
		operatorID := cmd.ActorID
		ruleset := model.ModerationRuleset{
			BaseRulesetID: &baseID,
			Status:        StatusBuilding,
			OperatorID:    &operatorID,
		}
		if err := tx.Create(&ruleset).Error; err != nil {
			return fmt.Errorf("创建候选规则集: %w", err)
		}

		// 批量插入新规则事实，每条关联候选规则集 ID。
		for _, draft := range cmd.Additions {
			rule := model.ModerationRule{
				Name:               draft.Name,
				RuleType:           draft.RuleType,
				Pattern:            draft.Pattern,
				DedupeHash:         draft.DedupeHash[:],
				Category:           draft.Category,
				Effect:             draft.Effect,
				RiskLevel:          draft.RiskLevel,
				Priority:           draft.Priority,
				SourceID:           draft.SourceID,
				ActivatedRulesetID: ruleset.ID,
			}
			if err := tx.Create(&rule).Error; err != nil {
				return fmt.Errorf("创建候选规则行: %w", err)
			}
		}

		// 批量插入停用关系。
		for _, ruleID := range cmd.RemoveRuleIDs {
			removal := model.ModerationRulesetRemoval{
				RulesetID: ruleset.ID,
				RuleID:    ruleID,
				CreatedAt: time.Now(),
			}
			if err := tx.Create(&removal).Error; err != nil {
				return fmt.Errorf("创建候选停用关系: %w", err)
			}
		}

		candidate = CandidateRecord{
			RulesetID:     ruleset.ID,
			Status:        ruleset.Status,
			BaseRulesetID: baseID,
			CreatedAt:     ruleset.CreatedAt,
			UpdatedAt:     ruleset.UpdatedAt,
		}
		return nil
	})
	if err != nil {
		return CandidateRecord{}, err
	}
	return candidate, nil
}

// PublishCandidate 在事务内验证基础版本、应用停用、切换已发布规则集。
func (r *repository) PublishCandidate(ctx context.Context, id, expectedBase uint64) error {
	if r == nil || r.db == nil {
		return errors.New("规则管理仓库未初始化")
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 加锁并校验候选规则集状态。
		var candidate model.ModerationRuleset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			Take(&candidate).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCandidateNotFound
			}
			return fmt.Errorf("锁定候选规则集: %w", err)
		}
		if candidate.Status != StatusReady {
			return ErrRulesetConflict
		}
		if candidate.BaseRulesetID == nil || *candidate.BaseRulesetID != expectedBase {
			return ErrRulesetConflict
		}

		// 加锁并校验当前已发布规则集。
		var current model.ModerationRuleset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ?", StatusPublished).
			Order("id DESC").
			Take(&current).Error; err != nil {
			return fmt.Errorf("锁定当前已发布规则集: %w", err)
		}
		if current.ID != expectedBase {
			return ErrRulesetConflict
		}

		// 应用停用关系：为被停用规则填写 deactivated_ruleset_id。
		if err := tx.Table("moderation_rule").
			Where("id IN (?)",
				tx.Table("moderation_ruleset_removal").
					Select("rule_id").
					Where("ruleset_id = ?", id)).
			Update("deactivated_ruleset_id", id).Error; err != nil {
			return fmt.Errorf("应用规则停用: %w", err)
		}

		// 旧规则集转 superseded，候选转 published。
		now := time.Now()
		if err := tx.Model(&current).Updates(map[string]any{
			"status":     StatusSuperseded,
			"updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("归档旧规则集: %w", err)
		}
		if err := tx.Model(&candidate).Updates(map[string]any{
			"status":     StatusPublished,
			"updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("发布候选规则集: %w", err)
		}
		return nil
	})
}

// ClaimNextRuleset 原子认领一个指定状态的规则集，将其更新为 publishing 以避免重复处理。
func (r *repository) ClaimNextRuleset(ctx context.Context, status string) (*CandidateRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("规则管理仓库未初始化")
	}

	var claimed CandidateRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ruleset model.ModerationRuleset
		if err := tx.Where("status = ?", status).
			Order("id ASC").
			Limit(1).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&ruleset).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("查询待处理规则集: %w", err)
		}
		if err := tx.Model(&ruleset).Update("status", StatusBuilding).Error; err != nil {
			return fmt.Errorf("认领规则集: %w", err)
		}
		baseID := uint64(0)
		if ruleset.BaseRulesetID != nil {
			baseID = *ruleset.BaseRulesetID
		}
		claimed = CandidateRecord{
			RulesetID:     ruleset.ID,
			Status:        StatusBuilding,
			BaseRulesetID: baseID,
			CreatedAt:     ruleset.CreatedAt,
			UpdatedAt:     ruleset.UpdatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if claimed.RulesetID == 0 {
		return nil, nil
	}
	return &claimed, nil
}

// FailRuleset 将规则集标记为失败并记录失败原因。
func (r *repository) FailRuleset(ctx context.Context, id uint64, failureCode string) error {
	if r == nil || r.db == nil {
		return errors.New("规则管理仓库未初始化")
	}
	return r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       StatusFailed,
			"failure_code": failureCode,
			"updated_at":   time.Now(),
		}).Error
}

// SaveRulesetBuildResult 写回索引构建统计和对象元数据，并将状态切换为 ready。
func (r *repository) SaveRulesetBuildResult(ctx context.Context, id uint64, result BuildResult) error {
	if r == nil || r.db == nil {
		return errors.New("规则管理仓库未初始化")
	}
	return r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Where("id = ?", id).
		Updates(map[string]any{
			"status":            StatusReady,
			"rule_count":        result.RuleCount,
			"keyword_count":     result.KeywordCount,
			"regexp_count":      result.RegexpCount,
			"composite_count":   result.CompositeCount,
			"index_bytes":       result.IndexBytes,
			"build_peak_bytes":  result.BuildPeakBytes,
			"build_duration_ms": result.BuildDurationMS,
			"index_object_key":  result.IndexObjectKey,
			"index_sha256":      result.IndexSHA256,
			"updated_at":        time.Now(),
		}).Error
}

// CancelCandidate 取消尚未发布的候选规则集。
func (r *repository) CancelCandidate(ctx context.Context, id, actorID uint64) error {
	if r == nil || r.db == nil {
		return errors.New("规则管理仓库未初始化")
	}
	result := r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Where("id = ? AND status IN ?", id, []string{StatusBuilding, StatusReady}).
		Updates(map[string]any{
			"status":     StatusFailed,
			"failure_code": "canceled",
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("取消候选规则集: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrCandidateNotFound
	}
	return nil
}

// StreamCandidateRules 流式输出候选规则集构建索引所需的全部规则：
// 基础版本的有效规则（排除停用）加上候选新增规则。
func (r *repository) StreamCandidateRules(ctx context.Context, baseVersion, candidateID uint64, visit func(RuleRecord) error) error {
	if r == nil || r.db == nil {
		return errors.New("规则管理仓库未初始化")
	}
	if visit == nil {
		return errors.New("规则行流参数无效")
	}

	rows, err := r.db.WithContext(ctx).
		Table("moderation_rule AS rule").
		Select("rule.id", "rule.rule_type", "rule.pattern", "rule.risk_level", "rule.effect", "rule.priority").
		Joins("JOIN moderation_ruleset AS activation ON activation.id = rule.activated_ruleset_id").
		Where(
			"(activation.status IN ? AND rule.activated_ruleset_id <= ? AND (rule.deactivated_ruleset_id IS NULL OR rule.deactivated_ruleset_id > ?) AND rule.id NOT IN (?)) OR rule.activated_ruleset_id = ?",
			[]string{StatusPublished, StatusSuperseded}, baseVersion, baseVersion,
			r.db.Table("moderation_ruleset_removal").Select("rule_id").Where("ruleset_id = ?", candidateID),
			candidateID,
		).
		Order("rule.id ASC").
		Rows()
	if err != nil {
		return fmt.Errorf("打开候选规则行流: %w", err)
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
			return fmt.Errorf("扫描候选规则行: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取候选规则行流: %w", err)
	}
	return nil
}

// GetRulesetRemovals 返回候选规则集的停用规则 ID 列表。
func (r *repository) GetRulesetRemovals(ctx context.Context, rulesetID uint64) ([]uint64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("规则管理仓库未初始化")
	}
	var ids []uint64
	err := r.db.WithContext(ctx).
		Table("moderation_ruleset_removal").
		Select("rule_id").
		Where("ruleset_id = ?", rulesetID).
		Order("rule_id ASC").
		Pluck("rule_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("查询候选停用规则: %w", err)
	}
	return ids, nil
}

// GetCandidate 返回指定规则集的候选摘要。
func (r *repository) GetCandidate(ctx context.Context, id uint64) (CandidateRecord, error) {
	if r == nil || r.db == nil {
		return CandidateRecord{}, errors.New("规则管理仓库未初始化")
	}
	var row struct {
		ID              uint64
		Status          string
		BaseRulesetID   *uint64
		RuleCount       uint64
		KeywordCount    uint64
		RegexpCount     uint64
		CompositeCount  uint64
		IndexBytes      uint64
		BuildPeakBytes  uint64
		IndexObjectKey  *string
		IndexSHA256     *string
		FailureCode     *string
		CreatedAt       time.Time
		UpdatedAt       time.Time
	}
	err := r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Select("id", "status", "base_ruleset_id", "rule_count", "keyword_count",
			"regexp_count", "composite_count", "index_bytes", "build_peak_bytes",
			"index_object_key", "index_sha256", "failure_code", "created_at", "updated_at").
		Where("id = ?", id).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CandidateRecord{}, ErrCandidateNotFound
		}
		return CandidateRecord{}, fmt.Errorf("查询候选规则集: %w", err)
	}
	baseID := uint64(0)
	if row.BaseRulesetID != nil {
		baseID = *row.BaseRulesetID
	}
	objectKey := ""
	if row.IndexObjectKey != nil {
		objectKey = *row.IndexObjectKey
	}
	sha := ""
	if row.IndexSHA256 != nil {
		sha = *row.IndexSHA256
	}
	return CandidateRecord{
		RulesetID:      row.ID,
		Status:         row.Status,
		BaseRulesetID:  baseID,
		RuleCount:      row.RuleCount,
		KeywordCount:   row.KeywordCount,
		RegexpCount:    row.RegexpCount,
		CompositeCount: row.CompositeCount,
		IndexBytes:     row.IndexBytes,
		BuildPeakBytes: row.BuildPeakBytes,
		IndexObjectKey: objectKey,
		IndexSHA256:    sha,
		FailureCode:    row.FailureCode,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

// HasImportForRuleset 检查规则集是否被导入任务引用，决定是否自动发布。
func (r *repository) HasImportForRuleset(ctx context.Context, rulesetID uint64) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("规则管理仓库未初始化")
	}
	var count int64
	err := r.db.WithContext(ctx).
		Table("moderation_rule_import").
		Where("ruleset_id = ?", rulesetID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("查询导入关联: %w", err)
	}
	return count > 0, nil
}
