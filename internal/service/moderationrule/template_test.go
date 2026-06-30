package moderationrule

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCSVTemplateContainsHeaderAndExample(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteCSVTemplate(&buf))
	output := buf.String()
	assert.Contains(t, output, "#")
	assert.Contains(t, output, "pattern,category,risk_level,effect,priority")
	assert.Contains(t, output, "示例关键词请删除")
}

func TestWriteTXTTemplateContainsOnlyComments(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteTXTTemplate(&buf))
	output := buf.String()
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		assert.True(t, strings.HasPrefix(line, "#"), "TXT 模板每行应为注释: %s", line)
	}
}

func TestWriteExportIncludesBOMAndHeader(t *testing.T) {
	var buf bytes.Buffer
	rules := []ExportRule{
		{ID: 1, RuleType: "keyword", Pattern: "测试", Category: "fraud", Effect: "review", RiskLevel: "medium", Priority: 100, SourceID: 1, Active: true},
	}
	require.NoError(t, WriteExport(&buf, rules))
	output := buf.Bytes()
	// UTF-8 BOM
	assert.Equal(t, []byte{0xEF, 0xBB, 0xBF}, output[:3])
	// 包含表头
	assert.Contains(t, string(output), "id,name,rule_type,pattern")
	// 包含规则数据
	assert.Contains(t, string(output), "测试")
	assert.Contains(t, string(output), "true")
}

func TestWriteExportEscapesFormulaInjection(t *testing.T) {
	var buf bytes.Buffer
	rules := []ExportRule{
		{ID: 1, RuleType: "keyword", Pattern: "=cmd|test", Category: "other", Effect: "review", RiskLevel: "low", Priority: 100, SourceID: 1, Active: false},
	}
	require.NoError(t, WriteExport(&buf, rules))
	output := buf.String()
	assert.Contains(t, output, "'=cmd|test")
}

func TestEscapeCSVCell(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"normal", "normal"},
		{"=evil", "'=evil"},
		{"+evil", "'+evil"},
		{"-evil", "'-evil"},
		{"@evil", "'@evil"},
		{"", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expect, escapeCSVCell(tt.input), "escapeCSVCell(%q)", tt.input)
	}
}
