package moderationrule

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// ParsedRow 是单行解析结果，字段按行值优先、缺省值兜底。
type ParsedRow struct {
	LineNumber int
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

	for {
		if maxRows > 0 && len(rows) >= maxRows {
			break
		}
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

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if maxRows > 0 && len(rows) >= maxRows {
			break
		}
		if utf8.RuneCountInString(line) > 500 {
			errs = append(errs, RowError{LineNumber: lineNumber, Field: "pattern", Value: line, Code: "pattern_too_long", Message: "关键词超过 500 字符"})
			continue
		}
		rows = append(rows, ParsedRow{
			LineNumber: lineNumber,
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
		Category:   defaults.Category,
		Effect:     defaults.Effect,
		RiskLevel:  defaults.RiskLevel,
		Priority:   defaults.Priority,
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

	return row, errs
}

// validateParsedRow 校验单行内容的枚举和长度约束。
func validateParsedRow(row ParsedRow, maxPatternChars int) []RowError {
	var errs []RowError
	if row.Pattern == "" {
		errs = append(errs, RowError{LineNumber: row.LineNumber, Field: "pattern", Code: "pattern_empty", Message: "模式不能为空"})
	} else if utf8.RuneCountInString(row.Pattern) > maxPatternChars {
		errs = append(errs, RowError{LineNumber: row.LineNumber, Field: "pattern", Value: row.Pattern, Code: "pattern_too_long", Message: fmt.Sprintf("模式超过 %d 字符", maxPatternChars)})
	}
	if !isValidCategory(row.Category) {
		errs = append(errs, RowError{LineNumber: row.LineNumber, Field: "category", Value: row.Category, Code: "invalid_category", Message: "分类无效"})
	}
	if !isValidEffect(row.Effect) {
		errs = append(errs, RowError{LineNumber: row.LineNumber, Field: "effect", Value: row.Effect, Code: "invalid_effect", Message: "规则效果无效"})
	}
	if !isValidRiskLevel(row.RiskLevel) {
		errs = append(errs, RowError{LineNumber: row.LineNumber, Field: "risk_level", Value: row.RiskLevel, Code: "invalid_risk_level", Message: "风险等级无效"})
	}
	return errs
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
