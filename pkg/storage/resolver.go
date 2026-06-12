package storage

import (
	"context"
	"strings"
)

// ObjectURLResolver 解析对象存储 key，返回可直接访问的 Garage 或 CDN 签名 URL。
type ObjectURLResolver interface {
	ObjectURL(ctx context.Context, objectName string) (string, error)
}

// ObjectStore 提供对象访问 URL、存在性检查和写入能力。
type ObjectStore interface {
	ObjectURLResolver
	ObjectExists(ctx context.Context, objectName string) (bool, error)
	PutObject(ctx context.Context, objectName string, data []byte, contentType string) error
}

// IsAbsoluteURL 判断给定的 URL 是否是一个绝对路径（以 http:// 或 https:// 开头）。
// 超过两处使用，封装为公共方法以复用。
func IsAbsoluteURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

// ResolvePtrURL 尝试解析可能的对象存储路径指针为完整的访问 URL 指针。
// 超过两处使用，封装为公共方法以复用。
// 如果传入的 url 为空或者是完整的绝对路径，则直接返回原指针；
// 否则使用传入的 resolver 进行解析，解析失败则静默返回原指针。
func ResolvePtrURL(resolver ObjectURLResolver, url *string) *string {
	if url == nil || resolver == nil {
		return url
	}
	trimmed := strings.TrimSpace(*url)
	if trimmed == "" || IsAbsoluteURL(trimmed) {
		return url
	}
	if resolved, err := resolver.ObjectURL(context.Background(), trimmed); err == nil {
		return &resolved
	}
	return url
}

// ResolveURL 尝试解析可能的对象存储路径为完整的访问 URL。
// 如果传入的 url 为空或者是完整的绝对路径，则直接返回原字符串；
// 否则使用传入的 resolver 进行解析，解析失败则静默返回原字符串。
func ResolveURL(resolver ObjectURLResolver, url string) string {
	if resolver == nil {
		return url
	}
	trimmed := strings.TrimSpace(url)
	if trimmed == "" || IsAbsoluteURL(trimmed) {
		return url
	}
	if resolved, err := resolver.ObjectURL(context.Background(), trimmed); err == nil {
		return resolved
	}
	return url
}
