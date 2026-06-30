package moderationrule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// CleanupRepository 提供规则集和导入产物的有界清理查询。
type CleanupRepository interface {
	DeleteFailedRulesetRules(ctx context.Context, rulesetID uint64) (int64, error)
	DeleteExpiredRulesetArtifacts(ctx context.Context, before time.Time, limit int) ([]uint64, error)
	MarkRulesetArtifactsDeleted(ctx context.Context, ids []uint64) error
	DeleteExpiredImportArtifacts(ctx context.Context, before time.Time, limit int) ([]ImportArtifact, error)
	MarkImportArtifactsDeleted(ctx context.Context, ids []uint64) error
}

// ImportArtifact 是需要清理对象存储的导入任务摘要。
type ImportArtifact struct {
	ImportID      uint64
	ObjectKey     string
	ErrorObjectKey *string
}

type cleanupRepository struct {
	db *gorm.DB
}

// NewCleanupRepository 创建规则清理仓库。
func NewCleanupRepository(db *gorm.DB) CleanupRepository {
	return &cleanupRepository{db: db}
}

// DeleteFailedRulesetRules 删除失败/取消候选的未发布规则事实和停用关系。
func (r *cleanupRepository) DeleteFailedRulesetRules(ctx context.Context, rulesetID uint64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("清理仓库未初始化")
	}
	result := r.db.WithContext(ctx).
		Where("ruleset_id = ?", rulesetID).
		Delete(&struct {
			RulesetID uint64 `gorm:"column:ruleset_id"`
		}{}, "moderation_ruleset_removal")
	if result.Error != nil {
		return 0, fmt.Errorf("删除候选停用关系: %w", result.Error)
	}
	deleted := result.RowsAffected

	result = r.db.WithContext(ctx).
		Table("moderation_rule").
		Where("activated_ruleset_id = ? AND NOT EXISTS (?)",
			rulesetID,
			r.db.Table("moderation_ruleset").Select("1").Where("id = activated_ruleset_id AND status = ?", StatusPublished),
		).
		Delete(nil)
	if result.Error != nil {
		return deleted, fmt.Errorf("删除未发布规则事实: %w", result.Error)
	}
	deleted += result.RowsAffected
	return deleted, nil
}

// DeleteExpiredRulesetArtifacts 返回过期失败/取消/superseded 规则集的 ID 列表。
func (r *cleanupRepository) DeleteExpiredRulesetArtifacts(ctx context.Context, before time.Time, limit int) ([]uint64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("清理仓库未初始化")
	}
	if limit <= 0 {
		limit = 100
	}
	var ids []uint64
	err := r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Select("id").
		Where("status IN ? AND updated_at < ? AND index_object_key IS NOT NULL",
			[]string{StatusFailed, StatusSuperseded}, before).
		Order("id ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("查询过期规则集产物: %w", err)
	}
	return ids, nil
}

// MarkRulesetArtifactsDeleted 清除规则集的索引对象引用，表示产物已删除。
func (r *cleanupRepository) MarkRulesetArtifactsDeleted(ctx context.Context, ids []uint64) error {
	if r == nil || r.db == nil {
		return errors.New("清理仓库未初始化")
	}
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Where("id IN ?", ids).
		Updates(map[string]any{
			"index_object_key": nil,
			"index_sha256":     nil,
			"updated_at":       time.Now(),
		}).Error
}

// DeleteExpiredImportArtifacts 返回过期导入任务的对象键列表。
func (r *cleanupRepository) DeleteExpiredImportArtifacts(ctx context.Context, before time.Time, limit int) ([]ImportArtifact, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("清理仓库未初始化")
	}
	if limit <= 0 {
		limit = 100
	}
	var artifacts []ImportArtifact
	err := r.db.WithContext(ctx).
		Table("moderation_rule_import").
		Select("id", "object_key", "error_object_key").
		Where("updated_at < ? AND validation_status IN ?",
			before, []string{ImportStatusInvalid, ImportStatusCanceled}).
		Order("id ASC").
		Limit(limit).
		Scan(&artifacts).Error
	if err != nil {
		return nil, fmt.Errorf("查询过期导入产物: %w", err)
	}
	return artifacts, nil
}

// MarkImportArtifactsDeleted 清除导入任务的对象引用。
func (r *cleanupRepository) MarkImportArtifactsDeleted(ctx context.Context, ids []uint64) error {
	if r == nil || r.db == nil {
		return errors.New("清理仓库未初始化")
	}
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Table("moderation_rule_import").
		Where("id IN ?", ids).
		Updates(map[string]any{
			"object_key":       "",
			"error_object_key": nil,
			"updated_at":       time.Now(),
		}).Error
}
