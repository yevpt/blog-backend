package moderationrule

import (
	"encoding/csv"
	"fmt"
	"io"
)

// WriteCSVTemplate 写入带 # 说明行和标准表头的 CSV 模板。
// 模板不得包含会被误发布的真实敏感词。
func WriteCSVTemplate(w io.Writer) error {
	lines := []string{
		"# 审核规则导入模板",
		"# 列说明：name rule_type pattern category effect risk_level priority",
		"# keyword 示例：名称,keyword,关键词,other,review,medium,100",
		"# regexp 示例：名称,regexp,^pattern$,other,review,high,100",
		"# composite 示例：名称,composite,信号一&&信号二,other,review,high,100",
		"name,rule_type,pattern,category,effect,risk_level,priority",
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("写入 CSV 模板: %w", err)
		}
	}
	return nil
}

// WriteTXTTemplate 写入仅含 # 说明行的 TXT 模板。
func WriteTXTTemplate(w io.Writer) error {
	lines := []string{
		"# 审核规则导入模板（TXT 格式）",
		"# 每行一个关键词，空行和 # 开头行自动跳过",
		"# 分类、风险、效果和优先级使用上传表单缺省值",
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("写入 TXT 模板: %w", err)
		}
	}
	return nil
}

// WriteExport 将规则列表流式导出为带 UTF-8 BOM 的 CSV。
// 对以 = + - @ 开头的单元格前置单引号，防止电子表格公式注入。
func WriteExport(w io.Writer, rules []ExportRule) error {
	// 写入 UTF-8 BOM 供 Excel 正确识别编码。
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return fmt.Errorf("写入 BOM: %w", err)
	}

	csvWriter := csv.NewWriter(w)
	if err := csvWriter.Write([]string{"id", "name", "rule_type", "pattern", "category", "effect", "risk_level", "priority", "source_id", "active"}); err != nil {
		return fmt.Errorf("写入导出表头: %w", err)
	}

	for _, rule := range rules {
		name := ""
		if rule.Name != nil {
			name = *rule.Name
		}
		active := "false"
		if rule.Active {
			active = "true"
		}
		row := []string{
			fmt.Sprintf("%d", rule.ID),
			escapeCSVCell(name),
			escapeCSVCell(rule.RuleType),
			escapeCSVCell(rule.Pattern),
			escapeCSVCell(rule.Category),
			escapeCSVCell(rule.Effect),
			escapeCSVCell(rule.RiskLevel),
			fmt.Sprintf("%d", rule.Priority),
			fmt.Sprintf("%d", rule.SourceID),
			active,
		}
		if err := csvWriter.Write(row); err != nil {
			return fmt.Errorf("写入导出行: %w", err)
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

// ExportRule 是导出单行所需的最小规则字段。
type ExportRule struct {
	ID        uint64
	Name      *string
	RuleType  string
	Pattern   string
	Category  string
	Effect    string
	RiskLevel string
	Priority  int32
	SourceID  uint64
	Active    bool
}

// escapeCSVCell 对以 = + - @ 开头的单元格前置单引号，防止公式注入。
func escapeCSVCell(value string) string {
	if value == "" {
		return value
	}
	first := value[0]
	if first == '=' || first == '+' || first == '-' || first == '@' {
		return "'" + value
	}
	return value
}
