package moderationrule

import (
	"context"
	"fmt"
	"time"

	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
	"go.uber.org/zap"
)

// ProcessNextImport 认领并处理一个排队中的导入任务。
func (m *manager) ProcessNextImport(ctx context.Context) error {
	imp, err := m.repo.ClaimNextImport(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("认领导入任务: %w", err)
	}
	if imp == nil {
		return nil
	}
	return m.processImport(ctx, imp)
}

// processImport 执行单次导入校验和候选创建。
func (m *manager) processImport(ctx context.Context, imp *repoMod.ImportRecord) error {
	result, err := m.validateImport(ctx, *imp)
	if err != nil {
		m.logger.Error("导入校验失败",
			zap.Uint64("import_id", imp.ID),
			zap.Error(err),
		)
		_ = m.repo.UpdateImportValidation(ctx, repoMod.UpdateImportValidationCommand{
			ID:               imp.ID,
			ValidationStatus: repoMod.ImportStatusInvalid,
			TotalRows:        result.totalRows,
			ErrorRows:        result.errorRows,
		})
		return err
	}

	status := repoMod.ImportStatusValid
	if result.errorRows > 0 {
		status = repoMod.ImportStatusInvalid
	}

	var errorKey *string
	if result.errorKey != "" {
		errorKey = &result.errorKey
	}

	if err := m.repo.UpdateImportValidation(ctx, repoMod.UpdateImportValidationCommand{
		ID:               imp.ID,
		ValidationStatus: status,
		TotalRows:        result.totalRows,
		ValidRows:        result.validRows,
		DuplicateRows:    result.duplicateRows,
		ErrorRows:        result.errorRows,
		ErrorObjectKey:   errorKey,
		RulesetID:        result.rulesetID,
	}); err != nil {
		return fmt.Errorf("更新导入校验结果: %w", err)
	}
	return nil
}

// CreateImport 创建导入任务，先确保来源存在再写入任务记录。
func (m *manager) CreateImport(ctx context.Context, input CreateImportInput) (repoMod.ImportRecord, error) {
	if err := validateImportInput(input); err != nil {
		return repoMod.ImportRecord{}, err
	}
	format, err := normalizeImportFormat(input.Format)
	if err != nil {
		return repoMod.ImportRecord{}, err
	}

	source, err := m.repo.EnsureSource(ctx, input.SourceName)
	if err != nil {
		return repoMod.ImportRecord{}, fmt.Errorf("确保来源存在: %w", err)
	}

	return m.repo.CreateImport(ctx, repoMod.CreateImportCommand{
		FileName:         input.FileName,
		Format:           format,
		FileSize:         input.FileSize,
		ObjectKey:        input.ObjectKey,
		SourceID:         source.ID,
		DefaultCategory:  input.DefaultCategory,
		DefaultEffect:    input.DefaultEffect,
		DefaultRiskLevel: input.DefaultRiskLevel,
		DefaultPriority:  input.DefaultPriority,
		OperatorID:       input.OperatorID,
	})
}

// ListImports 分页查询导入历史。
func (m *manager) ListImports(ctx context.Context, afterID uint64, limit int) (repoMod.ImportPage, error) {
	return m.repo.ListImports(ctx, afterID, limit)
}

// GetImport 返回导入任务详情。
func (m *manager) GetImport(ctx context.Context, id uint64) (repoMod.ImportRecord, error) {
	return m.repo.GetImport(ctx, id)
}

// CancelImport 取消导入任务。
func (m *manager) CancelImport(ctx context.Context, id, actorID uint64) error {
	if err := m.repo.CancelImport(ctx, id, actorID, time.Now()); err != nil {
		if isRepoNotFound(err) {
			return ErrCandidateNotFound
		}
		return fmt.Errorf("取消导入任务: %w", err)
	}
	return nil
}

// ResetInterrupted 重置中断的导入和构建任务，在进程启动时调用。
func (m *manager) ResetInterrupted(ctx context.Context) error {
	count, err := m.repo.ResetInterruptedImports(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("重置中断导入: %w", err)
	}
	if count > 0 {
		m.logger.Info("重置中断导入任务", zap.Int64("count", count))
	}
	return nil
}

func isRepoNotFound(err error) bool {
	return err.Error() == repoMod.ErrCandidateNotFound.Error()
}
