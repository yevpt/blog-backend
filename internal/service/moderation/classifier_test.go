package moderation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	moderationrepo "github.com/vpt/blog-backend/internal/repository/moderation"
	repositorymock "github.com/vpt/blog-backend/internal/repository/moderation/mock"
	"github.com/vpt/blog-backend/internal/service/moderation"
)

func TestNewClassifierFromRepositoryLoadsCurrentRuleSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymock.NewMockRepository(ctrl)
	repo.EXPECT().LoadEnabledRules(gomock.Any()).Return([]moderationrepo.RuleRecord{
		{ID: 1, RuleType: "keyword", Pattern: "风险词", RiskLevel: moderationrepo.RiskMedium, RulesetVersion: 3},
	}, nil)

	classifier, err := moderation.NewClassifierFromRepository(context.Background(), repo, zap.NewNop())

	require.NoError(t, err)
	got := classifier.Classify(processed("命中风险词"))
	assert.Equal(t, moderation.RiskMedium, got.Risk)
	assert.Equal(t, uint64(3), got.RulesetVersion)
}

func TestSanitizeRemovesExecutableHTML(t *testing.T) {
	processor := moderation.NewContentProcessor()
	raw := `<p onclick="alert(1)">正文<script>alert(2)</script></p>` +
		`<a href="javascript:alert(3)" onmouseover="alert(4)">危险链接</a>` +
		`<a href="https://example.com/path">安全链接</a>`

	got, err := processor.Process(raw, 100)

	require.NoError(t, err)
	assert.NotContains(t, got.Published, "script")
	assert.NotContains(t, got.Published, "onclick")
	assert.NotContains(t, got.Published, "onmouseover")
	assert.NotContains(t, got.Published, "javascript:")
	assert.Contains(t, got.Published, `<p>正文</p>`)
	assert.Contains(t, got.Published, `href="https://example.com/path"`)
	assert.Equal(t, "正文危险链接安全链接", got.PlainText)
	assert.Equal(t, []string{"https://example.com/path"}, got.Links)
}

func TestSanitizeRejectsContentPastPlainTextLimit(t *testing.T) {
	processor := moderation.NewContentProcessor()

	_, err := processor.Process("<p>一二三四</p>", 3)

	assert.ErrorIs(t, err, moderation.ErrContentTooLong)
}

func TestNormalizeCanonicalizesClassificationText(t *testing.T) {
	raw := " ＡＢＣ\u200b 測試！！！加---微微微 "

	got := moderation.NormalizeText(raw)

	assert.Equal(t, "abc测试加微", got)
}

func TestNormalizeRemovesDefaultIgnorableEvasionCharacters(t *testing.T) {
	tests := []struct {
		name      string
		separator string
	}{
		{name: "combining grapheme joiner", separator: "\u034f"},
		{name: "BMP variation selector", separator: "\ufe0f"},
		{name: "supplementary variation selector", separator: "\U000E0100"},
		{name: "zero width format", separator: "\u200b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := moderation.NormalizeText("违" + tt.separator + "禁")

			assert.Equal(t, "违禁", got)
		})
	}
}

func TestClassifierMatchesKeywordRegexAndCompositeRules(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshot(
		rule(1, moderation.RuleKeyword, moderation.RiskMedium, "加 微"),
		rule(2, moderation.RuleRegexp, moderation.RiskMedium, `vx\d{3}`),
		rule(3, moderation.RuleComposite, moderation.RiskHigh, "扫码 && 入群"),
	))

	got := classifier.Classify(processed("加微，vx123；请掃碼后入群"))

	assert.Equal(t, moderation.RiskHigh, got.Risk)
	assert.ElementsMatch(t, []uint64{1, 2, 3}, got.RuleMatchIDs)
}

func TestClassifierNormalizesRegexpLiteralRunes(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshot(
		rule(1, moderation.RuleRegexp, moderation.RiskHigh, `違禁`),
		rule(2, moderation.RuleRegexp, moderation.RiskMedium, `VX\d+`),
	))

	got := classifier.Classify(processed("违禁 vx123"))

	assert.Equal(t, moderation.RiskHigh, got.Risk)
	assert.ElementsMatch(t, []uint64{1, 2}, got.RuleMatchIDs)
}

func TestClassifierPreservesRegexpEscapesClassesAndUnicodeProperties(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshot(
		rule(1, moderation.RuleRegexp, moderation.RiskHigh, `\A\x56\x58\d+[a-z]\p{Han}\z`),
	))

	got := classifier.Classify(processed("VX123q禁"))

	assert.Equal(t, moderation.RiskHigh, got.Risk)
	assert.Equal(t, []uint64{1}, got.RuleMatchIDs)
}

func TestClassifierRemovesDefaultIgnorablesFromRegexpLiteralAndContent(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshot(
		rule(1, moderation.RuleRegexp, moderation.RiskHigh, "違\u034f禁"),
	))

	got := classifier.Classify(processed("违\ufe0f\U000E0100禁"))

	assert.Equal(t, moderation.RiskHigh, got.Risk)
	assert.Equal(t, []uint64{1}, got.RuleMatchIDs)
}

func TestClassifierUsesHighestRiskAndExactMatchedRuleIDs(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshot(
		rule(10, moderation.RuleKeyword, moderation.RiskLow, "普通"),
		rule(20, moderation.RuleKeyword, moderation.RiskMedium, "加微"),
		rule(30, moderation.RuleKeyword, moderation.RiskHigh, "违禁"),
		rule(40, moderation.RuleKeyword, moderation.RiskHigh, "未命中"),
	))

	got := classifier.Classify(processed("普通，加 微，违\u200b禁"))

	assert.Equal(t, moderation.RiskHigh, got.Risk)
	assert.ElementsMatch(t, []uint64{10, 20, 30}, got.RuleMatchIDs)
}

func TestClassifierRejectsInvalidRegexSnapshot(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshot(
		rule(1, moderation.RuleKeyword, moderation.RiskLow, "安全"),
	))

	err := classifier.ReplaceSnapshot(snapshotVersion(2,
		rule(2, moderation.RuleRegexp, moderation.RiskHigh, "("),
	))

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid RE2 pattern")
	assert.Equal(t, moderation.RiskLow, classifier.Classify(processed("安全")).Risk)
}

func TestClassifierEmptyColdSnapshotDegradesToMedium(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), moderation.RuleSnapshot{})

	got := classifier.Classify(processed("普通内容"))

	assert.Equal(t, moderation.RiskMedium, got.Risk)
	assert.Empty(t, got.RuleMatchIDs)
}

func TestClassifierInvalidColdSnapshotDegradesToMedium(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshot(
		rule(1, moderation.RuleRegexp, moderation.RiskHigh, "("),
	))

	got := classifier.Classify(processed("普通内容"))

	assert.Equal(t, moderation.RiskMedium, got.Risk)
	assert.Empty(t, got.RuleMatchIDs)
}

func TestClassifierKeepsImmutableLastGoodSnapshot(t *testing.T) {
	rules := []moderation.CompiledRule{
		rule(1, moderation.RuleKeyword, moderation.RiskHigh, "原始词"),
	}
	classifier := moderation.NewClassifier(zap.NewNop(), moderation.RuleSnapshot{Version: 1, Rules: rules})
	rules[0].Pattern = "篡改词"

	err := classifier.ReplaceSnapshot(snapshotVersion(2,
		rule(2, moderation.RuleRegexp, moderation.RiskHigh, "("),
	))

	require.Error(t, err)
	assert.Equal(t, moderation.RiskHigh, classifier.Classify(processed("原始詞")).Risk)
	assert.Equal(t, moderation.RiskLow, classifier.Classify(processed("篡改词")).Risk)
}

func TestClassifierRejectsEmptyRefreshAndKeepsLastGoodSnapshot(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshot(
		rule(1, moderation.RuleKeyword, moderation.RiskHigh, "保留词"),
	))

	err := classifier.ReplaceSnapshot(moderation.RuleSnapshot{Version: 2})

	require.Error(t, err)
	assert.Equal(t, moderation.RiskHigh, classifier.Classify(processed("保留词")).Risk)
}

func TestClassifierRejectsRulesetVersionRegression(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshotVersion(2,
		rule(1, moderation.RuleKeyword, moderation.RiskHigh, "旧规则"),
	))

	err := classifier.ReplaceSnapshot(snapshotVersion(1,
		rule(2, moderation.RuleKeyword, moderation.RiskHigh, "回退规则"),
	))

	require.Error(t, err)
	assert.Equal(t, moderation.RiskHigh, classifier.Classify(processed("旧规则")).Risk)
	assert.Equal(t, moderation.RiskLow, classifier.Classify(processed("回退规则")).Risk)
}

func TestClassifierOnlyReadsSanitizedPlainText(t *testing.T) {
	classifier := moderation.NewClassifier(zap.NewNop(), snapshot(
		rule(1, moderation.RuleKeyword, moderation.RiskHigh, "脚本风险"),
	))
	content := moderation.ProcessedContent{
		Published: `<script>脚本风险</script><p>安全正文</p>`,
		PlainText: "安全正文",
	}

	got := classifier.Classify(content)

	assert.Equal(t, moderation.RiskLow, got.Risk)
}

func snapshot(rules ...moderation.CompiledRule) moderation.RuleSnapshot {
	return snapshotVersion(1, rules...)
}

func snapshotVersion(version uint64, rules ...moderation.CompiledRule) moderation.RuleSnapshot {
	return moderation.RuleSnapshot{Version: version, Rules: rules}
}

func rule(id uint64, ruleType moderation.RuleType, risk moderation.RiskLevel, pattern string) moderation.CompiledRule {
	return moderation.CompiledRule{
		ID:      id,
		Type:    ruleType,
		Risk:    risk,
		Pattern: pattern,
	}
}

func processed(text string) moderation.ProcessedContent {
	return moderation.ProcessedContent{PlainText: strings.TrimSpace(text)}
}
