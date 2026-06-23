# Article Assets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement article editor image upload, CDN URL normalization, temporary-object archiving, and stale image cleanup without letting DB content point at missing Garage objects.

**Architecture:** Keep HTTP binding in handlers, resource orchestration in services, and DB transactions in repositories. Garage object operations remain behind `pkg/storage` interfaces; article services use copy-first compensation and only persist normalized object keys.

**Tech Stack:** Go 1.25+, Gin, GORM, Redis rate limiting, AWS S3-compatible Garage, zap, testify, gomock, sqlmock, swaggo.

---

## File Structure

- Create `pkg/imagefile/imagefile.go`: validate uploaded image bytes, preserve original JPG/PNG/WebP/GIF bytes, compute MD5 and extension.
- Create `pkg/imagefile/imagefile_test.go`: cover valid PNG/GIF/WebP/JPG and invalid bytes.
- Modify `pkg/storage/resolver.go`: add `ObjectCopier` and `ObjectKeyResolver` interfaces.
- Modify `pkg/storage/storage.go`: expose `CopyObject` and `ObjectKey`.
- Modify `pkg/storage/garage.go`: store endpoint host and implement `copyObject`.
- Modify `pkg/storage/cache.go`: forward `CopyObject` and `ObjectKey` from cached resolver.
- Create `pkg/storage/key.go`: parse CDN/Garage URL or bare key into normalized object key.
- Create `pkg/storage/key_test.go`: cover bucket stripping, query/hash stripping, external host rejection, path traversal.
- Modify `internal/middleware/ratelimit.go`: add temp upload rate limiter with stricter normal-user limits and looser admin limits.
- Modify `internal/middleware/ratelimit_test.go`: cover normal-user block and admin relaxed path.
- Create `internal/dto/upload.go`: temp upload response DTO.
- Create `internal/service/upload/upload.go`: upload service interface and implementation.
- Create `internal/service/upload/upload_test.go`: cover dir validation, invalid image, duplicate object, successful upload.
- Create `internal/handler/upload/upload.go`: multipart binding, auth extraction, Swagger annotations.
- Create `internal/handler/upload/upload_test.go`: handler tests with `httptest`.
- Modify `internal/router/router.go`: wire upload service/handler and register `POST /uploads/temp`.
- Modify `internal/repository/article/article.go`: add `PrepareArticle` callback to `ArticleSaveData`.
- Modify `internal/repository/article/mutation.go`: call `PrepareArticle` inside transaction after article ID exists and before relation writes.
- Modify `internal/repository/article/article_test.go`: assert new article create can rewrite content after ID is allocated.
- Create `internal/service/article/asset_sync.go`: parse image references, reject external image URLs, copy temp objects to formal keys, compute cleanup.
- Create `internal/service/article/asset_sync_test.go`: focused tests for Markdown/image parsing and compensation.
- Modify `internal/service/article/assets.go`: reuse normalized key parsing for deleted-asset moves.
- Modify `internal/service/article/article.go`: call asset sync from `Save`, map new resource errors to business responses.
- Modify `internal/service/article/article_test.go`: service tests for new/edit save resource behavior.
- Modify `internal/handler/article/article.go`: map new asset errors to `response.Fail`.
- Modify `internal/router/router_test.go`: include upload handler if route handler construction tests require it.

---

### Task 1: Storage Copy And Object Key Parsing

**Files:**
- Modify: `pkg/storage/resolver.go`
- Modify: `pkg/storage/storage.go`
- Modify: `pkg/storage/garage.go`
- Modify: `pkg/storage/cache.go`
- Create: `pkg/storage/key.go`
- Test: `pkg/storage/key_test.go`

- [ ] **Step 1: Write failing storage key parser tests**

Create `pkg/storage/key_test.go` with tests like:

```go
package storage_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/storage"
)

func TestObjectKeyFromValue_StripsBucketQueryAndHash(t *testing.T) {
	parser := storage.NewObjectKeyParser(storage.ObjectKeyParserConfig{
		Bucket:       "blog",
		AllowedHosts: []string{"cdn.example.com", "garage.example.com"},
	})

	key, err := parser.ObjectKey("https://cdn.example.com/blog/articles/45/images/a.png?sign=1#x")

	require.NoError(t, err)
	assert.Equal(t, "articles/45/images/a.png", key)
}

func TestObjectKeyFromValue_RejectsExternalHost(t *testing.T) {
	parser := storage.NewObjectKeyParser(storage.ObjectKeyParserConfig{
		Bucket:       "blog",
		AllowedHosts: []string{"cdn.example.com"},
	})

	_, err := parser.ObjectKey("https://evil.example.com/a.png")

	require.ErrorIs(t, err, storage.ErrExternalObjectURL)
	assert.Contains(t, err.Error(), "evil.example.com")
}

func TestObjectKeyFromValue_RejectsPathTraversal(t *testing.T) {
	parser := storage.NewObjectKeyParser(storage.ObjectKeyParserConfig{Bucket: "blog"})

	_, err := parser.ObjectKey("/blog/articles/45/../secret.png")

	require.ErrorIs(t, err, storage.ErrInvalidObjectKey)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./pkg/storage -run 'TestObjectKeyFromValue' -count=1`

Expected: FAIL because `NewObjectKeyParser`, `ObjectKeyParserConfig`, `ErrExternalObjectURL`, and `ErrInvalidObjectKey` do not exist.

- [ ] **Step 3: Implement object key parser and copy interfaces**

Add to `pkg/storage/resolver.go`:

```go
type ObjectCopier interface {
	CopyObject(ctx context.Context, sourceName string, targetName string) error
}

type ObjectKeyResolver interface {
	ObjectKey(value string) (string, error)
}

type ObjectStore interface {
	ObjectURLResolver
	ObjectMover
	ObjectCopier
	ObjectKeyResolver
	ObjectExists(ctx context.Context, objectName string) (bool, error)
	PutObject(ctx context.Context, objectName string, data []byte, contentType string) error
	DeleteObject(ctx context.Context, objectName string) error
}
```

Create `pkg/storage/key.go`:

```go
package storage

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

var (
	ErrExternalObjectURL = errors.New("对象 URL 不属于本站")
	ErrInvalidObjectKey  = errors.New("对象 key 无效")
)

type ObjectKeyParserConfig struct {
	Bucket       string
	AllowedHosts []string
}

type ObjectKeyParser struct {
	bucket       string
	allowedHosts map[string]struct{}
}

func NewObjectKeyParser(cfg ObjectKeyParserConfig) *ObjectKeyParser {
	hosts := make(map[string]struct{}, len(cfg.AllowedHosts))
	for _, host := range cfg.AllowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
		host = strings.Trim(host, "/")
		if host != "" {
			hosts[host] = struct{}{}
		}
	}
	return &ObjectKeyParser{bucket: strings.Trim(cfg.Bucket, "/"), allowedHosts: hosts}
}

func (p *ObjectKeyParser) ObjectKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidObjectKey
	}
	rawPath := value
	if IsAbsoluteURL(value) {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", fmt.Errorf("%w: %s", ErrInvalidObjectKey, value)
		}
		host := strings.ToLower(parsed.Host)
		if _, ok := p.allowedHosts[host]; !ok {
			return "", fmt.Errorf("%w: %s", ErrExternalObjectURL, host)
		}
		rawPath = parsed.Path
	}
	rawPath, _, _ = strings.Cut(rawPath, "?")
	rawPath, _, _ = strings.Cut(rawPath, "#")
	key := strings.TrimLeft(strings.TrimSpace(rawPath), "/")
	if p.bucket != "" && strings.HasPrefix(key, p.bucket+"/") {
		key = strings.TrimPrefix(key, p.bucket+"/")
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == ".." {
			return "", fmt.Errorf("%w: %s", ErrInvalidObjectKey, value)
		}
	}
	if strings.HasSuffix(key, "/") {
		return "", fmt.Errorf("%w: %s", ErrInvalidObjectKey, value)
	}
	key = path.Clean(key)
	if key == "." || key == "/" || strings.HasPrefix(key, "../") || key == ".." {
		return "", fmt.Errorf("%w: %s", ErrInvalidObjectKey, value)
	}
	return key, nil
}
```

Add `CopyObject` and `ObjectKey` methods to `Client` and `CachedObjectURLResolver`. In `garage.go`, split `moveObject` so `copyObject` performs only S3 CopyObject and `moveObject` calls `copyObject` then `DeleteObject`.

- [ ] **Step 4: Wire parser into storage clients**

Extend `clientImpl` with:

```go
keyParser *ObjectKeyParser
```

When building the client, set:

```go
impl.keyParser = NewObjectKeyParser(ObjectKeyParserConfig{
	Bucket: cfg.Bucket,
	AllowedHosts: storageAllowedHosts(cfg.Endpoint, cdnCfg),
})
```

Implement `storageAllowedHosts` in `garage.go` using `url.Parse` for configured endpoint and CDN host.

- [ ] **Step 5: Run storage tests**

Run: `go test ./pkg/storage -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/storage
git commit -m "feat(storage): 支持对象复制与URL反解"
```

---

### Task 2: Shared Image File Validation

**Files:**
- Create: `pkg/imagefile/imagefile.go`
- Create: `pkg/imagefile/imagefile_test.go`

- [ ] **Step 1: Write failing image validation tests**

Create `pkg/imagefile/imagefile_test.go`:

```go
package imagefile_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/imagefile"
)

func TestValidate_PreservesPNGAndComputesMD5(t *testing.T) {
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&buf, img))

	result, err := imagefile.Validate("cat.png", buf.Bytes(), 10*1024*1024)

	require.NoError(t, err)
	assert.Equal(t, ".png", result.Ext)
	assert.Equal(t, "image/png", result.ContentType)
	assert.Len(t, result.MD5, 32)
	assert.Equal(t, buf.Bytes(), result.Data)
}

func TestValidate_RejectsInvalidImage(t *testing.T) {
	_, err := imagefile.Validate("cat.png", []byte("not-image"), 10*1024*1024)

	require.ErrorIs(t, err, imagefile.ErrInvalidImage)
}

func TestValidate_RejectsTooLarge(t *testing.T) {
	_, err := imagefile.Validate("cat.png", []byte("abc"), 2)

	require.ErrorIs(t, err, imagefile.ErrImageTooLarge)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./pkg/imagefile -count=1`

Expected: FAIL because package does not exist.

- [ ] **Step 3: Implement image validation helper**

Create `pkg/imagefile/imagefile.go`:

```go
package imagefile

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"image"
	"strings"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

var (
	ErrInvalidImage  = errors.New("图片格式不支持，请上传 JPG、PNG、WebP 或 GIF")
	ErrImageTooLarge = errors.New("图片不能超过限制大小")
)

type Result struct {
	Data        []byte
	ContentType string
	Ext         string
	MD5         string
}

func Validate(name string, data []byte, maxBytes int) (Result, error) {
	if len(data) == 0 {
		return Result{}, ErrInvalidImage
	}
	if maxBytes > 0 && len(data) > maxBytes {
		return Result{}, ErrImageTooLarge
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Result{}, ErrInvalidImage
	}
	contentType, ext, ok := formatInfo(format)
	if !ok {
		return Result{}, ErrInvalidImage
	}
	sum := md5.Sum(data)
	return Result{
		Data:        data,
		ContentType: contentType,
		Ext:         ext,
		MD5:         hex.EncodeToString(sum[:]),
	}, nil
}

func formatInfo(format string) (string, string, bool) {
	switch strings.ToLower(format) {
	case "jpeg":
		return "image/jpeg", ".jpg", true
	case "png":
		return "image/png", ".png", true
	case "gif":
		return "image/gif", ".gif", true
	case "webp":
		return "image/webp", ".webp", true
	default:
		return "", "", false
	}
}
```

- [ ] **Step 4: Run imagefile tests**

Run: `go test ./pkg/imagefile -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/imagefile
git commit -m "feat(imagefile): 新增上传图片校验工具"
```

---

### Task 3: Temporary Upload API And Rate Limit

**Files:**
- Modify: `internal/middleware/ratelimit.go`
- Modify: `internal/middleware/ratelimit_test.go`
- Create: `internal/dto/upload.go`
- Create: `internal/service/upload/upload.go`
- Create: `internal/service/upload/upload_test.go`
- Create: `internal/handler/upload/upload.go`
- Create: `internal/handler/upload/upload_test.go`
- Modify: `internal/router/router.go`
- Modify: `internal/router/router_test.go`

- [ ] **Step 1: Write upload service tests**

Create `internal/service/upload/upload_test.go` using a fake `storage.ObjectStore`. Include:

```go
func TestService_UploadTempImage_StoresUserScopedKey(t *testing.T) {
	store := &fakeObjectStore{urls: map[string]string{}}
	svc := uploadservice.NewService(store)

	resp, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "images",
		Name:   "cat.png",
		Data:   smallPNG(t),
	})

	require.NoError(t, err)
	assert.Regexp(t, `^temp/articles/7/images/[a-f0-9]{32}\.png$`, resp.Key)
	assert.Equal(t, resp.Key, store.puts[0].key)
	assert.Equal(t, "image/png", store.puts[0].contentType)
}

func TestService_UploadTempImage_RejectsInvalidDir(t *testing.T) {
	svc := uploadservice.NewService(&fakeObjectStore{})

	_, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "../images",
		Name:   "cat.png",
		Data:   smallPNG(t),
	})

	require.ErrorIs(t, err, uploadservice.ErrUploadDirInvalid)
}
```

- [ ] **Step 2: Run service test to verify failure**

Run: `go test ./internal/service/upload -count=1`

Expected: FAIL because package does not exist.

- [ ] **Step 3: Implement upload DTO and service**

Create `internal/dto/upload.go`:

```go
package dto

type TempUploadResp struct {
	Key string `json:"key" example:"temp/articles/7/images/d018d0f4f7b2050d9399e96f87a97b83.png"`
	URL string `json:"url" example:"https://cdn.example.com/blog/temp/articles/7/images/d018d0f4f7b2050d9399e96f87a97b83.png"`
}
```

Create `internal/service/upload/upload.go` with:

```go
const MaxTempImageBytes = 10 * 1024 * 1024

var (
	ErrUploadInvalid     = errors.New("上传图片无效")
	ErrUploadTooLarge    = errors.New("图片不能超过 10MB")
	ErrUploadDirInvalid  = errors.New("上传目录无效")
	ErrUploadUnavailable = errors.New("对象存储不可用")
)

type Service interface {
	UploadTempImage(ctx context.Context, input TempImageInput) (*dto.TempUploadResp, error)
}

type TempImageInput struct {
	UserID uint
	Dir    string
	Name   string
	Data   []byte
}
```

Allowed dirs are exactly `images` and `covers`. Object key format is:

```go
fmt.Sprintf("temp/articles/%d/%s/%s%s", input.UserID, dir, result.MD5, result.Ext)
```

Check `ObjectExists`; if false, call `PutObject`; always return `ObjectURL`.

- [ ] **Step 4: Add upload handler tests and implementation**

Create `internal/handler/upload/upload.go` with a constructor `NewHandler(svc uploadservice.Service)` and method `TempImage(c *gin.Context)`. Read a single `file` with `io.LimitReader(file, uploadservice.MaxTempImageBytes+1)`, bind `dir`, get user ID from `jwt.GetClaims(c)`, then call service.

Map errors:

```go
case errors.Is(err, uploadservice.ErrUploadTooLarge):
	response.Fail(c, response.CodeBadRequest, err.Error())
case errors.Is(err, uploadservice.ErrUploadDirInvalid), errors.Is(err, uploadservice.ErrUploadInvalid):
	response.Fail(c, response.CodeBadRequest, err.Error())
default:
	response.ServerError(c)
```

Create handler tests for missing auth, invalid dir, and success.

- [ ] **Step 5: Add temp upload rate limiter**

Modify `internal/middleware/ratelimit.go`:

```go
func RateLimitTempUpload(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := RateLimitConfig{Window: 60 * time.Second, SoftLimit: 10, HardLimit: 40, BanDuration: 15 * time.Minute}
		if detail := GetUserDetail(c); detail != nil && roles.HasPermission(detail.Roles, roles.AdminRole) {
			cfg = RateLimitConfig{Window: 60 * time.Second, SoftLimit: 60, HardLimit: 180, BanDuration: 15 * time.Minute}
		}
		applyPrincipalRateLimit(c, rdb, cfg, tempUploadRateLimitPrincipal(c), "ban:temp-upload:"+tempUploadRateLimitPrincipal(c))
	}
}
```

Refactor existing limiter body into `applyPrincipalRateLimit` so strict/normal/moment/temp share behavior. Add tests that normal user gets 429 after 10 requests and admin is still allowed at request 11.

- [ ] **Step 6: Wire route**

Modify `internal/router/router.go`:

- Add `upload *uploadhandler.Handler` to `routeHandlers`.
- In `newRouteHandlers`, create `uploadSvc := uploadservice.NewService(objectStore)` and `upload: uploadhandler.NewHandler(uploadSvc)`.
- In `registerAuthedRoutes`, add:

```go
authed.POST("/uploads/temp", middleware.RateLimitTempUpload(redisClient), handlers.upload.TempImage)
```

- [ ] **Step 7: Run upload-related tests**

Run:

```bash
go test ./pkg/imagefile ./internal/service/upload ./internal/handler/upload ./internal/middleware ./internal/router -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/imagefile internal/dto/upload.go internal/service/upload internal/handler/upload internal/middleware internal/router
git commit -m "feat(upload): 新增文章临时图片上传接口"
```

---

### Task 4: Article Repository Prepare Callback

**Files:**
- Modify: `internal/repository/article/article.go`
- Modify: `internal/repository/article/mutation.go`
- Modify: `internal/repository/article/article_test.go`

- [ ] **Step 1: Write failing repository test**

Add a test in `internal/repository/article/article_test.go`:

```go
func TestArticleRepository_Save_PreparesArticleAfterIDAllocated(t *testing.T) {
	db, mock := newArticleRepoDB(t)
	repo := articlerepo.NewArticleRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `article`").
		WillReturnResult(sqlmock.NewResult(45, 1))
	mock.ExpectExec("UPDATE `article`").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "normalized 45", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint(45)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `article_category`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `article_category`").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM `article_tag`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `article_music`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM `article_recommend`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	expectFindAdminDetail(mock, 45)

	_, err := repo.Save(articlerepo.ArticleSaveData{
		Article: model.Article{Title: "T", Content: "raw", UserID: 7, Status: 1, CommentStatus: 1},
		CategoryIDs: []uint{1},
		PrepareArticle: func(article model.Article) (model.Article, error) {
			require.Equal(t, uint(45), article.ID)
			article.Content = "normalized 45"
			return article, nil
		},
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
```

Adjust SQL expectations to the existing helper style in the file.

- [ ] **Step 2: Run repository test to verify failure**

Run: `go test ./internal/repository/article -run TestArticleRepository_Save_PreparesArticleAfterIDAllocated -count=1`

Expected: FAIL because `PrepareArticle` does not exist.

- [ ] **Step 3: Implement callback**

Modify `ArticleSaveData`:

```go
PrepareArticle func(article model.Article) (model.Article, error)
```

Modify `Save` transaction so the article ID exists before callback:

```go
if data.Article.ID == 0 {
	if err := tx.Create(&data.Article).Error; err != nil {
		return err
	}
} else {
	var existing model.Article
	if err := tx.Select("id").First(&existing, data.Article.ID).Error; err != nil {
		return err
	}
}
if data.PrepareArticle != nil {
	prepared, err := data.PrepareArticle(data.Article)
	if err != nil {
		return err
	}
	data.Article = prepared
}
res := tx.Model(&model.Article{}).
	Where("id = ?", data.Article.ID).
	Updates(articleUpdateFields(data.Article))
if res.Error != nil {
	return res.Error
}
articleID = data.Article.ID
```

- [ ] **Step 4: Run repository package tests**

Run: `go test ./internal/repository/article -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/article
git commit -m "feat(repository): 支持文章保存前归一化"
```

---

### Task 5: Article Asset Sync Service

**Files:**
- Create: `internal/service/article/asset_sync.go`
- Create: `internal/service/article/asset_sync_test.go`
- Modify: `internal/service/article/assets.go`
- Modify: `internal/service/article/article.go`
- Modify: `internal/service/article/article_test.go`
- Modify: `internal/handler/article/article.go`

- [ ] **Step 1: Write failing asset sync tests**

Create `internal/service/article/asset_sync_test.go` in package `article` to test internals:

```go
func TestNormalizeArticleContent_CopiesTempImageAndRejectsExternal(t *testing.T) {
	store := &assetStore{
		keys: map[string]bool{"temp/articles/7/images/a.png": true},
		keyMap: map[string]string{
			"https://cdn.example.com/blog/temp/articles/7/images/a.png?a=1": "temp/articles/7/images/a.png",
		},
	}

	result, err := normalizeArticleAssets(context.Background(), store, articleAssetNormalizeInput{
		ArticleID: 45,
		UserID:    7,
		Content:   `![a](https://cdn.example.com/blog/temp/articles/7/images/a.png?a=1)`,
	})

	require.NoError(t, err)
	assert.Equal(t, "![a](articles/45/images/a.png)", result.Content)
	assert.Equal(t, []articleAssetCopy{{source: "temp/articles/7/images/a.png", target: "articles/45/images/a.png"}}, store.copies)
	assert.Equal(t, []string{"temp/articles/7/images/a.png"}, result.TempKeys)
}

func TestNormalizeArticleContent_RejectsExternalImage(t *testing.T) {
	_, err := normalizeArticleAssets(context.Background(), &assetStore{}, articleAssetNormalizeInput{
		ArticleID: 45,
		UserID:    7,
		Content:   `![bad](https://example.net/a.png)`,
	})

	require.ErrorIs(t, err, ErrArticleImageExternal)
	assert.Contains(t, err.Error(), "https://example.net/a.png")
}
```

- [ ] **Step 2: Run asset sync tests to verify failure**

Run: `go test ./internal/service/article -run 'TestNormalizeArticleContent' -count=1`

Expected: FAIL because asset sync helpers and errors do not exist.

- [ ] **Step 3: Implement asset sync helpers**

Create `internal/service/article/asset_sync.go` with:

```go
var (
	ErrArticleImageExternal = errors.New("文章图片不支持外链")
	ErrArticleImageInvalid  = errors.New("文章图片无效")
	ErrArticleImageNotFound = errors.New("文章图片不存在")
)

type articleAssetNormalizeInput struct {
	ArticleID uint
	UserID    uint
	Content   string
	Cover     *string
}

type articleAssetNormalizeResult struct {
	Content        string
	Cover          *string
	TempKeys       []string
	CopiedKeys     []string
	ReferencedKeys []string
}
```

Define an internal interface:

```go
type articleAssetStore interface {
	storage.ObjectStore
}
```

Parsing rules:

- Rewrite only image references: Markdown `![alt](url)` and HTML `<img src="url">`.
- Ordinary Markdown links `[label](https://example.com)` remain unchanged.
- For absolute image URLs, call `store.ObjectKey`. If it returns `storage.ErrExternalObjectURL`, return `fmt.Errorf("%w，请先上传到本站：%s", ErrArticleImageExternal, rawURL)`.
- Allow only:
  - `temp/articles/{userID}/images/`
  - `temp/articles/{userID}/covers/`
  - `articles/{articleID}/images/`
  - `articles/{articleID}/cover/`
- Temp image target is `articles/{articleID}/images/{base}`.
- Temp cover target is `articles/{articleID}/cover/{base}`.
- If target exists, skip copy; otherwise copy and append to `CopiedKeys`.
- Always verify final target exists after copy/skip.

- [ ] **Step 4: Integrate into ArticleService.Save**

Modify `Save` in `internal/service/article/article.go`:

1. If editing, load old detail with `s.repo.FindAdminDetail(*req.ID, nil)` before saving.
2. Assert `s.objectURLResolver` implements `storage.ObjectStore`; if content has article image references and store is missing, return `ErrArticleImageInvalid`.
3. Pass `PrepareArticle` to repository. The callback calls `normalizeArticleAssets` with the saved article ID and mutates `article.Content` and `article.CoverImgUrl`.
4. Track `copiedKeys`, `tempKeys`, and `oldReferencedKeys`.
5. If `repo.Save` returns error, delete `copiedKeys`.
6. If `repo.Save` succeeds, delete `tempKeys`.
7. If editing, move old keys that are not in new referenced keys to `deleted/{key}`.

Keep cleanup methods small:

```go
func (s *articleService) deleteArticleAssetKeys(ctx context.Context, keys []string) error
func (s *articleService) moveRemovedArticleAssets(ctx context.Context, oldContent string, newKeys []string, articleID uint) error
```

- [ ] **Step 5: Map errors in handler**

Modify `writeArticleResponse`:

```go
if errors.Is(err, articleservice.ErrArticleImageExternal) ||
	errors.Is(err, articleservice.ErrArticleImageInvalid) ||
	errors.Is(err, articleservice.ErrArticleImageNotFound) {
	response.Fail(c, response.CodeBadRequest, err.Error())
	return
}
```

- [ ] **Step 6: Run service tests**

Run:

```bash
go test ./internal/service/article -run 'TestNormalizeArticleContent|TestArticleService_Save' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/service/article internal/handler/article
git commit -m "feat(article): 保存文章时归档正文图片"
```

---

### Task 6: Swagger, Integration Verification, And Final Tests

**Files:**
- Modify: `docs/`
- Verify: all files changed by earlier tasks.

- [ ] **Step 1: Run focused tests**

Run:

```bash
go test ./pkg/storage ./pkg/imagefile ./internal/middleware ./internal/service/upload ./internal/handler/upload ./internal/repository/article ./internal/service/article ./internal/handler/article ./internal/router -count=1
```

Expected: PASS.

- [ ] **Step 2: Regenerate Swagger**

Run: `make swag`

Expected: command exits 0 and generated docs include `POST /uploads/temp`.

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 4: Commit docs and final adjustments**

```bash
git add docs internal pkg
git commit -m "docs(api): 更新临时上传接口文档"
```

If `make swag` produces no docs changes because swagger is not configured in this environment, skip the commit and record the command output in the final implementation summary.

---

## Self-Review

- Spec coverage: temp upload, user-scoped temp keys, registered-user auth, admin-relaxed rate limit, copy-before-commit, new-article simple ID allocation, CDN/Garage/key parsing, external image rejection, stale image cleanup, and tests are all covered by tasks.
- Placeholder scan: no unresolved placeholders or deferred implementation steps remain.
- Type consistency: storage interfaces expose `CopyObject` and `ObjectKey`; upload service consumes `storage.ObjectStore`; article asset sync consumes `storage.ObjectStore`; repository callback is `PrepareArticle func(model.Article) (model.Article, error)`.
