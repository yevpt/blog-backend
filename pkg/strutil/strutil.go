// Package strutil 提供字符串通用处理工具。
package strutil

import "strings"

// CleanOptional 规整可选字符串：去首尾空白；trim 后为空则返回 nil。
// 用于把前端传入的空白/空串归一为「未设置」。
func CleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// CleanOptionalUpdate 规整「可选更新」字符串：返回 trim 后的值，以及该字段是否参与更新。
// nil 表示未传字段（不参与更新）；trim 后为空表示传入空串以清空字段。
func CleanOptionalUpdate(value *string) (*string, bool) {
	if value == nil {
		return nil, false
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, true
	}
	return &trimmed, true
}
