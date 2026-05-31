package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appconfig "github.com/vpt/blog-backend/pkg/config"
)

const defaultPresignExpires = 7 * 24 * time.Hour

// objectPresigner 抽象 S3 预签名能力，仅供内部实现和单元测试替换。
type objectPresigner interface {
	PresignGetObject(
		ctx context.Context,
		in *s3.GetObjectInput,
		optFns ...func(*s3.PresignOptions),
	) (*v4.PresignedHTTPRequest, error)
}

// clientImpl 保存 Garage 客户端运行所需的内部状态。
type clientImpl struct {
	s3             *s3.Client      // 底层 S3 客户端
	presigner      objectPresigner // S3 GetObject 预签名器
	bucket         string          // 默认 bucket 名称
	useCDN         bool            // 是否优先生成 CDN 签名 URL
	cdnSigner      *CDNSigner      // CDN 签名器
	presignExpires time.Duration   // S3 预签名 URL 有效期
}

// newGarageClient 按配置创建 Garage 客户端，并按需接入 CDN 签名器。
func newGarageClient(cfg *appconfig.GarageConfig, cdnCfg *appconfig.CDNConfig) (*Client, error) {
	// 先校验基础配置，避免 AWS SDK 初始化后才暴露配置错误。
	if err := validateGarageConfig(cfg); err != nil {
		return nil, err
	}

	// 再创建 AWS SDK 配置，后续 S3 客户端会复用该配置。
	awsCfg, err := loadAWSConfig(cfg)
	if err != nil {
		return nil, err
	}

	// 根据 Garage 和 CDN 配置组装业务客户端。
	client, err := buildClient(awsCfg, cfg, cdnCfg)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// loadAWSConfig 创建 AWS SDK 配置，Garage endpoint 由 S3 client 单独指定。
func loadAWSConfig(cfg *appconfig.GarageConfig) (aws.Config, error) {
	// endpoint 在 S3 client 选项中指定，这里只负责区域和静态 AK/SK 凭证。
	return config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
	)
}

// validateGarageConfig 校验 Garage 客户端启动所需的最小配置。
func validateGarageConfig(cfg *appconfig.GarageConfig) error {
	// 配置对象缺失时直接返回明确错误，避免后续空指针。
	if cfg == nil {
		return errors.New("Garage 配置不能为空")
	}

	// endpoint、bucket、region 是创建 S3 客户端和签名请求的必要字段。
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.Region == "" {
		return errors.New("Garage endpoint、bucket、region 不能为空")
	}

	return nil
}

// buildClient 创建 storage.Client，并在启用 CDN 时初始化 CDN 签名器。
func buildClient(awsCfg aws.Config, cfg *appconfig.GarageConfig, cdnCfg *appconfig.CDNConfig) (*Client, error) {
	// 先创建底层 S3 客户端，再基于它创建预签名器。
	s3Client := newS3Client(awsCfg, cfg.Endpoint)
	impl := &clientImpl{
		s3:             s3Client,
		presigner:      s3.NewPresignClient(s3Client),
		bucket:         cfg.Bucket,
		useCDN:         cfg.CDN,
		presignExpires: defaultPresignExpires,
	}

	// 未启用 CDN 时，客户端只需要 S3 预签名能力。
	if !cfg.CDN {
		return &Client{impl: impl}, nil
	}

	// 启用 CDN 时，初始化独立签名器用于生成私有读 URL。
	signer, err := newCDNSigner(cdnCfg)
	if err != nil {
		return nil, err
	}
	impl.cdnSigner = signer

	return &Client{impl: impl}, nil
}

// newS3Client 创建指向 Garage endpoint 的 path-style S3 客户端。
func newS3Client(awsCfg aws.Config, endpoint string) *s3.Client {
	// UsePathStyle=true 保证 Garage 使用 endpoint/bucket/key 的路由形式。
	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
}

// objectURL 根据客户端配置选择对象 URL 生成方式。
func (c *Client) objectURL(ctx context.Context, objectName string) (string, error) {
	// 统一清理对象名，避免调用方传入前导斜杠造成 key 不一致。
	objectName = normalizeObjectName(objectName)
	if objectName == "" {
		return "", nil
	}

	// 启用 CDN 时返回 CDN 签名 URL，避免暴露 S3 预签名地址。
	if c.impl.useCDN {
		if c.impl.cdnSigner == nil {
			return "", errors.New("生成对象访问 URL 失败: CDN 签名器未初始化")
		}
		return c.impl.cdnSigner.SignPath(c.fullObjectPath(objectName))
	}

	// 未启用 CDN 时，返回 Garage S3 预签名 URL。
	return c.presignedObjectURL(ctx, objectName)
}

// presignedObjectURL 生成 Garage S3 GetObject 预签名 URL。
func (c *Client) presignedObjectURL(ctx context.Context, objectName string) (string, error) {
	// 预签名器缺失说明客户端未正确初始化，直接返回可定位错误。
	if c.impl.presigner == nil {
		return "", errors.New("生成对象访问 URL 失败: S3 预签名器未初始化")
	}

	// 调用 AWS SDK 生成 GetObject 预签名请求。
	req, err := c.impl.presigner.PresignGetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(c.impl.bucket),
			Key:    aws.String(normalizeObjectName(objectName)),
		},
		s3.WithPresignExpires(c.impl.presignExpires),
	)
	if err != nil {
		return "", fmt.Errorf("生成对象访问 URL 失败: %w", err)
	}

	// SDK 返回的 URL 已包含签名参数，直接交给上层使用。
	return req.URL, nil
}
