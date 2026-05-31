package storage

import "strings"

// fullObjectPath 返回 CDN 侧使用的 /bucket/object 完整路径。
func (c *Client) fullObjectPath(objectName string) string {
	// 先清理 bucket 两侧斜杠，避免拼接时出现重复分隔符。
	bucket := strings.Trim(c.impl.bucket, "/")

	// 再清理 objectName，确保对象 key 不以斜杠开头。
	objectName = normalizeObjectName(objectName)

	// 最后补齐 CDN 签名算法需要的前导斜杠。
	return "/" + strings.Trim(bucket+"/"+objectName, "/")
}

// normalizeObjectName 清理对象名空白和前导斜杠。
func normalizeObjectName(objectName string) string {
	// 先去掉首尾空白，再去掉前导斜杠，保留对象名内部路径结构。
	return strings.TrimLeft(strings.TrimSpace(objectName), "/")
}

// ensureLeadingSlash 确保路径以斜杠开头。
func ensureLeadingSlash(path string) string {
	// 已有前导斜杠时直接返回，避免重复添加。
	if strings.HasPrefix(path, "/") {
		return path
	}

	// 无前导斜杠时补齐，满足 URL path 的标准格式。
	return "/" + path
}

// defaultString 在 value 为空时返回 fallback。
func defaultString(value, fallback string) string {
	// 调用方未配置时使用默认值，减少配置噪音。
	if value == "" {
		return fallback
	}

	// 调用方显式配置时尊重传入值。
	return value
}
