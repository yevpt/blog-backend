package category

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/pkg/imagefile"
	uploadservice "github.com/vpt/blog-backend/internal/service/upload"
	"github.com/vpt/blog-backend/pkg/storage"
	"github.com/vpt/blog-backend/pkg/svgfile"
)

// UploadIcon 上传 SVG 图标，做 XML 白名单校验并重新编码，临时 key 包含内容哈希。
func (s *categoryService) UploadIcon(ctx context.Context, userID uint, name string, data []byte) (*dto.CategoryAssetUploadResp, error) {
	if s.store == nil {
		return nil, ErrCategoryAssetInvalid
	}

	result, err := svgfile.Validate(data)
	if err != nil {
		if errors.Is(err, svgfile.ErrSVGTooLarge) {
			return nil, fmt.Errorf("%w：SVG 图标不能超过 256KB", ErrCategoryAssetInvalid)
		}
		if errors.Is(err, svgfile.ErrSVGEmpty) {
			return nil, fmt.Errorf("%w：文件内容为空", ErrCategoryAssetInvalid)
		}
		return nil, fmt.Errorf("%w：%v", ErrCategoryAssetInvalid, err)
	}

	// 临时 key：temp/categories/{userID}/icon/{sha256}.svg，内容相同幂等复用
	key := fmt.Sprintf("temp/categories/%d/icon/%s.svg", userID, result.SHA256)
	exists, err := s.store.ObjectExists(ctx, key)
	if err != nil {
		return nil, ErrCategoryAssetInvalid
	}
	if !exists {
		if err := s.store.PutObject(ctx, key, result.Data, result.ContentType); err != nil {
			return nil, ErrCategoryAssetInvalid
		}
	}

	url, err := s.store.ObjectURL(ctx, key)
	if err != nil {
		return nil, ErrCategoryAssetInvalid
	}

	return &dto.CategoryAssetUploadResp{
		Key:  key,
		URL:  url,
		Size: int64(len(result.Data)),
		Mime: result.ContentType,
	}, nil
}

// UploadCover 上传封面位图，复用文章封面参数（10MB 读取上限、3MB 存储上限）。
func (s *categoryService) UploadCover(ctx context.Context, userID uint, name string, data []byte) (*dto.CategoryAssetUploadResp, error) {
	if s.store == nil {
		return nil, ErrCategoryAssetInvalid
	}

	validated, err := imagefile.Validate(name, data, uploadservice.MaxTempImageBytes)
	if err != nil {
		switch {
		case errors.Is(err, imagefile.ErrImageTooLarge):
			return nil, fmt.Errorf("%w：封面图片不能超过 10MB", ErrCategoryAssetInvalid)
		case errors.Is(err, imagefile.ErrImageTooManyPixels):
			return nil, fmt.Errorf("%w：%v", ErrCategoryAssetInvalid, err)
		default:
			return nil, fmt.Errorf("%w：封面格式不支持，请上传 JPG、PNG、WebP 或 GIF", ErrCategoryAssetInvalid)
		}
	}

	var storedData []byte
	var contentType string
	var ext string
	var md5hex string

	if validated.ContentType == "image/gif" {
		// GIF 沿用文章限制，不另造压缩参数
		if len(validated.Data) > uploadservice.MaxArticleTempImageStoredBytes {
			return nil, fmt.Errorf("%w：GIF 封面不能超过 3MB", ErrCategoryAssetInvalid)
		}
		storedData = validated.Data
		contentType = validated.ContentType
		ext = validated.Ext
		md5hex = validated.MD5
	} else {
		// 非 GIF：按文章封面参数压缩/转码
		stored, processErr := imagefile.PrepareForStorage(name, validated.Data, imagefile.PrepareOptions{
			MaxStoredBytes: uploadservice.MaxArticleTempImageStoredBytes,
		})
		if processErr != nil {
			return nil, fmt.Errorf("%w：封面压缩失败，请换一张更小的图片", ErrCategoryAssetInvalid)
		}
		storedData = stored.Data
		contentType = stored.ContentType
		ext = stored.Ext
		md5hex = stored.MD5
	}

	// 临时 key：temp/categories/{userID}/cover/{md5}.{ext}
	key := fmt.Sprintf("temp/categories/%d/cover/%s%s", userID, md5hex, ext)
	exists, err := s.store.ObjectExists(ctx, key)
	if err != nil {
		return nil, ErrCategoryAssetInvalid
	}
	if !exists {
		if err := s.store.PutObject(ctx, key, storedData, contentType); err != nil {
			return nil, ErrCategoryAssetInvalid
		}
	}

	url, err := s.store.ObjectURL(ctx, key)
	if err != nil {
		return nil, ErrCategoryAssetInvalid
	}

	return &dto.CategoryAssetUploadResp{
		Key:  key,
		URL:  url,
		Size: int64(len(storedData)),
		Mime: contentType,
	}, nil
}

// categoryAssetNormalizeResult 素材归一化结果。
type categoryAssetNormalizeResult struct {
	Key        string
	TempKey    string
	CopiedKeys []string
}

// normalizeCategoryIconKey 校验并归一化图标 key：
// - 本人临时 key：复制到正式目录
// - 本分类正式 key：确认存在
// - 其他：拒绝
func (s *categoryService) normalizeCategoryIconKey(
	ctx context.Context,
	userID, categoryID uint,
	rawKey string,
	previousKey *string,
) (*categoryAssetNormalizeResult, error) {
	key, err := s.resolveObjectKey(rawKey)
	if err != nil {
		return nil, err
	}
	if sameAsPrevious(key, previousKey) {
		return &categoryAssetNormalizeResult{Key: key}, nil
	}

	tempPrefix := fmt.Sprintf("temp/categories/%d/icon/", userID)
	formalPrefix := fmt.Sprintf("categories/%d/icon/", categoryID)

	switch {
	case strings.HasPrefix(key, tempPrefix):
		return s.promoteKey(ctx, key, formalPrefix+path.Base(key))
	case strings.HasPrefix(key, formalPrefix):
		return s.confirmFormalKey(ctx, key)
	case strings.HasPrefix(key, "temp/categories/"):
		return nil, fmt.Errorf("%w：图标 key 越权引用 %s", ErrCategoryAssetForbidden, key)
	case strings.HasPrefix(key, "categories/") && !strings.HasPrefix(key, formalPrefix):
		return nil, fmt.Errorf("%w：图标 key 不属于当前分类 %s", ErrCategoryAssetForbidden, key)
	default:
		return nil, fmt.Errorf("%w：不允许的图标 key %s", ErrCategoryAssetForbidden, key)
	}
}

// normalizeCategoryCoverKey 校验并归一化封面 key。
func (s *categoryService) normalizeCategoryCoverKey(
	ctx context.Context,
	userID, categoryID uint,
	rawKey string,
	previousKey *string,
) (*categoryAssetNormalizeResult, error) {
	key, err := s.resolveObjectKey(rawKey)
	if err != nil {
		return nil, err
	}
	if sameAsPrevious(key, previousKey) {
		return &categoryAssetNormalizeResult{Key: key}, nil
	}

	tempPrefix := fmt.Sprintf("temp/categories/%d/cover/", userID)
	formalPrefix := fmt.Sprintf("categories/%d/cover/", categoryID)

	switch {
	case strings.HasPrefix(key, tempPrefix):
		return s.promoteKey(ctx, key, formalPrefix+path.Base(key))
	case strings.HasPrefix(key, formalPrefix):
		return s.confirmFormalKey(ctx, key)
	case strings.HasPrefix(key, "temp/categories/"):
		return nil, fmt.Errorf("%w：封面 key 越权引用 %s", ErrCategoryAssetForbidden, key)
	case strings.HasPrefix(key, "categories/") && !strings.HasPrefix(key, formalPrefix):
		return nil, fmt.Errorf("%w：封面 key 不属于当前分类 %s", ErrCategoryAssetForbidden, key)
	default:
		return nil, fmt.Errorf("%w：不允许的封面 key %s", ErrCategoryAssetForbidden, key)
	}
}

// resolveObjectKey 将 raw 值（key 或本站 URL）解析为对象 key；拒绝外链。
func (s *categoryService) resolveObjectKey(rawValue string) (string, error) {
	if s.store == nil {
		return "", ErrCategoryAssetInvalid
	}
	key, err := s.store.ObjectKey(rawValue)
	if err == nil {
		return key, nil
	}
	if errors.Is(err, storage.ErrExternalObjectURL) {
		return "", fmt.Errorf("%w：不允许外链素材 %s", ErrCategoryAssetForbidden, rawValue)
	}
	trimmed := strings.TrimSpace(rawValue)
	if trimmed != "" && !storage.IsAbsoluteURL(trimmed) {
		return trimmed, nil
	}
	return "", fmt.Errorf("%w：素材 key 无效 %s", ErrCategoryAssetInvalid, rawValue)
}

// promoteKey 将临时对象复制到正式 key。
func (s *categoryService) promoteKey(ctx context.Context, source, target string) (*categoryAssetNormalizeResult, error) {
	exists, err := s.store.ObjectExists(ctx, target)
	if err != nil {
		return nil, ErrCategoryAssetInvalid
	}
	result := &categoryAssetNormalizeResult{TempKey: source}
	if !exists {
		if err := s.store.CopyObject(ctx, source, target); err != nil {
			return nil, ErrCategoryAssetInvalid
		}
		result.CopiedKeys = []string{target}
	}
	if err := s.ensureKeyExists(ctx, target); err != nil {
		return nil, err
	}
	result.Key = target
	return result, nil
}

// confirmFormalKey 确认正式 key 存在。
func (s *categoryService) confirmFormalKey(ctx context.Context, key string) (*categoryAssetNormalizeResult, error) {
	if err := s.ensureKeyExists(ctx, key); err != nil {
		return nil, err
	}
	return &categoryAssetNormalizeResult{Key: key}, nil
}

// ensureKeyExists 确认对象存在，否则返回 ErrCategoryAssetNotFound。
func (s *categoryService) ensureKeyExists(ctx context.Context, key string) error {
	exists, err := s.store.ObjectExists(ctx, key)
	if err != nil {
		return ErrCategoryAssetInvalid
	}
	if !exists {
		return fmt.Errorf("%w：%s", ErrCategoryAssetNotFound, key)
	}
	return nil
}

func sameAsPrevious(key string, previousKey *string) bool {
	if previousKey == nil {
		return false
	}
	previous := strings.TrimSpace(*previousKey)
	return previous != "" && key == previous
}

func replaceableCategoryIconKey(categoryID uint, key string) bool {
	prefix := fmt.Sprintf("categories/%d/icon/", categoryID)
	return strings.HasPrefix(strings.TrimSpace(key), prefix)
}

func replaceableCategoryCoverKey(categoryID uint, key string) bool {
	prefix := fmt.Sprintf("categories/%d/cover/", categoryID)
	return strings.HasPrefix(strings.TrimSpace(key), prefix)
}

// contentSHA256 计算内容 SHA256 十六进制字符串。
func contentSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
