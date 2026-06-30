package moderationrule

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
)

func TestCSVParserAcceptsQuotedNewlineAndLeadingComments(t *testing.T) {
	input := "# template\npattern,category,risk_level\n\"line1\nline2\",other,medium\n"
	rows, errs := parseCSV(strings.NewReader(input), testDefaults(), 1000)

	require.Empty(t, errs)
	require.Len(t, rows, 1)
	assert.Equal(t, "line1\nline2", rows[0].Pattern)
	assert.Equal(t, "other", rows[0].Category)
	assert.Equal(t, "medium", rows[0].RiskLevel)
}

func TestCSVParserAppliesDefaultsForMissingColumns(t *testing.T) {
	input := "pattern\n风险词\n"
	rows, errs := parseCSV(strings.NewReader(input), testDefaults(), 1000)

	require.Empty(t, errs)
	require.Len(t, rows, 1)
	assert.Equal(t, "风险词", rows[0].Pattern)
	assert.Equal(t, "fraud", rows[0].Category)
	assert.Equal(t, "review", rows[0].Effect)
}

func TestCSVParserRejectsMissingPatternHeader(t *testing.T) {
	input := "category,risk_level\nother,medium\n"
	_, errs := parseCSV(strings.NewReader(input), testDefaults(), 1000)

	require.Len(t, errs, 1)
	assert.Equal(t, "header_missing", errs[0].Code)
}

func TestCSVParserStopsAtMaxRows(t *testing.T) {
	input := "pattern\n词1\n词2\n词3\n"
	rows, _ := parseCSV(strings.NewReader(input), testDefaults(), 2)

	assert.Len(t, rows, 2)
}

func TestTXTParserSkipsCommentsAndBlankLines(t *testing.T) {
	input := "# 注释\n\n关键词1\n\n# 另一注释\n关键词2\n"
	rows, errs := parseTXT(strings.NewReader(input), testDefaults(), 1000)

	require.Empty(t, errs)
	require.Len(t, rows, 2)
	assert.Equal(t, "关键词1", rows[0].Pattern)
	assert.Equal(t, "关键词2", rows[1].Pattern)
}

func TestTXTParserUsesDefaultsForAllFields(t *testing.T) {
	input := "测试词\n"
	rows, errs := parseTXT(strings.NewReader(input), testDefaults(), 1000)

	require.Empty(t, errs)
	require.Len(t, rows, 1)
	assert.Equal(t, "fraud", rows[0].Category)
	assert.Equal(t, "review", rows[0].Effect)
	assert.Equal(t, "medium", rows[0].RiskLevel)
}

func TestValidateParsedRowRejectsInvalidCategory(t *testing.T) {
	row := ParsedRow{LineNumber: 1, Pattern: "词", Category: "invalid", Effect: "review", RiskLevel: "medium"}
	errs := validateParsedRow(row, 500)
	require.NotEmpty(t, errs)
	assert.Equal(t, "invalid_category", errs[0].Code)
}

func TestValidateParsedRowRejectsLongPattern(t *testing.T) {
	long := strings.Repeat("a", 501)
	row := ParsedRow{LineNumber: 1, Pattern: long, Category: "other", Effect: "review", RiskLevel: "medium"}
	errs := validateParsedRow(row, 500)
	require.NotEmpty(t, errs)
	assert.Equal(t, "pattern_too_long", errs[0].Code)
}

func TestCreateImportEnsuresSourceFirst(t *testing.T) {
	repo, mgr := newTestManager(t)
	repo.EXPECT().EnsureSource(gomock.Any(), "测试来源").Return(repoMod.SourceRecord{ID: 1, Name: "测试来源"}, nil)
	repo.EXPECT().CreateImport(gomock.Any(), gomock.Any()).Return(testImportRecord(), nil)

	_, err := mgr.CreateImport(context.Background(), CreateImportInput{
		FileName:         "test.csv",
		Format:           "csv",
		FileSize:         1024,
		ObjectKey:        "moderation/imports/test.csv",
		SourceName:       "测试来源",
		DefaultCategory:  "fraud",
		DefaultEffect:    "review",
		DefaultRiskLevel: "medium",
		DefaultPriority:  100,
		OperatorID:       1,
	})

	require.NoError(t, err)
}

func TestCreateImportRejectsInvalidFormat(t *testing.T) {
	_, mgr := newTestManager(t)

	_, err := mgr.CreateImport(context.Background(), CreateImportInput{
		FileName:         "test.zip",
		Format:           "zip",
		FileSize:         1024,
		ObjectKey:        "test",
		SourceName:       "来源",
		DefaultCategory:  "fraud",
		DefaultEffect:    "review",
		DefaultRiskLevel: "medium",
		DefaultPriority:  100,
		OperatorID:       1,
	})

	assert.Error(t, err)
}

func TestCancelImportReturnsNotFoundForMissing(t *testing.T) {
	repo, mgr := newTestManager(t)
	repo.EXPECT().CancelImport(gomock.Any(), uint64(99), gomock.Any(), gomock.Any()).Return(repoMod.ErrCandidateNotFound)

	err := mgr.CancelImport(context.Background(), 99, 1)
	assert.ErrorIs(t, err, ErrCandidateNotFound)
}

func testDefaults() ImportDefaults {
	return ImportDefaults{
		Category:  "fraud",
		Effect:    "review",
		RiskLevel: "medium",
		Priority:  100,
	}
}

func testImportRecord() repoMod.ImportRecord {
	return repoMod.ImportRecord{
		ID:               1,
		FileName:         "test.csv",
		Format:           "csv",
		FileSize:         1024,
		ObjectKey:        "moderation/imports/test.csv",
		SourceID:         1,
		DefaultCategory:  "fraud",
		DefaultEffect:    "review",
		DefaultRiskLevel: "medium",
		DefaultPriority:  100,
		ValidationStatus: "queued",
		OperatorID:       1,
	}
}
