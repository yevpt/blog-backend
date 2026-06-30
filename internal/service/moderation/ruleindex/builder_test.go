package ruleindex_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
)

func TestBuilderDoesNotExpandFailureOutputs(t *testing.T) {
	rules := nestedRules(500)
	snapshot := buildSnapshot(t, rules, defaultLimits())

	assert.Equal(t, len(rules), snapshot.Stats().DirectOutputCount)
}

func TestBuilderRejectsKeywordCapacityBeforeAcceptingExtraRule(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "甲", Risk: "medium", Effect: "review"},
		{ID: 2, Type: "keyword", Pattern: "乙", Risk: "medium", Effect: "review"},
	}
	limits := defaultLimits()
	limits.MaxKeywordRules = 1

	_, _, err := ruleindex.Build(context.Background(), 1, sliceSource(rules), limits)

	assert.ErrorIs(t, err, ruleindex.ErrIndexLimit)
}

func TestBuilderRejectsAllowForNonKeywordRule(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 1, Type: "regexp", Pattern: "风险", Risk: "medium", Effect: "allow"},
	}

	_, _, err := ruleindex.Build(context.Background(), 1, sliceSource(rules), defaultLimits())

	require.Error(t, err)
	assert.ErrorContains(t, err, "allow")
}

func TestBuilderRejectsPatternLengthAsIndexLimit(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 1, Type: "keyword", Pattern: "超长", Risk: "medium", Effect: "review"},
	}
	limits := defaultLimits()
	limits.MaxPatternRunes = 1

	_, _, err := ruleindex.Build(context.Background(), 1, sliceSource(rules), limits)

	assert.ErrorIs(t, err, ruleindex.ErrIndexLimit)
}

func TestBuilderBoundsCompositeRulesWithNonKeywordBudget(t *testing.T) {
	rules := []ruleindex.SourceRule{
		{ID: 1, Type: "regexp", Pattern: "风险", Risk: "medium", Effect: "review"},
		{ID: 2, Type: "composite", Pattern: "扫码&&入群", Risk: "high", Effect: "review"},
	}
	limits := defaultLimits()
	limits.MaxRegexpRules = 1

	_, _, err := ruleindex.Build(context.Background(), 1, sliceSource(rules), limits)

	assert.ErrorIs(t, err, ruleindex.ErrIndexLimit)
}

func TestBuilderRejectsLimitsAboveGlobalCapacity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ruleindex.Limits)
	}{
		{name: "keywords", mutate: func(l *ruleindex.Limits) { l.MaxKeywordRules = 500001 }},
		{name: "non-keywords", mutate: func(l *ruleindex.Limits) { l.MaxRegexpRules = 501 }},
		{name: "match IDs", mutate: func(l *ruleindex.Limits) { l.MaxMatchIDs = 129 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := defaultLimits()
			tt.mutate(&limits)

			_, _, err := ruleindex.Build(context.Background(), 1, sliceSource([]ruleindex.SourceRule{
				{ID: 1, Type: "keyword", Pattern: "甲", Risk: "medium", Effect: "review"},
			}), limits)

			assert.ErrorIs(t, err, ruleindex.ErrIndexLimit)
		})
	}
}

func buildSnapshot(t *testing.T, rules []ruleindex.SourceRule, limits ruleindex.Limits) *ruleindex.Snapshot {
	t.Helper()
	snapshot, _, err := ruleindex.Build(context.Background(), 1, sliceSource(rules), limits)
	require.NoError(t, err)
	return snapshot
}

func sliceSource(rules []ruleindex.SourceRule) ruleindex.Source {
	return func(ctx context.Context, visit func(ruleindex.SourceRule) error) error {
		for _, rule := range rules {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := visit(rule); err != nil {
				return err
			}
		}
		return nil
	}
}

func defaultLimits() ruleindex.Limits {
	return ruleindex.Limits{
		MaxKeywordRules: 500000,
		MaxRegexpRules:  200,
		MaxPatternRunes: 500,
		MaxMatchIDs:     128,
	}
}

func nestedRules(count int) []ruleindex.SourceRule {
	rules := make([]ruleindex.SourceRule, 0, count)
	var pattern strings.Builder
	for i := 0; i < count; i++ {
		pattern.WriteRune(rune(0x4E00 + i))
		rules = append(rules, ruleindex.SourceRule{
			ID: uint64(i + 1), Type: "keyword", Pattern: pattern.String(), Risk: "medium", Effect: "review",
		})
	}
	return rules
}
