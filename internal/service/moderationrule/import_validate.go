package moderationrule

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
)

// importValidationResult 是一次校验的统计结果。
type importValidationResult struct {
	totalRows     uint64
	validRows     uint64
	duplicateRows uint64
	errorRows     uint64
	errorKey      string
	rulesetID     *uint64
}

// validateImport 流式校验导入文件：解析、枚举校验、文件内去重、数据库去重。
// 错误边生成边写入临时 CSV 再流式上传，不保留全部错误在内存中。
func (m *manager) validateImport(ctx context.Context, imp repoMod.ImportRecord) (importValidationResult, error) {
	result := importValidationResult{}
	defaults := ImportDefaults{
		Category:  imp.DefaultCategory,
		Effect:    imp.DefaultEffect,
		RiskLevel: imp.DefaultRiskLevel,
		Priority:  imp.DefaultPriority,
	}

	// 打开对象流。
	reader, err := m.store.OpenObject(ctx, imp.ObjectKey, int64(imp.FileSize)+1024)
	if err != nil {
		return result, fmt.Errorf("打开导入文件: %w", err)
	}
	defer reader.Close()

	// 创建临时错误报告文件。
	errorPath := filepath.Join(os.TempDir(), fmt.Sprintf("moderation-import-errors-%d.csv", imp.ID))
	errorFile, err := os.Create(errorPath)
	if err != nil {
		return result, fmt.Errorf("创建错误报告文件: %w", err)
	}
	errorClosed := false
	defer func() {
		if !errorClosed {
			_ = errorFile.Close()
		}
		_ = os.Remove(errorPath)
	}()

	errorWriter := csv.NewWriter(errorFile)
	_ = errorWriter.Write([]string{"row", "field", "value", "error_code", "message"})

	// 文件内去重集合，最多保留 MaxImportRows 个摘要。
	fileHashes := make(map[repoMod.DedupeHash]int, 1024)
	var dbHashes []repoMod.DedupeHash

	// 获取当前规则集版本用于数据库去重。
	status, err := m.repo.CurrentStatus(ctx)
	if err != nil {
		return result, fmt.Errorf("读取规则集状态: %w", err)
	}

	// 解析并校验。
	var parseErrs []RowError
	var rows []ParsedRow
	if imp.Format == "csv" {
		rows, parseErrs = parseCSV(reader, defaults, m.cfg.MaxImportRows)
	} else {
		rows, parseErrs = parseTXT(reader, defaults, m.cfg.MaxImportRows)
	}

	// 写入解析错误。
	for _, e := range parseErrs {
		_ = errorWriter.Write([]string{fmt.Sprintf("%d", e.LineNumber), e.Field, e.Value, e.Code, e.Message})
		result.errorRows++
	}
	if len(parseErrs) > 0 {
		errorWriter.Flush()
		errorClosed = true
		_ = errorFile.Close()
		result.totalRows = uint64(len(rows) + len(parseErrs))
		err := m.uploadErrorReport(ctx, imp.ID, errorPath)
		if err != nil {
			return result, err
		}
		result.errorKey = fmt.Sprintf("moderation/imports/%d/errors.csv", imp.ID)
		return result, nil
	}

	// 逐行校验枚举和长度，收集有效行的摘要。
	for _, row := range rows {
		result.totalRows++
		rowErrs := validateParsedRow(m.cfg, row)
		for _, e := range rowErrs {
			_ = errorWriter.Write([]string{fmt.Sprintf("%d", e.LineNumber), e.Field, e.Value, e.Code, e.Message})
			result.errorRows++
		}
		if len(rowErrs) > 0 {
			continue
		}

		// 计算去重摘要。
		hash, _ := computeDedupeHash(row.Effect, row.RuleType, row.Pattern)
		if existingLine, exists := fileHashes[hash]; exists {
			_ = errorWriter.Write([]string{fmt.Sprintf("%d", row.LineNumber), "pattern", row.Pattern, "duplicate_in_file", fmt.Sprintf("与第 %d 行重复", existingLine)})
			result.duplicateRows++
			result.errorRows++
			continue
		}
		fileHashes[hash] = row.LineNumber
		dbHashes = append(dbHashes, hash)
		result.validRows++
	}

	// 分批查数据库重复。
	if len(dbHashes) > 0 {
		dupes, err := m.repo.FindDuplicateHashes(ctx, status.CurrentRulesetID, dbHashes)
		if err != nil {
			return result, fmt.Errorf("查询数据库重复: %w", err)
		}
		for hash, ruleID := range dupes {
			line := fileHashes[hash]
			_ = errorWriter.Write([]string{fmt.Sprintf("%d", line), "pattern", "", "duplicate_in_db", fmt.Sprintf("与已有规则 %d 重复", ruleID)})
			result.duplicateRows++
			result.errorRows++
			result.validRows--
		}
	}

	errorWriter.Flush()

	if result.errorRows > 0 {
		errorClosed = true
		_ = errorFile.Close()
		err := m.uploadErrorReport(ctx, imp.ID, errorPath)
		if err != nil {
			return result, err
		}
		result.errorKey = fmt.Sprintf("moderation/imports/%d/errors.csv", imp.ID)
		return result, nil
	}

	// 校验通过，创建候选规则集。
	candidate, err := m.createCandidateFromImport(ctx, imp, status.CurrentRulesetID)
	if err != nil {
		return result, err
	}

	result.rulesetID = &candidate.RulesetID
	return result, nil
}

// uploadErrorReport 上传错误报告 CSV 到对象存储。
func (m *manager) uploadErrorReport(ctx context.Context, importID uint64, errorPath string) error {
	readFile, err := os.Open(errorPath)
	if err != nil {
		return fmt.Errorf("打开错误报告上传: %w", err)
	}
	defer readFile.Close()

	stat, _ := readFile.Stat()
	errorKey := fmt.Sprintf("moderation/imports/%d/errors.csv", importID)
	if err := m.store.PutObjectStream(ctx, errorKey, readFile, stat.Size(), "text/csv; charset=utf-8"); err != nil {
		return fmt.Errorf("上传错误报告: %w", err)
	}
	return nil
}

// createCandidateFromImport 重新读取导入文件并创建候选规则集。
func (m *manager) createCandidateFromImport(ctx context.Context, imp repoMod.ImportRecord, baseRulesetID uint64) (repoMod.CandidateRecord, error) {
	reader, err := m.store.OpenObject(ctx, imp.ObjectKey, int64(imp.FileSize)+1024)
	if err != nil {
		return repoMod.CandidateRecord{}, fmt.Errorf("重新打开导入文件: %w", err)
	}
	defer reader.Close()

	defaults := ImportDefaults{
		Category:  imp.DefaultCategory,
		Effect:    imp.DefaultEffect,
		RiskLevel: imp.DefaultRiskLevel,
		Priority:  imp.DefaultPriority,
	}

	var rows []ParsedRow
	if imp.Format == "csv" {
		rows, _ = parseCSV(reader, defaults, m.cfg.MaxImportRows)
	} else {
		rows, _ = parseTXT(reader, defaults, m.cfg.MaxImportRows)
	}

	// 构建 RuleDraft 切片。
	drafts := make([]repoMod.RuleDraft, 0, len(rows))
	for _, row := range rows {
		hash, _ := computeDedupeHash(row.Effect, row.RuleType, row.Pattern)
		drafts = append(drafts, repoMod.RuleDraft{
			Name:       row.Name,
			RuleType:   row.RuleType,
			Pattern:    row.Pattern,
			DedupeHash: hash,
			Category:   row.Category,
			Effect:     row.Effect,
			RiskLevel:  row.RiskLevel,
			Priority:   row.Priority,
			SourceID:   imp.SourceID,
		})
	}

	if len(drafts) == 0 {
		return repoMod.CandidateRecord{}, ErrEmptyRuleset
	}

	return m.repo.CreateCandidate(ctx, repoMod.CreateCandidateCommand{
		BaseRulesetID: baseRulesetID,
		ActorID:       imp.OperatorID,
		Additions:     drafts,
	})
}

// normalizeImportFormat 归一化导入格式标识。
func normalizeImportFormat(format string) (string, error) {
	f := strings.ToLower(strings.TrimSpace(format))
	if f == "csv" || f == "txt" {
		return f, nil
	}
	return "", fmt.Errorf("不支持的导入格式 %q", format)
}

// validateImportInput 校验导入创建参数。
func validateImportInput(input CreateImportInput) error {
	if strings.TrimSpace(input.FileName) == "" {
		return fmt.Errorf("%w: 文件名不能为空", ErrInvalidRule)
	}
	if input.FileSize == 0 {
		return fmt.Errorf("%w: 文件大小不能为零", ErrInvalidRule)
	}
	if input.ObjectKey == "" {
		return fmt.Errorf("%w: 对象键不能为空", ErrInvalidRule)
	}
	name := strings.TrimSpace(input.SourceName)
	if name == "" {
		return fmt.Errorf("%w: 来源名称不能为空", ErrInvalidRule)
	}
	if utf8.RuneCountInString(name) > 100 {
		return fmt.Errorf("%w: 来源名称超过 100 字符", ErrInvalidRule)
	}
	if !isValidCategory(input.DefaultCategory) {
		return fmt.Errorf("%w: 缺省分类无效", ErrInvalidRule)
	}
	if !isValidEffect(input.DefaultEffect) {
		return fmt.Errorf("%w: 缺省规则效果无效", ErrInvalidRule)
	}
	if !isValidRiskLevel(input.DefaultRiskLevel) {
		return fmt.Errorf("%w: 缺省风险等级无效", ErrInvalidRule)
	}
	return nil
}
