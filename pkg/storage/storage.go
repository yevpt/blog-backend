package storage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	appconfig "github.com/vpt/blog-backend/pkg/config"
)

// 本文件是 pkg/storage 的对外入口。
// 调用方式看这里；具体实现分别在 garage.go、cdn.go、path.go 中。

// Client 封装 Garage S3 客户端和对象访问 URL 生成能力。
type Client struct {
	impl *clientImpl // 内部实现，外部调用方不需要关心具体字段
}

// NewGarage 初始化 Garage/S3 兼容客户端。
func NewGarage(cfg *appconfig.GarageConfig, cdnCfg *appconfig.CDNConfig) (*Client, error) {
	// 对外入口保持轻量，具体初始化流程交给内部实现。
	return newGarageClient(cfg, cdnCfg)
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
