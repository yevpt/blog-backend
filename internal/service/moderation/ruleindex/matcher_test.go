package ruleindex_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

func TestMatchAppliesAllowAndKeepsHighestRiskAfterIDCap(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "敏感", Risk: "high", Effect: "review"},
		{ID: 2, Type: "keyword", Pattern: "非敏感示例", Risk: "low", Effect: "allow"},
		{ID: 3, Type: "keyword", Pattern: "风险", Risk: "medium", Effect: "review"},
	}
	snapshot := buildSnapshot(t, rules, ruleindex.Limits{MaxMatchIDs: 1, MaxPatternRunes: 500})

	got := snapshot.Match(textnorm.Normalize("非敏感示例 风险"))

	assert.Equal(t, ruleindex.RiskMedium, got.Risk)
	assert.Equal(t, []uint64{3}, got.RuleIDs)
	assert.Equal(t, []uint64{1}, got.SuppressedIDs)
	assert.False(t, got.Truncated)
}

func TestMatchTruncationDoesNotLowerRisk(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 10, Type: "keyword", Pattern: "普通", Risk: "low", Effect: "review", Priority: 10},
		{ID: 20, Type: "keyword", Pattern: "风险", Risk: "medium", Effect: "review", Priority: 20},
		{ID: 30, Type: "keyword", Pattern: "违禁", Risk: "high", Effect: "review", Priority: 30},
	}
	snapshot := buildSnapshot(t, rules, ruleindex.Limits{MaxMatchIDs: 1, MaxPatternRunes: 500})

	got := snapshot.Match(textnorm.Normalize("普通 风险 违禁"))

	assert.Equal(t, ruleindex.RiskHigh, got.Risk)
	assert.Equal(t, []uint64{30}, got.RuleIDs)
	assert.True(t, got.Truncated)
}

func TestMatchOrdersIDsByRiskPriorityAndID(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 30, Type: "keyword", Pattern: "丙", Risk: "medium", Effect: "review", Priority: 20},
		{ID: 20, Type: "keyword", Pattern: "乙", Risk: "high", Effect: "review", Priority: 30},
		{ID: 10, Type: "keyword", Pattern: "甲", Risk: "high", Effect: "review", Priority: 10},
		{ID: 5, Type: "keyword", Pattern: "丁", Risk: "high", Effect: "review", Priority: 10},
	}
	snapshot := buildSnapshot(t, rules, defaultLimits())

	got := snapshot.Match(textnorm.Normalize("甲乙丙丁"))

	assert.Equal(t, []uint64{5, 10, 20, 30}, got.RuleIDs)
}

func TestMatchAllowOnlySuppressesFullyCoveredKeyword(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "敏感", Risk: "high", Effect: "review"},
		{ID: 2, Type: "keyword", Pattern: "非敏感示例", Risk: "low", Effect: "allow"},
	}
	snapshot := buildSnapshot(t, rules, defaultLimits())

	got := snapshot.Match(textnorm.Normalize("非敏感示例后仍有敏感"))

	assert.Equal(t, ruleindex.RiskHigh, got.Risk)
	assert.Equal(t, []uint64{1}, got.RuleIDs)
	assert.Equal(t, []uint64{1}, got.SuppressedIDs)
}

func TestMatchRunsNormalizedRegexpAndCompositeRulesOutsideAllow(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "违禁", Risk: "high", Effect: "review"},
		{ID: 2, Type: "keyword", Pattern: "非违禁示例", Risk: "low", Effect: "allow"},
		{ID: 3, Type: "regexp", Pattern: `VX\d+`, Risk: "medium", Effect: "review"},
		{ID: 4, Type: "composite", Pattern: "扫码 && 入群", Risk: "high", Effect: "review"},
	}
	snapshot := buildSnapshot(t, rules, defaultLimits())

	got := snapshot.Match(textnorm.Normalize("非違禁示例 vx123 请掃碼后入群"))

	assert.Equal(t, ruleindex.RiskHigh, got.Risk)
	assert.Equal(t, []uint64{4, 3}, got.RuleIDs)
	assert.Equal(t, []uint64{1}, got.SuppressedIDs)
}

func TestMatchManyRulesKeepsAllocationsBounded(t *testing.T) {
	rules := make([]ruleindex.SourceRule, 0, 256)
	var text strings.Builder
	for index := range 256 {
		pattern := fmt.Sprintf("匹配词%d", index)
		rules = append(rules, ruleindex.SourceRule{
			ID: uint64(index + 1), Type: "keyword", Pattern: pattern, Risk: "medium", Effect: "review",
		})
		text.WriteString(pattern)
		text.WriteByte(' ')
	}
	snapshot := buildSnapshot(t, rules, defaultLimits())
	normalized := textnorm.Normalize(text.String())

	allocations := testing.AllocsPerRun(20, func() {
		snapshot.Match(normalized)
	})

	assert.LessOrEqual(t, allocations, float64(10))
}
