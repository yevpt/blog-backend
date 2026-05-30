package storage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appconfig "github.com/vpt/blog-backend/pkg/config"
)

// Client 封装 S3 客户端，用于操作 Garage 对象存储
type Client struct {
	s3     *s3.Client
	bucket string
}

// NewGarage 初始化指向 Garage 的 S3 兼容客户端
// Garage 使用 S3 协议，只需将 endpoint 指向 Garage 地址即可
func NewGarage(cfg *appconfig.StorageConfig) (*Client, error) {
	// 自定义 endpoint 解析器，将请求转发到 Garage
	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               cfg.Endpoint,
				SigningRegion:     cfg.Region,
				// Garage 使用 path 风格访问（endpoint/bucket/key）
				HostnameImmutable: true,
			}, nil
		},
	)

	awsCfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		s3:     s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true }),
		bucket: cfg.Bucket,
	}, nil
}

// S3 返回底层 S3 客户端，供上层 service 使用
func (c *Client) S3() *s3.Client { return c.s3 }

// Bucket 返回默认 bucket 名称
func (c *Client) Bucket() string { return c.bucket }
