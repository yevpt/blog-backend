package moderationrule

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
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

// CreateImport 校验并上传导入文件，数据库写入失败时删除新对象。
func (m *manager) CreateImport(ctx context.Context, input CreateImportInput) (repoMod.ImportRecord, error) {
	format, err := normalizeImportFormat(input.Format)
	if err != nil {
		return repoMod.ImportRecord{}, err
	}
	input.Format = format
	if err := validateImportInput(m.cfg, input); err != nil {
		return repoMod.ImportRecord{}, err
	}
	if m.store == nil {
		return repoMod.ImportRecord{}, errors.New("规则导入存储未初始化")
	}

	file, err := stageImportBody(input.Body, input.FileSize)
	if err != nil {
		return repoMod.ImportRecord{}, fmt.Errorf("%w: %s", ErrInvalidRule, err.Error())
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()

	objectKey := fmt.Sprintf("moderation/imports/%d/%s.%s", input.OperatorID, uuid.NewString(), format)
	if err := m.store.PutObjectStream(ctx, objectKey, file, int64(input.FileSize), importContentType(format)); err != nil {
		return repoMod.ImportRecord{}, fmt.Errorf("上传导入文件: %w", err)
	}
	compensate := func() {
		if deleteErr := m.store.DeleteObject(ctx, objectKey); deleteErr != nil {
			m.logger.Error("删除未建任务的导入对象失败", zap.String("object_key", objectKey), zap.Error(deleteErr))
		}
	}

	source, err := m.repo.EnsureSource(ctx, input.SourceName)
	if err != nil {
		compensate()
		return repoMod.ImportRecord{}, fmt.Errorf("确保来源存在: %w", err)
	}

	imp, err := m.repo.CreateImport(ctx, repoMod.CreateImportCommand{
		FileName:         input.FileName,
		Format:           format,
		FileSize:         input.FileSize,
		ObjectKey:        objectKey,
		SourceID:         source.ID,
		DefaultCategory:  input.DefaultCategory,
		DefaultEffect:    input.DefaultEffect,
		DefaultRiskLevel: input.DefaultRiskLevel,
		DefaultPriority:  input.DefaultPriority,
		OperatorID:       input.OperatorID,
	})
	if err != nil {
		compensate()
		return repoMod.ImportRecord{}, fmt.Errorf("创建导入任务: %w", err)
	}
	return imp, nil
}

func stageImportBody(body io.Reader, expectedSize uint64) (*os.File, error) {
	file, err := os.CreateTemp("", "moderation-rule-import-*")
	if err != nil {
		return nil, fmt.Errorf("创建导入临时文件: %w", err)
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}
	written, err := io.Copy(file, io.LimitReader(body, int64(expectedSize)+1))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("读取导入文件: %w", err)
	}
	if written != int64(expectedSize) {
		cleanup()
		return nil, errors.New("文件大小与申报值不一致")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, err
	}
	reader := bufio.NewReader(file)
	for {
		r, size, readErr := reader.ReadRune()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			cleanup()
			return nil, fmt.Errorf("校验 UTF-8: %w", readErr)
		}
		if r == utf8.RuneError && size == 1 {
			cleanup()
			return nil, errors.New("文件必须是 UTF-8 编码")
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, err
	}
	return file, nil
}

func importContentType(format string) string {
	if format == "csv" {
		return "text/csv; charset=utf-8"
	}
	return "text/plain; charset=utf-8"
}

// ListImports 分页查询导入历史。
func (m *manager) ListImports(ctx context.Context, afterID uint64, limit int) (repoMod.ImportPage, error) {
	return m.repo.ListImports(ctx, afterID, limit)
}

// GetImport 返回导入任务详情。
func (m *manager) GetImport(ctx context.Context, id uint64) (repoMod.ImportRecord, error) {
	return m.repo.GetImport(ctx, id)
}

// OpenImportErrors 校验导入任务后打开有界错误报告流。
func (m *manager) OpenImportErrors(ctx context.Context, id uint64) (io.ReadCloser, error) {
	imp, err := m.repo.GetImport(ctx, id)
	if err != nil {
		if isRepoNotFound(err) {
			return nil, ErrImportReportNotFound
		}
		return nil, fmt.Errorf("读取导入任务: %w", err)
	}
	expectedKey := fmt.Sprintf("moderation/imports/%d/errors.csv", imp.ID)
	if imp.ValidationStatus != repoMod.ImportStatusInvalid || imp.ErrorObjectKey == nil || *imp.ErrorObjectKey != expectedKey {
		return nil, ErrImportReportNotFound
	}
	if m.store == nil {
		return nil, errors.New("规则导入存储未初始化")
	}
	maxBytes := int64(m.cfg.MaxImportFileMB+1) * 1024 * 1024
	reader, err := m.store.OpenObject(ctx, expectedKey, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("打开导入错误报告: %w", err)
	}
	return reader, nil
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
