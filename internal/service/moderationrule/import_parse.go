package moderationrule

import (
	"bufio"
	"encoding/csv"
	"io"
	"strconv"
	"strings"
)

// ParsedRow 是单行解析结果，字段按行值优先、缺省值兜底。
type ParsedRow struct {
	LineNumber int
	Name       *string
	RuleType   string
	Pattern    string
	Category   string
	Effect     string
	RiskLevel  string
	Priority   int32
}

// RowError 是单行校验错误。
type RowError struct {
	LineNumber int
	Field      string
	Value      string
	Code       string
	Message    string
}

// parseCSV 流式解析 CSV 输入，行值覆盖缺省值，最多解析 maxRows 行。
// 仅允许在表头前出现 # 注释行。
func parseCSV(r io.Reader, defaults ImportDefaults, maxRows int) ([]ParsedRow, []RowError) {
	bufReader := bufio.NewReader(r)
	// 跳过 # 开头的注释行，仅支持表头前注释。
	for {
		peek, err := bufReader.Peek(1)
		if err != nil {
			break
		}
		if peek[0] == '#' {
			_, _, err := bufReader.ReadLine()
			if err != nil {
				break
			}
			continue
		}
		break
	}

	reader := csv.NewReader(bufReader)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return nil, []RowError{{LineNumber: 0, Code: "header_missing", Message: "缺少 CSV 表头"}}
	}

	colIndex := mapCsvColumns(header)
	if _, ok := colIndex["pattern"]; !ok {
		return nil, []RowError{{LineNumber: 0, Code: "header_missing", Message: "CSV 表头缺少 pattern 列"}}
	}

	var rows []ParsedRow
	var errs []RowError
	lineNumber := 1
	dataRows := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errs = append(errs, RowError{LineNumber: lineNumber, Code: "parse_error", Message: err.Error()})
			lineNumber++
			continue
		}
		lineNumber++
		dataRows++
		if maxRows > 0 && dataRows > maxRows {
			errs = append(errs, RowError{LineNumber: lineNumber, Code: "row_limit_exceeded", Message: "导入行数超过上限"})
			break
		}

		row, rowErrs := applyCsvRow(record, colIndex, defaults, lineNumber)
		if len(rowErrs) > 0 {
			errs = append(errs, rowErrs...)
			continue
		}
		rows = append(rows, row)
	}
	return rows, errs
}

// parseTXT 流式解析 TXT 输入，每行一个关键词，统一使用缺省值。
func parseTXT(r io.Reader, defaults ImportDefaults, maxRows int) ([]ParsedRow, []RowError) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var rows []ParsedRow
	var errs []RowError
	lineNumber := 0
	dataRows := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		dataRows++
		if maxRows > 0 && dataRows > maxRows {
			errs = append(errs, RowError{LineNumber: lineNumber, Code: "row_limit_exceeded", Message: "导入行数超过上限"})
			break
		}
		rows = append(rows, ParsedRow{
			LineNumber: lineNumber,
			RuleType:   "keyword",
			Pattern:    line,
			Category:   defaults.Category,
			Effect:     defaults.Effect,
			RiskLevel:  defaults.RiskLevel,
			Priority:   defaults.Priority,
		})
	}
	if err := scanner.Err(); err != nil {
		errs = append(errs, RowError{LineNumber: lineNumber, Code: "read_error", Message: err.Error()})
	}
	return rows, errs
}

func mapCsvColumns(header []string) map[string]int {
	index := make(map[string]int, len(header))
	for i, col := range header {
		index[strings.TrimSpace(strings.ToLower(col))] = i
	}
	return index
}

func applyCsvRow(record []string, colIndex map[string]int, defaults ImportDefaults, lineNumber int) (ParsedRow, []RowError) {
	var errs []RowError
	row := ParsedRow{
		LineNumber: lineNumber,
		RuleType:   "keyword",
		Category:   defaults.Category,
		Effect:     defaults.Effect,
		RiskLevel:  defaults.RiskLevel,
		Priority:   defaults.Priority,
	}
	if idx, ok := colIndex["name"]; ok && idx < len(record) {
		if value := strings.TrimSpace(record[idx]); value != "" {
			row.Name = &value
		}
	}
	if idx, ok := colIndex["rule_type"]; ok && idx < len(record) {
		if value := strings.TrimSpace(strings.ToLower(record[idx])); value != "" {
			row.RuleType = value
		}
	}

	if idx, ok := colIndex["pattern"]; ok && idx < len(record) {
		row.Pattern = strings.TrimSpace(record[idx])
	}
	if row.Pattern == "" {
		errs = append(errs, RowError{LineNumber: lineNumber, Field: "pattern", Code: "pattern_empty", Message: "模式不能为空"})
	}

	if idx, ok := colIndex["category"]; ok && idx < len(record) {
		if val := strings.TrimSpace(record[idx]); val != "" {
			row.Category = val
		}
	}
	if idx, ok := colIndex["effect"]; ok && idx < len(record) {
		if val := strings.TrimSpace(record[idx]); val != "" {
			row.Effect = val
		}
	}
	if idx, ok := colIndex["risk_level"]; ok && idx < len(record) {
		if val := strings.TrimSpace(record[idx]); val != "" {
			row.RiskLevel = val
		}
	}
	if idx, ok := colIndex["priority"]; ok && idx < len(record) {
		if value := strings.TrimSpace(record[idx]); value != "" {
			priority, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				errs = append(errs, RowError{LineNumber: lineNumber, Field: "priority", Value: value, Code: "invalid_priority", Message: "优先级必须是 32 位整数"})
			} else {
				row.Priority = int32(priority)
			}
		}
	}

	return row, errs
}

// validateParsedRow 复用单条规则校验，并将错误定位到导入行。
func validateParsedRow(cfg ManagerConfig, row ParsedRow) []RowError {
	err := validateRuleInput(cfg, RuleInput{
		Name: row.Name, RuleType: row.RuleType, Pattern: row.Pattern, Category: row.Category,
		Effect: row.Effect, RiskLevel: row.RiskLevel, Priority: row.Priority, SourceID: 1,
	})
	if err == nil {
		return nil
	}
	field, code := classifyRuleValidationError(err)
	return []RowError{{LineNumber: row.LineNumber, Field: field, Value: parsedRowFieldValue(row, field), Code: code, Message: err.Error()}}
}

func classifyRuleValidationError(err error) (string, string) {
	message := err.Error()
	switch {
	case strings.Contains(message, "规则类型"):
		return "rule_type", "invalid_rule_type"
	case strings.Contains(message, "分类"):
		return "category", "invalid_category"
	case strings.Contains(message, "规则效果"), strings.Contains(message, "allow"):
		return "effect", "invalid_effect"
	case strings.Contains(message, "风险等级"):
		return "risk_level", "invalid_risk_level"
	case strings.Contains(message, "名称"):
		return "name", "invalid_name"
	case strings.Contains(message, "正则编译"):
		return "pattern", "invalid_regexp"
	case strings.Contains(message, "组合"):
		return "pattern", "invalid_composite"
	case strings.Contains(message, "超过"):
		return "pattern", "pattern_too_long"
	default:
		return "pattern", "invalid_pattern"
	}
}

func parsedRowFieldValue(row ParsedRow, field string) string {
	switch field {
	case "name":
		if row.Name != nil {
			return *row.Name
		}
	case "rule_type":
		return row.RuleType
	case "category":
		return row.Category
	case "effect":
		return row.Effect
	case "risk_level":
		return row.RiskLevel
	}
	return row.Pattern
}

func isValidCategory(value string) bool {
	for _, cat := range moderationCategories() {
		if cat.Key == value {
			return true
		}
	}
	return false
}

func isValidEffect(value string) bool {
	return value == "review" || value == "allow"
}

func isValidRiskLevel(value string) bool {
	return value == "low" || value == "medium" || value == "high"
}
