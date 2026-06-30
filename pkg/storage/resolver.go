package storage

import (
	"context"
	"io"
	"strings"
	"time"
)

// ObjectURLResolver 解析对象存储 key，返回可直接访问的 Garage 或 CDN 签名 URL。
type ObjectURLResolver interface {
	ObjectURL(ctx context.Context, objectName string) (string, error)
}

// ObjectMover 提供同 bucket 内对象移动能力。
type ObjectMover interface {
	MoveObject(ctx context.Context, sourceName string, targetName string) error
}

type ObjectCopier interface {
	CopyObject(ctx context.Context, sourceName string, targetName string) error
}

type ObjectKeyResolver interface {
	ObjectKey(value string) (string, error)
}

// ObjectReader 从对象存储读取原始字节，供需要校验内容而非仅解析 URL 的服务使用。
type ObjectReader interface {
	GetObject(ctx context.Context, objectName string) ([]byte, error)
}

// ObjectStreamStore 提供有界对象流读写能力，避免大型产物整块驻留内存。
type ObjectStreamStore interface {
	PutObjectStream(ctx context.Context, objectName string, body io.Reader, size int64, contentType string) error
	OpenObject(ctx context.Context, objectName string, maxBytes int64) (io.ReadCloser, error)
}

// ImageObjectReader 以图片专用上限读取历史原图和 CDN 回源图片。
type ImageObjectReader interface {
	GetImageObject(ctx context.Context, objectName string) ([]byte, error)
}

// ObjectMetadata 是对象清理所需的最小元数据。
type ObjectMetadata struct {
	Key          string
	LastModified time.Time
	Size         int64
}

// ObjectPage 是按 key 游标读取的一页对象。
type ObjectPage struct {
	Objects   []ObjectMetadata
	NextAfter string
	HasMore   bool
}

// ObjectPageLister 按固定前缀有界列出对象，避免清理任务全量加载 bucket。
type ObjectPageLister interface {
	ListObjectPage(ctx context.Context, prefix, after string, limit int) (ObjectPage, error)
}

// ObjectStore 提供对象访问 URL、存在性检查和写入能力。
type ObjectStore interface {
	ObjectURLResolver
	ObjectMover
	ObjectCopier
	ObjectKeyResolver
	ObjectExists(ctx context.Context, objectName string) (bool, error)
	PutObject(ctx context.Context, objectName string, data []byte, contentType string) error
	DeleteObject(ctx context.Context, objectName string) error
}

// ReadableObjectStore 是需要校验对象原始内容的完整存储能力。
type ReadableObjectStore interface {
	ObjectStore
	ObjectReader
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
