package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	appconfig "github.com/vpt/blog-backend/pkg/config"
)

const (
	defaultPresignExpires = 7 * 24 * time.Hour
	maxObjectReadBytes    = 2 * 1024 * 1024
	// maxCDNImageReadBytes 供 CDN 回源读原图并变换，需覆盖常见封面/手机封面 PNG。
	maxCDNImageReadBytes = 20 * 1024 * 1024
)

// ErrObjectTooLarge 表示对象内容超过调用方允许读取的字节上限。
var ErrObjectTooLarge = errors.New("对象超过读取上限")

// objectPresigner 抽象 S3 预签名能力，仅供内部实现和单元测试替换。
type objectPresigner interface {
	PresignGetObject(
		ctx context.Context,
		in *s3.GetObjectInput,
		optFns ...func(*s3.PresignOptions),
	) (*v4.PresignedHTTPRequest, error)
}

// objectAPI 抽象对象存在性检查和上传能力，便于业务层复用和单元测试替换。
type objectAPI interface {
	HeadObject(ctx context.Context, in *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListObjectsV2(ctx context.Context, in *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	CopyObject(ctx context.Context, in *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// clientImpl 保存 Garage 客户端运行所需的内部状态。
type clientImpl struct {
	s3             *s3.Client       // 底层 S3 客户端
	objectAPI      objectAPI        // S3 对象读写 API
	presigner      objectPresigner  // S3 GetObject 预签名器
	bucket         string           // 默认 bucket 名称
	useCDN         bool             // 是否优先生成 CDN 签名 URL
	cdnSigner      *CDNSigner       // CDN 签名器
	presignExpires time.Duration    // S3 预签名 URL 有效期
	keyParser      *ObjectKeyParser // 对象 URL/key 反解器
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
		objectAPI:      s3Client,
		presigner:      s3.NewPresignClient(s3Client),
		bucket:         cfg.Bucket,
		useCDN:         cfg.CDN,
		presignExpires: defaultPresignExpires,
	}
	impl.keyParser = NewObjectKeyParser(ObjectKeyParserConfig{
		Bucket:       cfg.Bucket,
		AllowedHosts: storageAllowedHosts(cfg.Endpoint, cdnCfg),
	})

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

// objectExists 判断对象 key 是否已经存在。
func (c *Client) objectExists(ctx context.Context, objectName string) (bool, error) {
	if c == nil || c.impl == nil || c.impl.objectAPI == nil {
		return false, errors.New("对象存储客户端未初始化")
	}
	objectName = normalizeObjectName(objectName)
	if objectName == "" {
		return false, nil
	}

	_, err := c.impl.objectAPI.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.impl.bucket),
		Key:    aws.String(objectName),
	})
	if err == nil {
		return true, nil
	}

	if IsObjectNotFound(err) {
		return false, nil
	}
	return false, err
}

// IsObjectNotFound 兼容 AWS SDK 和 Garage/S3 endpoint 返回的多种 404 错误形态。
func IsObjectNotFound(err error) bool {
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return true
		}
	}
	return false
}

// putObject 将完整对象内容写入 Garage。
func (c *Client) putObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	return c.putObjectStream(ctx, objectName, bytes.NewReader(data), int64(len(data)), contentType)
}

func (c *Client) putObjectStream(
	ctx context.Context,
	objectName string,
	body io.Reader,
	size int64,
	contentType string,
) error {
	if c == nil || c.impl == nil || c.impl.objectAPI == nil {
		return errors.New("对象存储客户端未初始化")
	}
	objectName = normalizeObjectName(objectName)
	if objectName == "" {
		return errors.New("对象名不能为空")
	}
	if body == nil || size < 0 {
		return errors.New("对象流参数无效")
	}

	_, err := c.impl.objectAPI.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.impl.bucket),
		Key:           aws.String(objectName),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	return err
}

func (c *Client) getObject(ctx context.Context, objectName string) ([]byte, error) {
	return c.getObjectWithMaxBytes(ctx, objectName, maxObjectReadBytes)
}

func (c *Client) getImageObject(ctx context.Context, objectName string) ([]byte, error) {
	return c.getObjectWithMaxBytes(ctx, objectName, maxCDNImageReadBytes)
}

func (c *Client) getObjectWithMaxBytes(ctx context.Context, objectName string, maxBytes int) ([]byte, error) {
	body, err := c.openObject(ctx, objectName, int64(maxBytes))
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return io.ReadAll(body)
}

func (c *Client) openObject(ctx context.Context, objectName string, maxBytes int64) (io.ReadCloser, error) {
	if c == nil || c.impl == nil || c.impl.objectAPI == nil {
		return nil, errors.New("对象存储客户端未初始化")
	}
	objectName = normalizeObjectName(objectName)
	if objectName == "" {
		return nil, errors.New("对象名不能为空")
	}
	if maxBytes <= 0 {
		return nil, errors.New("读取上限无效")
	}

	out, err := c.impl.objectAPI.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.impl.bucket),
		Key:    aws.String(objectName),
	})
	if err != nil {
		return nil, err
	}
	if out == nil || out.Body == nil {
		return nil, errors.New("对象响应体为空")
	}
	if out.ContentLength != nil && *out.ContentLength > maxBytes {
		err := objectTooLargeError(maxBytes)
		if closeErr := out.Body.Close(); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}
	return &boundedObjectReadCloser{body: out.Body, remaining: maxBytes, limit: maxBytes}, nil
}

type boundedObjectReadCloser struct {
	body      io.ReadCloser
	remaining int64
	limit     int64
	closed    bool
	tooLarge  bool
}

func (r *boundedObjectReadCloser) Read(p []byte) (int, error) {
	if r.tooLarge {
		return 0, objectTooLargeError(r.limit)
	}
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}

	readSize := int64(len(p))
	if r.remaining < readSize {
		readSize = r.remaining + 1
	}
	n, err := r.body.Read(p[:int(readSize)])
	if int64(n) <= r.remaining {
		r.remaining -= int64(n)
		return n, err
	}

	delivered := int(r.remaining)
	r.remaining = 0
	r.tooLarge = true
	_ = r.closeBody()
	return delivered, objectTooLargeError(r.limit)
}

func (r *boundedObjectReadCloser) Close() error {
	return r.closeBody()
}

func (r *boundedObjectReadCloser) closeBody() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.body.Close()
}

func objectTooLargeError(maxBytes int64) error {
	return fmt.Errorf("%w: %d", ErrObjectTooLarge, maxBytes)
}

func (c *Client) listObjectKeys(ctx context.Context, prefix string) ([]string, error) {
	if c == nil || c.impl == nil || c.impl.objectAPI == nil {
		return nil, errors.New("对象存储客户端未初始化")
	}
	prefix = normalizeObjectName(prefix)
	if prefix == "" {
		return nil, nil
	}

	keys := make([]string, 0, 32)
	var continuationToken *string
	for {
		out, err := c.impl.objectAPI.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.impl.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range out.Contents {
			key := strings.TrimSpace(aws.ToString(item.Key))
			if key != "" && !strings.HasSuffix(key, "/") {
				keys = append(keys, key)
			}
		}
		if !aws.ToBool(out.IsTruncated) {
			break
		}
		continuationToken = out.NextContinuationToken
	}
	return keys, nil
}

func (c *Client) listObjectPage(ctx context.Context, prefix, after string, limit int) (ObjectPage, error) {
	if c == nil || c.impl == nil || c.impl.objectAPI == nil {
		return ObjectPage{}, errors.New("对象存储客户端未初始化")
	}
	prefix = normalizeObjectName(prefix)
	after = normalizeObjectName(after)
	if prefix == "" || limit <= 0 {
		return ObjectPage{}, errors.New("对象分页参数无效")
	}
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(c.impl.bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(int32(limit)),
	}
	if after != "" {
		input.StartAfter = aws.String(after)
	}
	out, err := c.impl.objectAPI.ListObjectsV2(ctx, input)
	if err != nil {
		return ObjectPage{}, err
	}
	page := ObjectPage{Objects: make([]ObjectMetadata, 0, len(out.Contents)), HasMore: aws.ToBool(out.IsTruncated)}
	for _, item := range out.Contents {
		key := strings.TrimSpace(aws.ToString(item.Key))
		if key == "" {
			continue
		}
		// 即使是目录占位对象也推进游标，避免分页永久卡在同一页。
		page.NextAfter = key
		if strings.HasSuffix(key, "/") {
			continue
		}
		metadata := ObjectMetadata{Key: key, Size: aws.ToInt64(item.Size)}
		if item.LastModified != nil {
			metadata.LastModified = *item.LastModified
		}
		page.Objects = append(page.Objects, metadata)
	}
	return page, nil
}

func (c *Client) deleteObject(ctx context.Context, objectName string) error {
	if c == nil || c.impl == nil || c.impl.objectAPI == nil {
		return errors.New("对象存储客户端未初始化")
	}
	objectName = normalizeObjectName(objectName)
	if objectName == "" {
		return errors.New("对象名不能为空")
	}

	_, err := c.impl.objectAPI.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.impl.bucket),
		Key:    aws.String(objectName),
	})
	return err
}

func (c *Client) copyObject(ctx context.Context, sourceName string, targetName string) error {
	if c == nil || c.impl == nil || c.impl.objectAPI == nil {
		return errors.New("对象存储客户端未初始化")
	}
	sourceName = normalizeObjectName(sourceName)
	targetName = normalizeObjectName(targetName)
	if sourceName == "" || targetName == "" {
		return errors.New("对象名不能为空")
	}
	if sourceName == targetName {
		return nil
	}

	_, err := c.impl.objectAPI.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(c.impl.bucket),
		Key:        aws.String(targetName),
		CopySource: aws.String(copySource(c.impl.bucket, sourceName)),
	})
	return err
}

func (c *Client) moveObject(ctx context.Context, sourceName string, targetName string) error {
	sourceName = normalizeObjectName(sourceName)
	targetName = normalizeObjectName(targetName)
	if sourceName == targetName {
		return nil
	}
	if err := c.copyObject(ctx, sourceName, targetName); err != nil {
		return err
	}
	_, err := c.impl.objectAPI.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.impl.bucket),
		Key:    aws.String(sourceName),
	})
	return err
}

func (c *Client) objectKey(value string) (string, error) {
	if c == nil || c.impl == nil || c.impl.keyParser == nil {
		return "", errors.New("对象存储客户端未初始化")
	}
	return c.impl.keyParser.ObjectKey(value)
}

func storageAllowedHosts(endpoint string, cdnCfg *appconfig.CDNConfig) []string {
	var hosts []string
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Host != "" {
		hosts = append(hosts, parsed.Host)
	}
	if cdnCfg != nil && cdnCfg.Host != "" {
		if parsed, err := url.Parse(cdnCfg.Host); err == nil && parsed.Host != "" {
			hosts = append(hosts, parsed.Host)
		}
	}
	return hosts
}

func copySource(bucket string, key string) string {
	parts := strings.Split(normalizeObjectName(key), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Trim(bucket, "/") + "/" + strings.Join(parts, "/")
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
