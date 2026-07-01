package moderationrule

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	repoMod "github.com/vpt/blog-backend/internal/repository/moderationrule"
	repoModMock "github.com/vpt/blog-backend/internal/repository/moderationrule/mock"
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

func TestParseCSVSupportsAllRuleTypesAndCompleteFields(t *testing.T) {
	input := strings.Join([]string{
		"name,rule_type,pattern,category,effect,risk_level,priority",
		"关键词,keyword,风险词,fraud,review,medium,10",
		"正则,regexp,^risk-[0-9]+$,other,review,high,20",
		"组合,composite,信号一&&信号二,abuse,review,low,30",
		"默认类型,,普通词,other,allow,low,40",
	}, "\n") + "\n"

	rows, errs := parseCSV(strings.NewReader(input), testDefaults(), 1000)

	require.Empty(t, errs)
	require.Len(t, rows, 4)
	require.NotNil(t, rows[0].Name)
	assert.Equal(t, "关键词", *rows[0].Name)
	assert.Equal(t, "keyword", rows[0].RuleType)
	assert.Equal(t, int32(10), rows[0].Priority)
	assert.Equal(t, "regexp", rows[1].RuleType)
	assert.Equal(t, "composite", rows[2].RuleType)
	assert.Equal(t, "keyword", rows[3].RuleType)
}

func TestParseCSVReportsRuleValidationErrorsWithLineNumbers(t *testing.T) {
	input := strings.Join([]string{
		"name,rule_type,pattern,category,effect,risk_level,priority",
		"坏正则,regexp,[,other,review,medium,100",
		"单信号,composite,只有一个,other,review,medium,100",
		"放行正则,regexp,^allow$,other,allow,low,100",
		"坏优先级,keyword,词,other,review,low,abc",
	}, "\n") + "\n"

	rows, parseErrs := parseCSV(strings.NewReader(input), testDefaults(), 1000)

	require.Len(t, parseErrs, 1)
	assert.Equal(t, 5, parseErrs[0].LineNumber)
	assert.Equal(t, "priority", parseErrs[0].Field)
	assert.Equal(t, "invalid_priority", parseErrs[0].Code)
	require.Len(t, rows, 3)
	for i, expectedLine := range []int{2, 3, 4} {
		errs := validateParsedRow(testManagerConfig(), rows[i])
		require.NotEmpty(t, errs)
		assert.Equal(t, expectedLine, errs[0].LineNumber)
	}
}

func TestCSVParserRejectsMissingPatternHeader(t *testing.T) {
	input := "category,risk_level\nother,medium\n"
	_, errs := parseCSV(strings.NewReader(input), testDefaults(), 1000)

	require.Len(t, errs, 1)
	assert.Equal(t, "header_missing", errs[0].Code)
}

func TestCSVParserRejectsRowsBeyondLimit(t *testing.T) {
	input := "pattern\n词1\n词2\n词3\n"
	rows, errs := parseCSV(strings.NewReader(input), testDefaults(), 2)

	assert.Len(t, rows, 2)
	require.Len(t, errs, 1)
	assert.Equal(t, "row_limit_exceeded", errs[0].Code)
}

func TestTXTParserRejectsRowsBeyondLimit(t *testing.T) {
	rows, errs := parseTXT(strings.NewReader("词1\n词2\n词3\n"), testDefaults(), 2)

	assert.Len(t, rows, 2)
	require.Len(t, errs, 1)
	assert.Equal(t, "row_limit_exceeded", errs[0].Code)
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
	row := ParsedRow{LineNumber: 1, RuleType: "keyword", Pattern: "词", Category: "invalid", Effect: "review", RiskLevel: "medium", Priority: 100}
	errs := validateParsedRow(testManagerConfig(), row)
	require.NotEmpty(t, errs)
	assert.Equal(t, "invalid_category", errs[0].Code)
}

func TestValidateParsedRowRejectsLongPattern(t *testing.T) {
	long := strings.Repeat("a", 501)
	row := ParsedRow{LineNumber: 1, RuleType: "keyword", Pattern: long, Category: "other", Effect: "review", RiskLevel: "medium", Priority: 100}
	errs := validateParsedRow(testManagerConfig(), row)
	require.NotEmpty(t, errs)
	assert.Equal(t, "pattern_too_long", errs[0].Code)
}

func TestCreateImportEnsuresSourceFirst(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repoModMock.NewMockManagementRepository(ctrl)
	mgr := NewManager(repo, &importStoreStub{}, &fakeReplacer{}, testManagerConfig(), nil)
	repo.EXPECT().EnsureSource(gomock.Any(), "测试来源").Return(repoMod.SourceRecord{ID: 1, Name: "测试来源"}, nil)
	repo.EXPECT().CreateImport(gomock.Any(), gomock.Any()).Return(testImportRecord(), nil)
	content := "pattern\n测试\n"

	_, err := mgr.CreateImport(context.Background(), CreateImportInput{
		FileName:         "test.csv",
		Format:           "csv",
		FileSize:         uint64(len(content)),
		Body:             strings.NewReader(content),
		SourceName:       "测试来源",
		DefaultCategory:  "fraud",
		DefaultEffect:    "review",
		DefaultRiskLevel: "medium",
		DefaultPriority:  100,
		OperatorID:       1,
	})

	require.NoError(t, err)
}

func TestCreateImportUploadsBodyAndCompensatesWhenTaskCreationFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repoModMock.NewMockManagementRepository(ctrl)
	store := &importStoreStub{}
	mgr := NewManager(repo, store, &fakeReplacer{}, testManagerConfig(), nil)
	repo.EXPECT().EnsureSource(gomock.Any(), "测试来源").Return(repoMod.SourceRecord{ID: 3, Name: "测试来源"}, nil)
	repo.EXPECT().CreateImport(gomock.Any(), gomock.Any()).Return(repoMod.ImportRecord{}, errors.New("db down"))

	content := "pattern\n风险词\n"
	_, err := mgr.CreateImport(context.Background(), CreateImportInput{
		FileName: "rules.csv", Format: "csv", FileSize: uint64(len(content)), Body: strings.NewReader(content),
		SourceName: "测试来源", DefaultCategory: "fraud", DefaultEffect: "review",
		DefaultRiskLevel: "medium", DefaultPriority: 100, OperatorID: 9,
	})

	require.Error(t, err)
	require.Len(t, store.putKeys, 1)
	assert.Regexp(t, `^moderation/imports/9/[0-9a-f-]+\.csv$`, store.putKeys[0])
	assert.Equal(t, store.putKeys, store.deletedKeys)
}

func TestCreateImportRejectsFormatExtensionMismatch(t *testing.T) {
	_, mgr := newTestManager(t)

	_, err := mgr.CreateImport(context.Background(), CreateImportInput{
		FileName: "rules.txt", Format: "csv", FileSize: 5, Body: strings.NewReader("测试\n"),
		SourceName: "来源", DefaultCategory: "fraud", DefaultEffect: "review",
		DefaultRiskLevel: "medium", DefaultPriority: 100, OperatorID: 1,
	})

	assert.ErrorIs(t, err, ErrInvalidRule)
}

func TestValidateImportReportsParseAndRuleErrorsTogether(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repoModMock.NewMockManagementRepository(ctrl)
	content := "name,rule_type,pattern,category,effect,risk_level,priority\n坏正则,regexp,[,other,review,medium,100\n坏优先级,keyword,词,other,review,low,abc\n"
	store := &importStoreStub{openData: content}
	mgr := newManager(repo, store, &fakeReplacer{}, testManagerConfig(), nil)
	repo.EXPECT().CurrentStatus(gomock.Any()).Return(repoMod.StatusRecord{CurrentRulesetID: 7}, nil)

	result, err := mgr.validateImport(context.Background(), repoMod.ImportRecord{
		ID: 12, Format: "csv", FileSize: uint64(len(content)), ObjectKey: "moderation/imports/1/file.csv",
		DefaultCategory: "other", DefaultEffect: "review", DefaultRiskLevel: "medium", DefaultPriority: 100,
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(2), result.errorRows)
	require.Len(t, store.putBodies, 1)
	assert.Contains(t, store.putBodies[0], "2,pattern")
	assert.Contains(t, store.putBodies[0], "3,priority")
}

func TestCreateImportRejectsInvalidFormat(t *testing.T) {
	_, mgr := newTestManager(t)

	_, err := mgr.CreateImport(context.Background(), CreateImportInput{
		FileName:         "test.zip",
		Format:           "zip",
		FileSize:         1024,
		Body:             strings.NewReader("测试"),
		SourceName:       "来源",
		DefaultCategory:  "fraud",
		DefaultEffect:    "review",
		DefaultRiskLevel: "medium",
		DefaultPriority:  100,
		OperatorID:       1,
	})

	assert.Error(t, err)
}

type importStoreStub struct {
	putKeys     []string
	putBodies   []string
	deletedKeys []string
	openData    string
}

func (s *importStoreStub) PutObjectStream(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(body)
	s.putKeys = append(s.putKeys, key)
	s.putBodies = append(s.putBodies, string(data))
	return err
}

func (s *importStoreStub) OpenObject(context.Context, string, int64) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte(s.openData))), nil
}

func (s *importStoreStub) DeleteObject(_ context.Context, key string) error {
	s.deletedKeys = append(s.deletedKeys, key)
	return nil
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

func testManagerConfig() ManagerConfig {
	return ManagerConfig{MaxPatternChars: 500}
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
