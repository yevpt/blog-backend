package storage

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/config"
)

type fakePresigner struct {
	url     string        // mock 返回的预签名 URL
	err     error         // mock 返回的错误
	calls   int           // 记录预签名调用次数
	bucket  string        // 记录调用方传入的 bucket
	key     string        // 记录调用方传入的对象 key
	expires time.Duration // 记录调用方设置的过期时间
}

func (f *fakePresigner) PresignGetObject(
	_ context.Context,
	in *s3.GetObjectInput,
	opts ...func(*s3.PresignOptions),
) (*v4.PresignedHTTPRequest, error) {
	// 记录调用次数，用于验证缓存命中不会重复生成 URL。
	f.calls++

	// 记录请求参数，用于断言业务代码传入了正确 bucket 和 key。
	f.bucket = aws.ToString(in.Bucket)
	f.key = aws.ToString(in.Key)

	// 执行所有预签名选项，用于捕获 WithPresignExpires 设置的过期时间。
	options := &s3.PresignOptions{}
	for _, opt := range opts {
		opt(options)
	}
	f.expires = options.Expires

	// 按测试场景返回预设错误。
	if f.err != nil {
		return nil, f.err
	}

	// 按测试场景返回预设 URL。
	return &v4.PresignedHTTPRequest{URL: f.url}, nil
}

// TestCDNSignerSignPath_UsesTypeDSignature 验证 CDN URL 使用 TypeD 签名格式。
func TestCDNSignerSignPath_UsesTypeDSignature(t *testing.T) {
	// 创建固定时间的 CDN 签名器，保证签名结果可预测。
	signer, err := NewCDNSigner(&config.CDNConfig{
		Host:               "https://blog-oss.example.com",
		Secret:             "secret",
		SignQueryName:      "a",
		TimestampQueryName: "b",
	})
	require.NoError(t, err)
	signer.impl.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	// 对固定路径生成签名 URL。
	signedURL, err := signer.SignPath("/blog/images/cat.jpg")
	require.NoError(t, err)

	// 解析 URL 并校验 host、path、时间戳和签名值。
	parsed, err := url.Parse(signedURL)
	require.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "blog-oss.example.com", parsed.Host)
	assert.Equal(t, "/blog/images/cat.jpg", parsed.Path)
	assert.Equal(t, "6553f100", parsed.Query().Get("b"))
	assert.Equal(t, "8798fc7d805e9e85236adedc0ce6632a", parsed.Query().Get("a"))
}

// TestClientObjectURL_ReturnsEmptyForEmptyObjectName 验证空对象名不会生成访问 URL。
func TestClientObjectURL_ReturnsEmptyForEmptyObjectName(t *testing.T) {
	// 构造最小客户端，避免依赖真实 S3。
	client := &Client{impl: &clientImpl{bucket: "blog"}}

	// 请求空对象名。
	objectURL, err := client.ObjectURL(context.Background(), "")
	require.NoError(t, err)

	// 空对象名应返回空 URL。
	assert.Empty(t, objectURL)
}

// TestClientObjectURL_UsesCDNWhenEnabled 验证启用 CDN 时返回 CDN 签名 URL。
func TestClientObjectURL_UsesCDNWhenEnabled(t *testing.T) {
	// 创建固定时间的 CDN 签名器，避免断言受当前时间影响。
	signer, err := NewCDNSigner(&config.CDNConfig{
		Host:               "https://blog-oss.example.com",
		Secret:             "secret",
		SignQueryName:      "sign",
		TimestampQueryName: "t",
	})
	require.NoError(t, err)
	signer.impl.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	// 构造启用 CDN 的客户端。
	client := &Client{impl: &clientImpl{bucket: "blog", useCDN: true, cdnSigner: signer}}

	// 获取对象访问 URL。
	objectURL, err := client.ObjectURL(context.Background(), "/images/cat.jpg")
	require.NoError(t, err)

	// URL path 应包含 bucket，query 中应包含签名和时间戳。
	parsed, err := url.Parse(objectURL)
	require.NoError(t, err)
	assert.Equal(t, "/blog/images/cat.jpg", parsed.Path)
	assert.Equal(t, "6553f100", parsed.Query().Get("t"))
	assert.NotEmpty(t, parsed.Query().Get("sign"))
}

// TestClientObjectURL_UsesPresignWhenCDNDisabled 验证关闭 CDN 时返回 S3 预签名 URL。
func TestClientObjectURL_UsesPresignWhenCDNDisabled(t *testing.T) {
	// 使用 fakePresigner 捕获传入的 bucket、key 和过期时间。
	presigner := &fakePresigner{url: "https://garage.example.com/blog/images/cat.jpg?X-Amz-Signature=abc"}
	client := &Client{
		impl: &clientImpl{
			bucket:         "blog",
			presigner:      presigner,
			presignExpires: 15 * time.Minute,
		},
	}

	// 获取对象访问 URL。
	objectURL, err := client.ObjectURL(context.Background(), "/images/cat.jpg")
	require.NoError(t, err)

	// 返回值应来自预签名器，且传入参数应被正确清理。
	assert.Equal(t, "https://garage.example.com/blog/images/cat.jpg?X-Amz-Signature=abc", objectURL)
	assert.Equal(t, "blog", presigner.bucket)
	assert.Equal(t, "images/cat.jpg", presigner.key)
	assert.Equal(t, 15*time.Minute, presigner.expires)
}

// TestClientObjectURL_ReturnsPresignError 验证预签名失败时错误会向上返回。
func TestClientObjectURL_ReturnsPresignError(t *testing.T) {
	// 构造会返回错误的预签名器。
	presigner := &fakePresigner{err: errors.New("sign failed")}
	client := &Client{
		impl: &clientImpl{
			bucket:         "blog",
			presigner:      presigner,
			presignExpires: 15 * time.Minute,
		},
	}

	// 触发预签名 URL 生成。
	_, err := client.ObjectURL(context.Background(), "images/cat.jpg")

	// 错误应保留业务上下文，方便排查 URL 生成失败原因。
	require.Error(t, err)
	assert.ErrorContains(t, err, "生成对象访问 URL 失败")
}
