package moderationrule

import (
	"context"
	"fmt"
	"strings"

	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

// TestText 使用当前或候选规则集执行文本试跑，返回命中详情。
func (m *manager) TestText(ctx context.Context, cmd TestTextCommand) (TestResult, error) {
	if cmd.Text == "" {
		return TestResult{}, fmt.Errorf("%w: 试跑文本不能为空", ErrInvalidRule)
	}
	if len(cmd.Text) > 10000 {
		return TestResult{}, fmt.Errorf("%w: 试跑文本超过 10000 字符", ErrInvalidRule)
	}

	// 获取目标快照：指定候选时用缓存或加载，否则用当前快照。
	var snapshot *ruleindex.Snapshot
	var rulesetID uint64

	if cmd.RulesetID != nil && *cmd.RulesetID > 0 {
		rulesetID = *cmd.RulesetID
		snapshot = m.cache.Load(rulesetID)
		if snapshot == nil {
			return TestResult{}, ErrCandidateNotReady
		}
	} else {
		snapshot = m.currentSnapshot.Load()
		if snapshot == nil {
			return TestResult{}, ErrCandidateNotReady
		}
		rulesetID = snapshot.Version()
	}

	normalized := textnorm.Normalize(cmd.Text)
	matched := snapshot.Match(normalized)

	// 加载命中规则元数据。
	hits := make([]TestHit, 0, len(matched.RuleIDs)+len(matched.SuppressedIDs))
	allIDs := make([]uint64, 0, len(matched.RuleIDs)+len(matched.SuppressedIDs))
	allIDs = append(allIDs, matched.RuleIDs...)
	allIDs = append(allIDs, matched.SuppressedIDs...)

	if len(allIDs) > 0 {
		rules, err := m.repo.GetRulesByIDs(ctx, allIDs)
		if err != nil {
			return TestResult{}, fmt.Errorf("加载试跑命中规则: %w", err)
		}
		ruleMap := make(map[uint64]struct {
			RuleType  string
			Pattern   string
			Category  string
			RiskLevel string
			Effect    string
		}, len(rules))
		for _, r := range rules {
			ruleMap[r.ID] = struct {
				RuleType  string
				Pattern   string
				Category  string
				RiskLevel string
				Effect    string
			}{
				RuleType:  r.RuleType,
				Pattern:   r.Pattern,
				Category:  r.Category,
				RiskLevel: r.RiskLevel,
				Effect:    r.Effect,
			}
		}

		suppressedSet := make(map[uint64]bool, len(matched.SuppressedIDs))
		for _, id := range matched.SuppressedIDs {
			suppressedSet[id] = true
		}

		for _, id := range matched.RuleIDs {
			if info, ok := ruleMap[id]; ok {
				hits = append(hits, TestHit{
					RuleID:    id,
					RuleType:  info.RuleType,
					Pattern:   info.Pattern,
					Category:  info.Category,
					RiskLevel: info.RiskLevel,
					Effect:    info.Effect,
					Excerpt:   findExcerpt(info.RuleType, info.Pattern, normalized, cmd.Text),
				})
			}
		}
		for _, id := range matched.SuppressedIDs {
			if info, ok := ruleMap[id]; ok {
				hits = append(hits, TestHit{
					RuleID:    id,
					RuleType:  info.RuleType,
					Pattern:   info.Pattern,
					Category:  info.Category,
					RiskLevel: info.RiskLevel,
					Effect:    info.Effect,
					Excerpt:   findExcerpt(info.RuleType, info.Pattern, normalized, cmd.Text),
				})
			}
		}
	}

	risk := "low"
	switch matched.Risk {
	case ruleindex.RiskMedium:
		risk = "medium"
	case ruleindex.RiskHigh:
		risk = "high"
	}

	return TestResult{
		Risk:          risk,
		RulesetID:     rulesetID,
		RuleIDs:       matched.RuleIDs,
		SuppressedIDs: matched.SuppressedIDs,
		Truncated:     matched.Truncated,
		Hits:          hits,
	}, nil
}

// findExcerpt 在归一化文本中查找关键词位置，返回原始文本对应片段。
func findExcerpt(ruleType, pattern, normalized, original string) string {
	if ruleType == "keyword" {
		normalizedPattern := textnorm.Normalize(pattern)
		if idx := strings.Index(normalized, normalizedPattern); idx >= 0 {
			start := utf8IndexToRune(original, idx)
			end := utf8IndexToRune(original, idx+len(normalizedPattern))
			if start >= 0 && end <= len(original) {
				return original[start:end]
			}
		}
	}
	return pattern
}

// utf8IndexToRune 将归一化文本的字节索引近似映射回原始文本。
func utf8IndexToRune(s string, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	count := 0
	for i := range s {
		if count == runeIndex {
			return i
		}
		count++
	}
	return len(s)
}
