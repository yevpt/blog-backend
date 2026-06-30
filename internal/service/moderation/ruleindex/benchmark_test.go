package ruleindex_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

func BenchmarkBuild(b *testing.B) {
	rules := benchmarkRules(10000)
	source := sliceSource(rules)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := ruleindex.Build(context.Background(), 1, source, defaultLimits()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMatch(b *testing.B) {
	snapshot := buildSnapshot(b, benchmarkRules(10000), defaultLimits())
	text := textnorm.Normalize("普通文本包含风险词9999和少量其他内容")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		snapshot.Match(text)
	}
}

func benchmarkRules(count int) []ruleindex.SourceRule {
	rules := make([]ruleindex.SourceRule, 0, count)
	for index := range count {
		rules = append(rules, ruleindex.SourceRule{
			ID: uint64(index + 1), Type: "keyword", Pattern: fmt.Sprintf("风险词%d", index), Risk: "medium", Effect: "review",
		})
	}
	return rules
}
