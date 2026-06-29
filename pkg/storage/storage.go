package storage

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/redis/go-redis/v9"
	appconfig "github.com/vpt/blog-backend/pkg/config"
)

// 本文件是 pkg/storage 的对外入口。
// 调用方式看这里；具体实现分别在 garage.go、cdn.go、path.go 中。

// DefaultObjectURLCacheTTL 是对象访问 URL 的默认 Redis 缓存时间，短于 Garage 预签名默认 7 天有效期。
const DefaultObjectURLCacheTTL = 6 * 24 * time.Hour

// Client 封装 Garage S3 客户端和对象访问 URL 生成能力。
type Client struct {
	impl *clientImpl // 内部实现，外部调用方不需要关心具体字段
}

// NewGarage 初始化 Garage/S3 兼容客户端。
func NewGarage(cfg *appconfig.GarageConfig, cdnCfg *appconfig.CDNConfig) (*Client, error) {
	// 对外入口保持轻量，具体初始化流程交给内部实现。
	return newGarageClient(cfg, cdnCfg)
}

// NewCachedGarage 初始化 Garage/S3 客户端，并用 Redis 缓存对象访问 URL。
func NewCachedGarage(
	cfg *appconfig.GarageConfig,
	cdnCfg *appconfig.CDNConfig,
	rdb *redis.Client,
) (*CachedObjectURLResolver, error) {
	// 先创建底层对象存储客户端，保留配置校验和错误返回。
	client, err := NewGarage(cfg, cdnCfg)
	if err != nil {
		return nil, err
	}

	// 再套上 Redis 缓存包装，调用方只需要持有 ObjectURL 能力。
	return NewCachedObjectURLResolver(client, rdb, DefaultObjectURLCacheTTL), nil
}

// ObjectURL 按配置返回对象访问 URL：启用 CDN 时返回 CDN 签名 URL，否则返回 S3 预签名 URL。
func (c *Client) ObjectURL(ctx context.Context, objectName string) (string, error) {
	// 对外方法只表达意图，具体分支逻辑由内部方法处理。
	return c.objectURL(ctx, objectName)
}

// PresignedObjectURL 生成 Garage S3 GetObject 预签名 URL。
func (c *Client) PresignedObjectURL(ctx context.Context, objectName string) (string, error) {
	// 对外方法保留显式生成 S3 预签名 URL 的能力。
	return c.presignedObjectURL(ctx, objectName)
}

// ObjectExists 判断对象 key 是否已经存在，常用于按内容摘要去重上传。
func (c *Client) ObjectExists(ctx context.Context, objectName string) (bool, error) {
	// 对外入口只表达存在性语义，S3 HeadObject 细节由内部实现处理。
	return c.objectExists(ctx, objectName)
}

// PutObject 将对象内容写入 Garage。
func (c *Client) PutObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	// 调用方负责传入最终对象 bytes；本方法不创建任何临时文件。
	return c.putObject(ctx, objectName, data, contentType)
}

// GetObject 从 Garage 直读对象内容，供管理端归一化等需要绕过 CDN 的场景使用。
func (c *Client) GetObject(ctx context.Context, objectName string) ([]byte, error) {
	return c.getObject(ctx, objectName)
}

// GetImageObject 从 Garage 直读图片原图，供 CDN 回源变换使用，读取上限高于 GetObject。
func (c *Client) GetImageObject(ctx context.Context, objectName string) ([]byte, error) {
	return c.getImageObject(ctx, objectName)
}

// ListObjectKeys 列出指定前缀下的对象 key。
func (c *Client) ListObjectKeys(ctx context.Context, prefix string) ([]string, error) {
	return c.listObjectKeys(ctx, prefix)
}

// ListObjectPage 按 key 游标有界列出对象及最后修改时间。
func (c *Client) ListObjectPage(ctx context.Context, prefix, after string, limit int) (ObjectPage, error) {
	return c.listObjectPage(ctx, prefix, after, limit)
}

// DeleteObject 从 Garage 删除对象。
func (c *Client) DeleteObject(ctx context.Context, objectName string) error {
	return c.deleteObject(ctx, objectName)
}

// MoveObject 将对象移动到新的 key。
func (c *Client) MoveObject(ctx context.Context, sourceName string, targetName string) error {
	// Garage/S3 没有原子重命名，内部使用同 bucket 复制后删除源对象。
	return c.moveObject(ctx, sourceName, targetName)
}

// CopyObject 在同 bucket 内复制对象，不删除源对象。
func (c *Client) CopyObject(ctx context.Context, sourceName string, targetName string) error {
	return c.copyObject(ctx, sourceName, targetName)
}

// ObjectKey 从对象 URL 或路径反解出对象 key。
func (c *Client) ObjectKey(value string) (string, error) {
	return c.objectKey(value)
}

// S3 返回底层 S3 客户端，供需要直接操作对象存储的 service 使用。
func (c *Client) S3() *s3.Client {
	// 只暴露已初始化的底层客户端，不允许外部改写 Client 状态。
	return c.impl.s3
}

// Bucket 返回默认 bucket 名称。
func (c *Client) Bucket() string {
	// 返回配置中的默认 bucket，便于上层构造对象路径。
	return c.impl.bucket
}

// CachedObjectURLResolver 为对象访问 URL 增加 Redis 缓存，避免同一对象反复生成不同签名 URL。
type CachedObjectURLResolver struct {
	impl *cachedObjectURLResolverImpl // 内部实现，外部调用方只需要调用 ObjectURL
}

// NewCachedObjectURLResolver 创建带 Redis 缓存的对象访问 URL 解析器。
func NewCachedObjectURLResolver(client *Client, rdb *redis.Client, ttl time.Duration) *CachedObjectURLResolver {
	// 对外入口保持薄转发，缓存 key、命中和回填逻辑交给内部实现。
	return newCachedObjectURLResolver(client, rdb, ttl)
}

// ObjectURL 返回对象访问 URL，优先读取 Redis 缓存，未命中时再生成并写回缓存。
func (r *CachedObjectURLResolver) ObjectURL(ctx context.Context, objectName string) (string, error) {
	// 对外方法只表达缓存解析语义，具体流程由内部方法处理。
	return r.objectURL(ctx, objectName)
}

// ObjectExists 判断对象 key 是否已经存在。
func (r *CachedObjectURLResolver) ObjectExists(ctx context.Context, objectName string) (bool, error) {
	return r.impl.client.ObjectExists(ctx, objectName)
}

// PutObject 将对象内容写入 Garage。
func (r *CachedObjectURLResolver) PutObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	if err := r.impl.client.PutObject(ctx, objectName, data, contentType); err != nil {
		return err
	}
	r.invalidateObjectURLCache(ctx, objectName)
	return nil
}

// GetObject 从 Garage 直读对象内容。
func (r *CachedObjectURLResolver) GetObject(ctx context.Context, objectName string) ([]byte, error) {
	return r.impl.client.GetObject(ctx, objectName)
}

// GetImageObject 从 Garage 直读图片原图，供 CDN 回源变换使用。
func (r *CachedObjectURLResolver) GetImageObject(ctx context.Context, objectName string) ([]byte, error) {
	return r.impl.client.GetImageObject(ctx, objectName)
}

// ListObjectKeys 列出指定前缀下的对象 key。
func (r *CachedObjectURLResolver) ListObjectKeys(ctx context.Context, prefix string) ([]string, error) {
	return r.impl.client.ListObjectKeys(ctx, prefix)
}

// ListObjectPage 按 key 游标有界列出对象及最后修改时间。
func (r *CachedObjectURLResolver) ListObjectPage(ctx context.Context, prefix, after string, limit int) (ObjectPage, error) {
	return r.impl.client.ListObjectPage(ctx, prefix, after, limit)
}

// DeleteObject 从 Garage 删除对象。
func (r *CachedObjectURLResolver) DeleteObject(ctx context.Context, objectName string) error {
	return r.impl.client.DeleteObject(ctx, objectName)
}

// MoveObject 将对象移动到新的 key。
func (r *CachedObjectURLResolver) MoveObject(ctx context.Context, sourceName string, targetName string) error {
	return r.impl.client.MoveObject(ctx, sourceName, targetName)
}

// CopyObject 在同 bucket 内复制对象，不删除源对象。
func (r *CachedObjectURLResolver) CopyObject(ctx context.Context, sourceName string, targetName string) error {
	return r.impl.client.CopyObject(ctx, sourceName, targetName)
}

// ObjectKey 从对象 URL 或路径反解出对象 key。
func (r *CachedObjectURLResolver) ObjectKey(value string) (string, error) {
	return r.impl.client.ObjectKey(value)
}

// CDNSigner 使用腾讯云 CDN TypeD 兼容算法生成私有读 URL。
type CDNSigner struct {
	impl *cdnSignerImpl // 内部实现，外部调用方只需要调用 SignPath
}

// NewCDNSigner 创建 CDN TypeD 签名器。
func NewCDNSigner(cfg *appconfig.CDNConfig) (*CDNSigner, error) {
	// 对外入口只负责暴露构造能力，校验和默认值由内部实现处理。
	return newCDNSigner(cfg)
}

// SignPath 为指定 CDN 文件路径生成带签名和时间戳的完整 URL。
func (s *CDNSigner) SignPath(filePath string) (string, error) {
	// 对外方法只表达签名语义，具体算法由内部方法处理。
	return s.signPath(filePath)
}
