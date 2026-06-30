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

// CreateImport 创建一个 queued 状态的导入任务。
func (r *repository) CreateImport(ctx context.Context, cmd CreateImportCommand) (ImportRecord, error) {
	if r == nil || r.db == nil {
		return ImportRecord{}, errors.New("规则管理仓库未初始化")
	}
	if cmd.SourceID == 0 {
		return ImportRecord{}, errors.New("导入任务来源 ID 不能为空")
	}

	imp := model.ModerationRuleImport{
		FileName:         cmd.FileName,
		Format:           cmd.Format,
		FileSize:         cmd.FileSize,
		ObjectKey:        cmd.ObjectKey,
		SourceID:         cmd.SourceID,
		DefaultCategory:  cmd.DefaultCategory,
		DefaultEffect:    cmd.DefaultEffect,
		DefaultRiskLevel: cmd.DefaultRiskLevel,
		DefaultPriority:  cmd.DefaultPriority,
		ValidationStatus: ImportStatusQueued,
		OperatorID:       cmd.OperatorID,
	}
	if err := r.db.WithContext(ctx).Create(&imp).Error; err != nil {
		return ImportRecord{}, fmt.Errorf("创建导入任务: %w", err)
	}
	return importRecordFromModel(imp), nil
}

// ClaimNextImport 原子认领一个 queued 导入任务，将其更新为 validating。
func (r *repository) ClaimNextImport(ctx context.Context, now time.Time) (*ImportRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("规则管理仓库未初始化")
	}

	var claimed ImportRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var imp model.ModerationRuleImport
		result := tx.Where("validation_status = ?", ImportStatusQueued).
			Order("id ASC").
			Limit(1).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Find(&imp)
		if result.Error != nil {
			return fmt.Errorf("查询待处理导入任务: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Model(&imp).Updates(map[string]any{
			"validation_status": ImportStatusValidating,
			"updated_at":        now,
		}).Error; err != nil {
			return fmt.Errorf("认领导入任务: %w", err)
		}
		imp.ValidationStatus = ImportStatusValidating
		imp.UpdatedAt = now
		claimed = importRecordFromModel(imp)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if claimed.ID == 0 {
		return nil, nil
	}
	return &claimed, nil
}

// UpdateImportValidation 写回校验状态、行数统计和关联规则集。
func (r *repository) UpdateImportValidation(ctx context.Context, cmd UpdateImportValidationCommand) error {
	if r == nil || r.db == nil {
		return errors.New("规则管理仓库未初始化")
	}
	updates := map[string]any{
		"validation_status": cmd.ValidationStatus,
		"total_rows":        cmd.TotalRows,
		"valid_rows":        cmd.ValidRows,
		"duplicate_rows":    cmd.DuplicateRows,
		"error_rows":        cmd.ErrorRows,
		"updated_at":        time.Now(),
	}
	if cmd.ErrorObjectKey != nil {
		updates["error_object_key"] = *cmd.ErrorObjectKey
	}
	if cmd.RulesetID != nil {
		updates["ruleset_id"] = *cmd.RulesetID
	}
	result := r.db.WithContext(ctx).
		Table("moderation_rule_import").
		Where("id = ?", cmd.ID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("更新导入校验结果: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrCandidateNotFound
	}
	return nil
}

// ListImports 按游标分页查询导入历史。
func (r *repository) ListImports(ctx context.Context, afterID uint64, limit int) (ImportPage, error) {
	if r == nil || r.db == nil {
		return ImportPage{}, errors.New("规则管理仓库未初始化")
	}
	limit = normalizeListLimit(limit)

	query := r.db.WithContext(ctx).
		Table("moderation_rule_import").
		Select("id", "file_name", "format", "file_size", "object_key", "source_id",
			"default_category", "default_effect", "default_risk_level", "default_priority",
			"validation_status", "total_rows", "valid_rows", "duplicate_rows", "error_rows",
			"error_object_key", "ruleset_id", "operator_id", "created_at", "updated_at").
		Order("id DESC")

	if afterID > 0 {
		query = query.Where("id < ?", afterID)
	}
	query = query.Limit(limit + 1)

	rows, err := query.Rows()
	if err != nil {
		return ImportPage{}, fmt.Errorf("查询导入历史: %w", err)
	}
	defer rows.Close()

	imports := make([]ImportRecord, 0, limit+1)
	for rows.Next() {
		var rec ImportRecord
		if err := rows.Scan(
			&rec.ID, &rec.FileName, &rec.Format, &rec.FileSize, &rec.ObjectKey,
			&rec.SourceID, &rec.DefaultCategory, &rec.DefaultEffect, &rec.DefaultRiskLevel,
			&rec.DefaultPriority, &rec.ValidationStatus, &rec.TotalRows, &rec.ValidRows,
			&rec.DuplicateRows, &rec.ErrorRows, &rec.ErrorObjectKey, &rec.RulesetID,
			&rec.OperatorID, &rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return ImportPage{}, fmt.Errorf("扫描导入行: %w", err)
		}
		imports = append(imports, rec)
	}
	if err := rows.Err(); err != nil {
		return ImportPage{}, fmt.Errorf("读取导入行流: %w", err)
	}

	page := ImportPage{Imports: imports}
	if len(imports) > limit {
		page.HasMore = true
		page.NextCursor = imports[limit].ID
		page.Imports = imports[:limit]
	}
	return page, nil
}

// GetImport 返回指定导入任务的详情。
func (r *repository) GetImport(ctx context.Context, id uint64) (ImportRecord, error) {
	if r == nil || r.db == nil {
		return ImportRecord{}, errors.New("规则管理仓库未初始化")
	}
	var rec ImportRecord
	err := r.db.WithContext(ctx).
		Table("moderation_rule_import").
		Select("id", "file_name", "format", "file_size", "object_key", "source_id",
			"default_category", "default_effect", "default_risk_level", "default_priority",
			"validation_status", "total_rows", "valid_rows", "duplicate_rows", "error_rows",
			"error_object_key", "ruleset_id", "operator_id", "created_at", "updated_at").
		Where("id = ?", id).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ImportRecord{}, ErrCandidateNotFound
		}
		return ImportRecord{}, fmt.Errorf("查询导入任务: %w", err)
	}
	return rec, nil
}

// CancelImport 取消尚未发布的导入任务。
func (r *repository) CancelImport(ctx context.Context, id, actorID uint64, now time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("规则管理仓库未初始化")
	}
	result := r.db.WithContext(ctx).
		Table("moderation_rule_import").
		Where("id = ? AND validation_status IN ?", id, []string{ImportStatusQueued, ImportStatusValidating, ImportStatusValid}).
		Updates(map[string]any{
			"validation_status": ImportStatusCanceled,
			"updated_at":        now,
		})
	if result.Error != nil {
		return fmt.Errorf("取消导入任务: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrCandidateNotFound
	}
	return nil
}

// ResetInterruptedImports 将 validating 状态的导入任务重置为 queued，用于进程重启后恢复。
func (r *repository) ResetInterruptedImports(ctx context.Context, now time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("规则管理仓库未初始化")
	}
	result := r.db.WithContext(ctx).
		Table("moderation_rule_import").
		Where("validation_status = ?", ImportStatusValidating).
		Updates(map[string]any{
			"validation_status": ImportStatusQueued,
			"updated_at":        now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("重置中断导入任务: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func importRecordFromModel(imp model.ModerationRuleImport) ImportRecord {
	return ImportRecord{
		ID:               imp.ID,
		FileName:         imp.FileName,
		Format:           imp.Format,
		FileSize:         imp.FileSize,
		ObjectKey:        imp.ObjectKey,
		SourceID:         imp.SourceID,
		DefaultCategory:  imp.DefaultCategory,
		DefaultEffect:    imp.DefaultEffect,
		DefaultRiskLevel: imp.DefaultRiskLevel,
		DefaultPriority:  imp.DefaultPriority,
		ValidationStatus: imp.ValidationStatus,
		TotalRows:        imp.TotalRows,
		ValidRows:        imp.ValidRows,
		DuplicateRows:    imp.DuplicateRows,
		ErrorRows:        imp.ErrorRows,
		ErrorObjectKey:   imp.ErrorObjectKey,
		RulesetID:        imp.RulesetID,
		OperatorID:       imp.OperatorID,
		CreatedAt:        imp.CreatedAt,
		UpdatedAt:        imp.UpdatedAt,
	}
}
