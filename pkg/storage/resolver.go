package storage

import "context"

// ObjectURLResolver 解析对象存储 key，返回可直接访问的 Garage 或 CDN 签名 URL。
type ObjectURLResolver interface {
	ObjectURL(ctx context.Context, objectName string) (string, error)
}
