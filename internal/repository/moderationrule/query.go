package moderationrule

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

const (
	// defaultListLimit 是游标分页的默认页大小。
	defaultListLimit = 20
	// maxListLimit 是游标分页的最大页大小。
	maxListLimit = 100
	// duplicateHashChunkSize 限制单次 IN 查询的参数数量，避免超过 MySQL 占位符上限。
	duplicateHashChunkSize = 1000
)

func (r *repository) ListRules(ctx context.Context, filter RuleFilter) (RulePage, error) {
	if r == nil || r.db == nil {
		return RulePage{}, errors.New("规则管理仓库未初始化")
	}
	limit := normalizeListLimit(filter.Limit)

	// Active 过滤需要当前已发布版本，仅在筛选时查询以避免无谓开销。
	var currentVersion uint64
	if filter.Active != nil {
		version, err := r.currentPublishedVersion(ctx)
		if err != nil {
			return RulePage{}, err
		}
		currentVersion = version
	}

	query := r.db.WithContext(ctx).
		Table("moderation_rule AS rule").
		Select(
			"rule.id", "rule.name", "rule.rule_type", "rule.pattern", "rule.category",
			"rule.effect", "rule.risk_level", "rule.priority", "rule.source_id",
			"rule.activated_ruleset_id", "rule.deactivated_ruleset_id", "rule.replaces_rule_id",
			"rule.created_at", "rule.updated_at",
		)

	// ExactID 精确查询优先于游标。
	if filter.ExactID > 0 {
		query = query.Where("id = ?", filter.ExactID)
	} else if filter.AfterID > 0 {
		query = query.Where("id > ?", filter.AfterID)
	}

	// ExactPattern 和 PatternPrefix 互斥，精确优先。
	if filter.ExactPattern != "" {
		query = query.Where("pattern = ?", filter.ExactPattern)
	} else if filter.PatternPrefix != "" {
		query = query.Where("pattern LIKE ?", filter.PatternPrefix+"%")
	}

	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.RuleType != "" {
		query = query.Where("rule_type = ?", filter.RuleType)
	}
	if filter.RiskLevel != "" {
		query = query.Where("risk_level = ?", filter.RiskLevel)
	}
	if filter.Effect != "" {
		query = query.Where("effect = ?", filter.Effect)
	}
	if filter.SourceID > 0 {
		query = query.Where("source_id = ?", filter.SourceID)
	}

	if filter.Active != nil {
		if *filter.Active {
			query = query.Where("activated_ruleset_id <= ?", currentVersion).
				Where("deactivated_ruleset_id IS NULL")
		} else {
			query = query.Where("deactivated_ruleset_id <= ?", currentVersion)
		}
	}

	query = query.Order("id ASC").Limit(limit + 1)

	rows, err := query.Rows()
	if err != nil {
		return RulePage{}, fmt.Errorf("查询规则列表: %w", err)
	}
	defer rows.Close()

	rules := make([]RuleListRecord, 0, limit+1)
	for rows.Next() {
		var rec RuleListRecord
		if err := rows.Scan(
			&rec.ID, &rec.Name, &rec.RuleType, &rec.Pattern, &rec.Category,
			&rec.Effect, &rec.RiskLevel, &rec.Priority, &rec.SourceID,
			&rec.ActivatedRulesetID, &rec.DeactivatedRulesetID, &rec.ReplacesRuleID,
			&rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return RulePage{}, fmt.Errorf("扫描规则行: %w", err)
		}
		if filter.Active != nil {
			rec.Active = *filter.Active
		}
		rules = append(rules, rec)
	}
	if err := rows.Err(); err != nil {
		return RulePage{}, fmt.Errorf("读取规则行流: %w", err)
	}

	page := RulePage{Rules: rules}
	// 取 limit+1 行，最后一行作为游标哨兵，不返回给调用方。
	if len(rules) > limit {
		page.HasMore = true
		page.NextCursor = rules[limit].ID
		page.Rules = rules[:limit]
	}
	return page, nil
}

func (r *repository) ListSources(ctx context.Context) ([]SourceRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("规则管理仓库未初始化")
	}
	rows, err := r.db.WithContext(ctx).
		Table("moderation_rule_source").
		Select("id", "name", "created_at").
		Order("id ASC").
		Rows()
	if err != nil {
		return nil, fmt.Errorf("查询规则来源: %w", err)
	}
	defer rows.Close()

	sources := make([]SourceRecord, 0)
	for rows.Next() {
		var rec SourceRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描规则来源行: %w", err)
		}
		sources = append(sources, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取规则来源流: %w", err)
	}
	return sources, nil
}

func (r *repository) EnsureSource(ctx context.Context, name string) (SourceRecord, error) {
	if r == nil || r.db == nil {
		return SourceRecord{}, errors.New("规则管理仓库未初始化")
	}
	if strings.TrimSpace(name) == "" {
		return SourceRecord{}, errors.New("来源名称不能为空")
	}

	// 先查已有来源，命中则直接返回，避免重复插入。
	var existing model.ModerationRuleSource
	err := r.db.WithContext(ctx).
		Table("moderation_rule_source").
		Select("id", "name", "created_at").
		Where("name = ?", name).
		Order("id ASC").
		Limit(1).
		Take(&existing).Error
	if err == nil {
		return SourceRecord{ID: existing.ID, Name: existing.Name, CreatedAt: existing.CreatedAt}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return SourceRecord{}, fmt.Errorf("查询规则来源: %w", err)
	}

	// 未命中则创建新来源。
	now := time.Now()
	source := model.ModerationRuleSource{Name: name, CreatedAt: now, UpdatedAt: now}
	if err := r.db.WithContext(ctx).Create(&source).Error; err != nil {
		return SourceRecord{}, fmt.Errorf("创建规则来源: %w", err)
	}
	return SourceRecord{ID: source.ID, Name: source.Name, CreatedAt: source.CreatedAt}, nil
}

func (r *repository) FindDuplicateHashes(ctx context.Context, currentRulesetID uint64, hashes []DedupeHash) (map[DedupeHash]uint64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("规则管理仓库未初始化")
	}
	if len(hashes) == 0 {
		return nil, nil
	}

	result := make(map[DedupeHash]uint64, len(hashes))
	for start := 0; start < len(hashes); start += duplicateHashChunkSize {
		end := start + duplicateHashChunkSize
		if end > len(hashes) {
			end = len(hashes)
		}
		chunk := hashes[start:end]

		// 转为 [][]byte 让 GORM 按切片展开 IN 占位符，每个 []byte 作为单个二进制值。
		byteSlices := make([][]byte, len(chunk))
		for i, h := range chunk {
			byteSlices[i] = h[:]
		}

		rows, err := r.db.WithContext(ctx).
			Table("moderation_rule").
			Select("dedupe_hash", "id").
			Where("dedupe_hash IN ?", byteSlices).
			Where("activated_ruleset_id <= ?", currentRulesetID).
			Where("deactivated_ruleset_id IS NULL OR deactivated_ruleset_id > ?", currentRulesetID).
			Rows()
		if err != nil {
			return nil, fmt.Errorf("查询重复摘要: %w", err)
		}
		for rows.Next() {
			var hashBytes []byte
			var ruleID uint64
			if err := rows.Scan(&hashBytes, &ruleID); err != nil {
				rows.Close()
				return nil, fmt.Errorf("扫描重复摘要: %w", err)
			}
			var hash DedupeHash
			copy(hash[:], hashBytes)
			result[hash] = ruleID
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("读取重复摘要流: %w", err)
		}
	}
	return result, nil
}

func (r *repository) CurrentStatus(ctx context.Context) (StatusRecord, error) {
	if r == nil || r.db == nil {
		return StatusRecord{}, errors.New("规则管理仓库未初始化")
	}

	// 查询当前已发布规则集的统计摘要。
	var published struct {
		ID              uint64
		RuleCount       uint64
		KeywordCount    uint64
		RegexpCount     uint64
		CompositeCount  uint64
		IndexBytes      uint64
		BuildPeakBytes  uint64
		BuildDurationMS uint64
		IndexObjectKey  *string
		IndexSHA256     *string
		UpdatedAt       time.Time
	}
	err := r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Select("id", "rule_count", "keyword_count", "regexp_count", "composite_count",
			"index_bytes", "build_peak_bytes", "build_duration_ms", "index_object_key",
			"index_sha256", "updated_at").
		Where("status = ?", "published").
		Order("id DESC").
		Limit(1).
		Take(&published).Error
	if err != nil {
		return StatusRecord{}, fmt.Errorf("读取当前规则集状态: %w", err)
	}

	status := StatusRecord{
		CurrentRulesetID: published.ID,
		RuleCount:        published.RuleCount,
		KeywordCount:     published.KeywordCount,
		RegexpCount:      published.RegexpCount,
		CompositeCount:   published.CompositeCount,
		IndexBytes:       published.IndexBytes,
		BuildPeakBytes:   published.BuildPeakBytes,
		BuildDurationMS:  published.BuildDurationMS,
		IndexObjectKey:   published.IndexObjectKey,
		IndexSHA256:      published.IndexSHA256,
		UpdatedAt:        published.UpdatedAt,
	}

	// 查询当前候选规则集（building 或 ready），没有时返回 nil。
	var candidate struct {
		ID              uint64
		Status          string
		BaseRulesetID   uint64
		RuleCount       uint64
		KeywordCount    uint64
		RegexpCount     uint64
		CompositeCount  uint64
		IndexBytes      uint64
		BuildPeakBytes  uint64
		FailureCode     *string
		CreatedAt       time.Time
		UpdatedAt       time.Time
	}
	err = r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Select("id", "status", "base_ruleset_id", "rule_count", "keyword_count",
			"regexp_count", "composite_count", "index_bytes", "build_peak_bytes",
			"failure_code", "created_at", "updated_at").
		Where("status IN ?", []string{"building", "ready"}).
		Order("id DESC").
		Limit(1).
		Take(&candidate).Error
	if err == nil {
		status.Candidate = &CandidateRecord{
			RulesetID:      candidate.ID,
			Status:         candidate.Status,
			BaseRulesetID:  candidate.BaseRulesetID,
			RuleCount:      candidate.RuleCount,
			KeywordCount:   candidate.KeywordCount,
			RegexpCount:    candidate.RegexpCount,
			CompositeCount: candidate.CompositeCount,
			IndexBytes:     candidate.IndexBytes,
			BuildPeakBytes: candidate.BuildPeakBytes,
			FailureCode:    candidate.FailureCode,
			CreatedAt:      candidate.CreatedAt,
			UpdatedAt:      candidate.UpdatedAt,
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return StatusRecord{}, fmt.Errorf("读取候选规则集状态: %w", err)
	}

	return status, nil
}

func (r *repository) currentPublishedVersion(ctx context.Context) (uint64, error) {
	var result struct{ ID uint64 }
	err := r.db.WithContext(ctx).
		Table("moderation_ruleset").
		Select("id").
		Where("status = ?", "published").
		Order("id DESC").
		Limit(1).
		Take(&result).Error
	if err != nil {
		return 0, fmt.Errorf("读取当前已发布版本: %w", err)
	}
	return result.ID, nil
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}

// GetRulesByIDs 批量查询指定 ID 的规则记录，顺序不保证。
func (r *repository) GetRulesByIDs(ctx context.Context, ids []uint64) ([]RuleListRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("规则管理仓库未初始化")
	}
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := r.db.WithContext(ctx).
		Table("moderation_rule AS rule").
		Select(
			"rule.id", "rule.name", "rule.rule_type", "rule.pattern", "rule.category",
			"rule.effect", "rule.risk_level", "rule.priority", "rule.source_id",
			"rule.activated_ruleset_id", "rule.deactivated_ruleset_id", "rule.replaces_rule_id",
			"rule.created_at", "rule.updated_at",
		).
		Where("id IN ?", ids).
		Order("id ASC").
		Rows()
	if err != nil {
		return nil, fmt.Errorf("批量查询规则: %w", err)
	}
	defer rows.Close()

	rules := make([]RuleListRecord, 0, len(ids))
	for rows.Next() {
		var rec RuleListRecord
		if err := rows.Scan(
			&rec.ID, &rec.Name, &rec.RuleType, &rec.Pattern, &rec.Category,
			&rec.Effect, &rec.RiskLevel, &rec.Priority, &rec.SourceID,
			&rec.ActivatedRulesetID, &rec.DeactivatedRulesetID, &rec.ReplacesRuleID,
			&rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描规则行: %w", err)
		}
		rules = append(rules, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取规则行流: %w", err)
	}
	return rules, nil
}
