package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
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

type fakeObjectAPI struct {
	headErr      error
	getErr       error
	listErr      error
	putErr       error
	copyErr      error
	deleteErr    error
	headBucket   string
	headKey      string
	getBucket    string
	getKey       string
	getBody      []byte
	getOutput    *s3.GetObjectOutput
	listPrefix   string
	listKeys     []string
	putBucket    string
	putKey       string
	putType      string
	putBody      []byte
	putReader    io.Reader
	putSize      int64
	copyBucket   string
	copyKey      string
	copySource   string
	deleteBucket string
	deleteKey    string
	headCalls    int
	getCalls     int
	listCalls    int
	putCalls     int
	copyCalls    int
	deleteCalls  int
}

func (f *fakeObjectAPI) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	f.headCalls++
	f.headBucket = aws.ToString(in.Bucket)
	f.headKey = aws.ToString(in.Key)
	if f.headErr != nil {
		return nil, f.headErr
	}
	return &s3.HeadObjectOutput{}, nil
}

func (f *fakeObjectAPI) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.getCalls++
	f.getBucket = aws.ToString(in.Bucket)
	f.getKey = aws.ToString(in.Key)
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getOutput != nil {
		return f.getOutput, nil
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(f.getBody))}, nil
}

func (f *fakeObjectAPI) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.listCalls++
	f.listPrefix = aws.ToString(in.Prefix)
	if f.listErr != nil {
		return nil, f.listErr
	}
	contents := make([]types.Object, 0, len(f.listKeys))
	for _, key := range f.listKeys {
		contents = append(contents, types.Object{Key: aws.String(key)})
	}
	return &s3.ListObjectsV2Output{Contents: contents}, nil
}

func (f *fakeObjectAPI) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putCalls++
	f.putBucket = aws.ToString(in.Bucket)
	f.putKey = aws.ToString(in.Key)
	f.putType = aws.ToString(in.ContentType)
	f.putReader = in.Body
	f.putSize = aws.ToInt64(in.ContentLength)
	body, err := io.ReadAll(in.Body)
	if err == nil {
		f.putBody = body
	}
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &s3.PutObjectOutput{}, nil
}

type countingReader struct {
	io.Reader
	readBytes int
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.readBytes += n
	return n, err
}

type trackedReadCloser struct {
	io.Reader
	closeCount int
}

func (r *trackedReadCloser) Close() error {
	r.closeCount++
	return nil
}

func (f *fakeObjectAPI) CopyObject(_ context.Context, in *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	f.copyCalls++
	f.copyBucket = aws.ToString(in.Bucket)
	f.copyKey = aws.ToString(in.Key)
	f.copySource = aws.ToString(in.CopySource)
	if f.copyErr != nil {
		return nil, f.copyErr
	}
	return &s3.CopyObjectOutput{}, nil
}

func (f *fakeObjectAPI) DeleteObject(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.deleteCalls++
	f.deleteBucket = aws.ToString(in.Bucket)
	f.deleteKey = aws.ToString(in.Key)
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &s3.DeleteObjectOutput{}, nil
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

func TestClientObjectExists_ReturnsTrue(t *testing.T) {
	api := &fakeObjectAPI{}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	exists, err := client.ObjectExists(context.Background(), "/avatar/user/a.jpg")
	log.Println(exists)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "blog", api.headBucket)
	assert.Equal(t, "avatar/user/a.jpg", api.headKey)
}

func TestClientObjectExists_ReturnsFalseForNotFound(t *testing.T) {
	api := &fakeObjectAPI{headErr: &types.NotFound{}}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	exists, err := client.ObjectExists(context.Background(), "avatar/user/missing.jpg")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestClientObjectExists_ReturnsFalseForGenericNotFound(t *testing.T) {
	api := &fakeObjectAPI{headErr: &smithy.GenericAPIError{Code: "NotFound", Message: "not found"}}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	exists, err := client.ObjectExists(context.Background(), "avatar/user/missing.jpg")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestClientPutObject_UploadsBytes(t *testing.T) {
	api := &fakeObjectAPI{}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	err := client.PutObject(context.Background(), "avatar/user/a.jpg", []byte("image"), "image/jpeg")

	require.NoError(t, err)
	assert.Equal(t, 1, api.putCalls)
	assert.Equal(t, "blog", api.putBucket)
	assert.Equal(t, "avatar/user/a.jpg", api.putKey)
	assert.Equal(t, "image/jpeg", api.putType)
	assert.Equal(t, []byte("image"), api.putBody)
}

func TestPutObjectStreamForwardsReaderWithoutReadAll(t *testing.T) {
	api := &fakeObjectAPI{}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}
	source := &countingReader{Reader: strings.NewReader("rules")}

	err := client.PutObjectStream(context.Background(), "moderation/rules.csv", source, 5, "text/csv")

	require.NoError(t, err)
	assert.Same(t, source, api.putReader)
	assert.Equal(t, 5, source.readBytes)
	assert.Equal(t, int64(5), api.putSize)
}

func TestOpenObjectRejectsContentLengthAboveLimitAndClosesBody(t *testing.T) {
	body := &trackedReadCloser{Reader: strings.NewReader("oversized")}
	api := &fakeObjectAPI{getOutput: &s3.GetObjectOutput{Body: body, ContentLength: aws.Int64(9)}}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	reader, err := client.OpenObject(context.Background(), "x", 8)

	assert.Nil(t, reader)
	assert.ErrorIs(t, err, ErrObjectTooLarge)
	assert.Equal(t, 1, body.closeCount)
}

func TestOpenObjectRejectsStreamingOverflowAndClosesBody(t *testing.T) {
	body := &trackedReadCloser{Reader: strings.NewReader("oversized")}
	api := &fakeObjectAPI{getOutput: &s3.GetObjectOutput{Body: body}}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	reader, err := client.OpenObject(context.Background(), "x", 8)
	require.NoError(t, err)
	data, err := io.ReadAll(reader)

	assert.Equal(t, "oversize", string(data))
	assert.ErrorIs(t, err, ErrObjectTooLarge)
	assert.Equal(t, 1, body.closeCount)
	require.NoError(t, reader.Close())
	assert.Equal(t, 1, body.closeCount)
}

func TestClientDeleteObject_RemovesObject(t *testing.T) {
	api := &fakeObjectAPI{}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	err := client.DeleteObject(context.Background(), "/moments/a.jpg")

	require.NoError(t, err)
	assert.Equal(t, 1, api.deleteCalls)
	assert.Equal(t, "blog", api.deleteBucket)
	assert.Equal(t, "moments/a.jpg", api.deleteKey)
}

func TestClientMoveObject_CopiesThenDeletesSource(t *testing.T) {
	api := &fakeObjectAPI{}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	err := client.MoveObject(context.Background(), "/articles/9/images/a b.png", "deleted/articles/9/images/a b.png")

	require.NoError(t, err)
	assert.Equal(t, 1, api.copyCalls)
	assert.Equal(t, 1, api.deleteCalls)
	assert.Equal(t, "blog", api.copyBucket)
	assert.Equal(t, "deleted/articles/9/images/a b.png", api.copyKey)
	assert.Equal(t, "blog/articles/9/images/a%20b.png", api.copySource)
	assert.Equal(t, "blog", api.deleteBucket)
	assert.Equal(t, "articles/9/images/a b.png", api.deleteKey)
}

func TestClientGetObject_RejectsContentOverDefaultLimit(t *testing.T) {
	body := bytes.Repeat([]byte("a"), maxObjectReadBytes+1)
	api := &fakeObjectAPI{getBody: body}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	_, err := client.GetObject(context.Background(), "articles/1/cover/big.png")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrObjectTooLarge)
}

func TestClientGetImageObject_AllowsContentAboveDefaultLimit(t *testing.T) {
	body := bytes.Repeat([]byte("a"), maxObjectReadBytes+1)
	api := &fakeObjectAPI{getBody: body}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	got, err := client.GetImageObject(context.Background(), "articles/1/mobile-cover/big.png")

	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestClientGetImageObject_RejectsContentOverImageLimit(t *testing.T) {
	body := bytes.Repeat([]byte("a"), maxCDNImageReadBytes+1)
	api := &fakeObjectAPI{getBody: body}
	client := &Client{impl: &clientImpl{bucket: "blog", objectAPI: api}}

	_, err := client.GetImageObject(context.Background(), "articles/1/mobile-cover/huge.png")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrObjectTooLarge)
}
