package moderationrule

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
	"go.uber.org/zap"
)

// CreateRule 校验输入、检查重复、创建 building 候选并返回任务引用。
func (m *manager) CreateRule(ctx context.Context, cmd CreateRuleCommand) (Job, error) {
	if err := validateRuleInput(m.cfg, cmd.Rule); err != nil {
		return Job{}, err
	}
	status, err := m.repo.CurrentStatus(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("读取规则集状态: %w", err)
	}
	if cmd.ExpectedRulesetID != status.CurrentRulesetID {
		return Job{}, ErrRulesetConflict
	}

	hash, _ := computeDedupeHash(cmd.Rule.Effect, cmd.Rule.RuleType, cmd.Rule.Pattern)
	if err := m.checkDuplicate(ctx, status.CurrentRulesetID, hash); err != nil {
		return Job{}, err
	}

	draft := inputToDraft(cmd.Rule, hash)
	candidate, err := m.repo.CreateCandidate(ctx, repoMod.CreateCandidateCommand{
		BaseRulesetID: status.CurrentRulesetID,
		ActorID:       cmd.ActorID,
		Additions:     []repoMod.RuleDraft{draft},
	})
	if err != nil {
		return Job{}, fmt.Errorf("创建候选规则集: %w", err)
	}
	return Job{RulesetID: candidate.RulesetID, BaseRulesetID: candidate.BaseRulesetID, Status: candidate.Status}, nil
}

// ReplaceRule 创建替代规则事实和旧规则停用候选。
func (m *manager) ReplaceRule(ctx context.Context, cmd ReplaceRuleCommand) (Job, error) {
	if cmd.RuleID == 0 {
		return Job{}, fmt.Errorf("%w: 规则 ID 不能为空", ErrInvalidRule)
	}
	if err := validateRuleInput(m.cfg, cmd.Rule); err != nil {
		return Job{}, err
	}
	status, err := m.repo.CurrentStatus(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("读取规则集状态: %w", err)
	}
	if cmd.ExpectedRulesetID != status.CurrentRulesetID {
		return Job{}, ErrRulesetConflict
	}

	hash, _ := computeDedupeHash(cmd.Rule.Effect, cmd.Rule.RuleType, cmd.Rule.Pattern)
	if err := m.checkDuplicate(ctx, status.CurrentRulesetID, hash); err != nil {
		return Job{}, err
	}

	draft := inputToDraft(cmd.Rule, hash)
	candidate, err := m.repo.CreateCandidate(ctx, repoMod.CreateCandidateCommand{
		BaseRulesetID: status.CurrentRulesetID,
		ActorID:       cmd.ActorID,
		Additions:     []repoMod.RuleDraft{draft},
		RemoveRuleIDs: []uint64{cmd.RuleID},
	})
	if err != nil {
		return Job{}, fmt.Errorf("创建替代候选规则集: %w", err)
	}
	return Job{RulesetID: candidate.RulesetID, BaseRulesetID: candidate.BaseRulesetID, Status: candidate.Status}, nil
}

// BatchStatus 批量启用或停用规则并创建候选。
func (m *manager) BatchStatus(ctx context.Context, cmd BatchStatusCommand) (Job, error) {
	if len(cmd.RuleIDs) == 0 {
		return Job{}, fmt.Errorf("%w: 批量操作规则 ID 不能为空", ErrInvalidRule)
	}
	if len(cmd.RuleIDs) > 1000 {
		return Job{}, ErrBatchLimit
	}
	status, err := m.repo.CurrentStatus(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("读取规则集状态: %w", err)
	}
	if cmd.ExpectedRulesetID != status.CurrentRulesetID {
		return Job{}, ErrRulesetConflict
	}

	if !cmd.Active {
		// 停用：直接创建带 removal 的候选。
		candidate, err := m.repo.CreateCandidate(ctx, repoMod.CreateCandidateCommand{
			BaseRulesetID: status.CurrentRulesetID,
			ActorID:       cmd.ActorID,
			RemoveRuleIDs: cmd.RuleIDs,
		})
		if err != nil {
			return Job{}, fmt.Errorf("创建批量停用候选: %w", err)
		}
		return Job{RulesetID: candidate.RulesetID, BaseRulesetID: candidate.BaseRulesetID, Status: candidate.Status}, nil
	}

	// 启用：查询旧规则内容，创建内容相同的新规则事实。
	rules, err := m.repo.GetRulesByIDs(ctx, cmd.RuleIDs)
	if err != nil {
		return Job{}, fmt.Errorf("查询批量启用规则: %w", err)
	}
	if len(rules) == 0 {
		return Job{}, ErrRuleNotFound
	}
	additions := make([]repoMod.RuleDraft, 0, len(rules))
	for _, rule := range rules {
		hash, _ := computeDedupeHash(rule.Effect, rule.RuleType, rule.Pattern)
		additions = append(additions, repoMod.RuleDraft{
			Name:       rule.Name,
			RuleType:   rule.RuleType,
			Pattern:    rule.Pattern,
			DedupeHash: hash,
			Category:   rule.Category,
			Effect:     rule.Effect,
			RiskLevel:  rule.RiskLevel,
			Priority:   rule.Priority,
			SourceID:   rule.SourceID,
		})
	}
	candidate, err := m.repo.CreateCandidate(ctx, repoMod.CreateCandidateCommand{
		BaseRulesetID: status.CurrentRulesetID,
		ActorID:       cmd.ActorID,
		Additions:     additions,
	})
	if err != nil {
		return Job{}, fmt.Errorf("创建批量启用候选: %w", err)
	}
	return Job{RulesetID: candidate.RulesetID, BaseRulesetID: candidate.BaseRulesetID, Status: candidate.Status}, nil
}

// PublishCandidate 发布 ready 候选：先提交 DB 事务，再原子替换内存快照。
// 不在索引构建或对象下载期间持有发布互斥锁。
func (m *manager) PublishCandidate(ctx context.Context, rulesetID, expectedBase, actorID uint64) error {
	// 获取发布互斥锁，保证同一时刻最多一个候选进入发布。
	select {
	case m.publishMu <- struct{}{}:
		defer func() { <-m.publishMu }()
	default:
		return fmt.Errorf("另一个候选正在发布中")
	}

	// 先提交 DB 事务，确认版本一致性。
	if err := m.repo.PublishCandidate(ctx, rulesetID, expectedBase); err != nil {
		if errors.Is(err, repoMod.ErrRulesetConflict) {
			return ErrRulesetConflict
		}
		if errors.Is(err, repoMod.ErrCandidateNotFound) {
			return ErrCandidateNotFound
		}
		return fmt.Errorf("发布候选规则集: %w", err)
	}

	// 加载已发布的索引对象并原子替换快照。
	snapshot, err := m.loadPublishedSnapshot(ctx, rulesetID)
	if err != nil {
		m.logger.Error("发布后加载索引失败，当前快照不变",
			zap.Uint64("ruleset_id", rulesetID),
			zap.Error(err),
		)
		m.cache.Clear()
		return err
	}

	if err := m.replacer.ReplaceSnapshot(snapshot); err != nil {
		m.logger.Error("发布后替换快照失败",
			zap.Uint64("ruleset_id", rulesetID),
			zap.Error(err),
		)
		m.cache.Clear()
		return err
	}

	m.currentSnapshot.Store(snapshot)
	m.cache.Clear()
	return nil
}

// CancelCandidate 取消候选并清除缓存。
func (m *manager) CancelCandidate(ctx context.Context, rulesetID, actorID uint64) error {
	if err := m.repo.CancelCandidate(ctx, rulesetID, actorID); err != nil {
		if errors.Is(err, repoMod.ErrCandidateNotFound) {
			return ErrCandidateNotFound
		}
		return fmt.Errorf("取消候选规则集: %w", err)
	}
	m.cache.Clear()
	return nil
}

// checkDuplicate 检查摘要是否与当前活动规则重复。
func (m *manager) checkDuplicate(ctx context.Context, currentRulesetID uint64, hash repoMod.DedupeHash) error {
	dupes, err := m.repo.FindDuplicateHashes(ctx, currentRulesetID, []repoMod.DedupeHash{hash})
	if err != nil {
		return fmt.Errorf("检查规则重复: %w", err)
	}
	if len(dupes) > 0 {
		return fmt.Errorf("%w: 规则 ID %d", ErrDuplicateRule, dupes[hash])
	}
	return nil
}

// loadPublishedSnapshot 从对象存储加载索引文件并恢复快照。
func (m *manager) loadPublishedSnapshot(ctx context.Context, rulesetID uint64) (*ruleindex.Snapshot, error) {
	candidate, err := m.repo.GetCandidate(ctx, rulesetID)
	if err != nil {
		return nil, err
	}
	if candidate.IndexObjectKey == "" {
		return nil, fmt.Errorf("规则集 %d 缺少索引对象", rulesetID)
	}

	reader, err := m.store.OpenObject(ctx, candidate.IndexObjectKey, int64(candidate.IndexBytes)+1024)
	if err != nil {
		return nil, fmt.Errorf("打开索引对象: %w", err)
	}
	defer reader.Close()

	snapshot, _, err := ruleindex.ReadFrom(reader, m.limits)
	if err != nil {
		return nil, fmt.Errorf("读取索引文件: %w", err)
	}
	return snapshot, nil
}

// computeDedupeHash 按 effect + NUL + type + NUL + duplicateBasis 计算 SHA-256。
// keyword 的 basis 是归一化后的模式，regexp/composite 的 basis 是原始模式。
func computeDedupeHash(effect, ruleType, pattern string) (repoMod.DedupeHash, string) {
	var basis string
	if ruleType == "keyword" {
		basis = textnorm.Normalize(pattern)
	} else {
		basis = pattern
	}
	return sha256.Sum256([]byte(effect + "\x00" + ruleType + "\x00" + basis)), basis
}

func inputToDraft(input RuleInput, hash repoMod.DedupeHash) repoMod.RuleDraft {
	return repoMod.RuleDraft{
		Name:       input.Name,
		RuleType:   input.RuleType,
		Pattern:    input.Pattern,
		DedupeHash: hash,
		Category:   input.Category,
		Effect:     input.Effect,
		RiskLevel:  input.RiskLevel,
		Priority:   input.Priority,
		SourceID:   input.SourceID,
	}
}

// validateRuleInput 校验规则内容的安全边界。
func validateRuleInput(cfg ManagerConfig, input RuleInput) error {
	if input.Name != nil && utf8.RuneCountInString(strings.TrimSpace(*input.Name)) > 100 {
		return fmt.Errorf("%w: 名称超过 100 字符", ErrInvalidRule)
	}
	if err := validateRuleType(input.RuleType); err != nil {
		return err
	}
	if err := validateCategory(input.Category); err != nil {
		return err
	}
	if err := validateEffect(input.Effect, input.RuleType); err != nil {
		return err
	}
	if err := validateRiskLevel(input.RiskLevel); err != nil {
		return err
	}
	if input.Pattern == "" {
		return fmt.Errorf("%w: 模式不能为空", ErrInvalidRule)
	}
	if utf8.RuneCountInString(input.Pattern) > cfg.MaxPatternChars {
		return fmt.Errorf("%w: 模式超过 %d 字符", ErrInvalidRule, cfg.MaxPatternChars)
	}
	if input.SourceID == 0 {
		return fmt.Errorf("%w: 来源 ID 不能为空", ErrInvalidRule)
	}
	if input.RuleType == "regexp" {
		if _, err := textnorm.CompileRegexp(input.Pattern); err != nil {
			return fmt.Errorf("%w: 正则编译失败: %s", ErrInvalidRule, err.Error())
		}
	}
	if input.RuleType == "composite" {
		if err := validateComposite(input.Pattern, cfg.MaxPatternChars); err != nil {
			return err
		}
	}
	return nil
}

func validateRuleType(value string) error {
	switch value {
	case "keyword", "regexp", "composite":
		return nil
	default:
		return fmt.Errorf("%w: 规则类型无效 %q", ErrInvalidRule, value)
	}
}

func validateCategory(value string) error {
	for _, cat := range moderationCategories() {
		if cat.Key == value {
			return nil
		}
	}
	return fmt.Errorf("%w: 分类无效 %q", ErrInvalidRule, value)
}

func validateEffect(effect, ruleType string) error {
	switch effect {
	case "review":
		return nil
	case "allow":
		if ruleType != "keyword" {
			return fmt.Errorf("%w: allow 仅支持关键词规则", ErrInvalidRule)
		}
		return nil
	default:
		return fmt.Errorf("%w: 规则效果无效 %q", ErrInvalidRule, effect)
	}
}

func validateRiskLevel(value string) error {
	switch value {
	case "low", "medium", "high":
		return nil
	default:
		return fmt.Errorf("%w: 风险等级无效 %q", ErrInvalidRule, value)
	}
}

func validateComposite(pattern string, maxRunes int) error {
	parts := strings.Split(pattern, "&&")
	if len(parts) < 2 {
		return fmt.Errorf("%w: 组合规则至少需要两个 && 信号", ErrInvalidRule)
	}
	for _, part := range parts {
		normalized := textnorm.Normalize(part)
		if utf8.RuneCountInString(normalized) == 0 {
			return fmt.Errorf("%w: 组合信号归一化后不能为空", ErrInvalidRule)
		}
		if utf8.RuneCountInString(normalized) > maxRunes {
			return fmt.Errorf("%w: 组合信号超过长度上限", ErrInvalidRule)
		}
	}
	return nil
}
