package ruleindex_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/service/moderation/ruleindex"
	"github.com/vpt/blog-backend/internal/service/moderation/textnorm"
)

func TestRepeatedBuildKeepsOnlyCurrentSnapshotReference(t *testing.T) {
	var current atomic.Pointer[ruleindex.Snapshot]
	for version := 1; version <= 20; version++ {
		current.Store(buildGeneratedSnapshot(t, uint64(version), 10000))
	}

	assert.Equal(t, uint64(20), current.Load().Version())
}

func BenchmarkBuild100K(b *testing.B) { benchmarkCapacityBuild(b, 100000) }

func BenchmarkBuild500K(b *testing.B) { benchmarkCapacityBuild(b, 500000) }

func BenchmarkMatchCapacity(b *testing.B) {
	rules := generatedRules(100000)
	snapshot := buildSnapshot(b, rules, defaultLimits())
	for _, length := range []int{800, 2000, 10000} {
		b.Run(fmt.Sprintf("%d/no_match", length), func(b *testing.B) {
			benchmarkCapacityMatch(b, snapshot, sizedNormalizedText("普通内容甲乙丙丁", length))
		})
		b.Run(fmt.Sprintf("%d/one_match", length), func(b *testing.B) {
			base := sizedNormalizedText("普通内容甲乙丙丁", length-8)
			benchmarkCapacityMatch(b, snapshot, base+textnorm.Normalize(rules[9999].Pattern))
		})
		b.Run(fmt.Sprintf("%d/many_matches", length), func(b *testing.B) {
			var patterns strings.Builder
			for index := 0; index < 256; index++ {
				patterns.WriteString(rules[index].Pattern)
				patterns.WriteByte(' ')
			}
			benchmarkCapacityMatch(b, snapshot, sizedNormalizedText(patterns.String(), length))
		})
	}
}

func benchmarkCapacityBuild(b *testing.B, count int) {
	rules := generatedRules(count)
	uniquePatterns := normalizedPatternCount(rules)
	source := sliceSource(rules)
	var snapshot *ruleindex.Snapshot
	var stats ruleindex.Stats
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var err error
		snapshot, stats, err = ruleindex.Build(context.Background(), 1, source, defaultLimits())
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(stats.RuleCount), "rules")
	b.ReportMetric(float64(uniquePatterns), "unique-patterns")
	b.ReportMetric(float64(count-uniquePatterns)*100/float64(count), "duplicate-%")
	b.ReportMetric(float64(stats.StateCount), "nodes")
	b.ReportMetric(float64(stats.EdgeCount), "edges")
	b.ReportMetric(float64(stats.DirectOutputCount), "outputs")
	b.ReportMetric(float64(snapshot.EncodedSize()), "artifact-B")
	b.ReportMetric(float64(stats.IndexBytes), "retained-B")
	b.ReportMetric(float64(stats.BuildPeakBytes), "peak-B")
}

func normalizedPatternCount(rules []ruleindex.SourceRule) int {
	unique := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		unique[textnorm.Normalize(rule.Pattern)] = struct{}{}
	}
	return len(unique)
}

func benchmarkCapacityMatch(b *testing.B, snapshot *ruleindex.Snapshot, text string) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for range b.N {
		snapshot.Match(text)
	}
}

func buildGeneratedSnapshot(t testing.TB, version uint64, count int) *ruleindex.Snapshot {
	t.Helper()
	snapshot, _, err := ruleindex.Build(context.Background(), version, sliceSource(generatedRules(count)), defaultLimits())
	require.NoError(t, err)
	return snapshot
}

func generatedRules(count int) []ruleindex.SourceRule {
	rules := make([]ruleindex.SourceRule, 0, count)
	for index := range count {
		pattern := fmt.Sprintf("风险词%c%c", rune(0x3400+index/20000), rune(0x4E00+index%20000))
		rules = append(rules, ruleindex.SourceRule{
			ID: uint64(index + 1), Type: "keyword", Pattern: pattern, Risk: "medium", Effect: "review",
		})
	}
	return rules
}

func sizedNormalizedText(seed string, runeCount int) string {
	if runeCount <= 0 {
		return ""
	}
	normalizedSeed := textnorm.Normalize(seed)
	var raw strings.Builder
	for raw.Len() < runeCount*3 {
		raw.WriteString(normalizedSeed)
	}
	runes := []rune(textnorm.Normalize(raw.String()))
	if len(runes) > runeCount {
		runes = runes[:runeCount]
	}
	return string(runes)
}
