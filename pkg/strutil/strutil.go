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
