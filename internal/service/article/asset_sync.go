package article

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	ErrArticleImageExternal = errors.New("文章图片不支持外链")
	ErrArticleImageInvalid  = errors.New("文章图片无效")
	ErrArticleImageNotFound = errors.New("文章图片不存在")
)

var (
	markdownImagePattern = regexp.MustCompile(`!\[[^\]]*]\(([^)]+)\)`)
	htmlImagePattern     = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc\s*=\s*['"]([^'"]+)['"][^>]*>`)
)

type articleAssetStore interface {
	storage.ObjectStore
}

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

type articleAssetCopy struct {
	source string
	target string
}

func normalizeArticleAssets(ctx context.Context, store articleAssetStore, input articleAssetNormalizeInput) (*articleAssetNormalizeResult, error) {
	if store == nil {
		return nil, ErrArticleImageInvalid
	}
	normalizer := newArticleAssetNormalizer(ctx, store, input.ArticleID, input.UserID)
	content, err := normalizer.normalizeContent(input.Content)
	if err != nil {
		return nil, err
	}
	cover, err := normalizer.normalizeCover(input.Cover)
	if err != nil {
		return nil, err
	}
	return &articleAssetNormalizeResult{
		Content:        content,
		Cover:          cover,
		TempKeys:       normalizer.tempKeys(),
		CopiedKeys:     normalizer.copiedKeys(),
		ReferencedKeys: normalizer.referencedKeys(),
	}, nil
}

type articleAssetNormalizer struct {
	ctx       context.Context
	store     articleAssetStore
	articleID uint
	userID    uint

	tempKeySet       map[string]struct{}
	copiedKeySet     map[string]struct{}
	referencedKeySet map[string]struct{}
}

func newArticleAssetNormalizer(ctx context.Context, store articleAssetStore, articleID uint, userID uint) *articleAssetNormalizer {
	return &articleAssetNormalizer{
		ctx:              ctx,
		store:            store,
		articleID:        articleID,
		userID:           userID,
		tempKeySet:       make(map[string]struct{}),
		copiedKeySet:     make(map[string]struct{}),
		referencedKeySet: make(map[string]struct{}),
	}
}

func (n *articleAssetNormalizer) normalizeContent(content string) (string, error) {
	rewritten, err := n.rewritePattern(content, markdownImagePattern, 1)
	if err != nil {
		return "", err
	}
	return n.rewritePattern(rewritten, htmlImagePattern, 1)
}

func (n *articleAssetNormalizer) normalizeCover(cover *string) (*string, error) {
	if cover == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*cover)
	if value == "" {
		return cover, nil
	}
	key, err := n.normalizeImageURL(value)
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (n *articleAssetNormalizer) rewritePattern(content string, pattern *regexp.Regexp, urlGroup int) (string, error) {
	matches := pattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}
	var builder strings.Builder
	last := 0
	for _, match := range matches {
		fullEnd := match[1]
		groupStart := match[urlGroup*2]
		groupEnd := match[urlGroup*2+1]
		if groupStart < 0 || groupEnd < 0 {
			continue
		}
		rawURL := content[groupStart:groupEnd]
		key, err := n.normalizeImageURL(rawURL)
		if err != nil {
			return "", err
		}
		builder.WriteString(content[last:groupStart])
		builder.WriteString(key)
		last = groupEnd
		if last < fullEnd {
			// 保留图片标签中 URL 之外的其他内容。
			builder.WriteString(content[last:fullEnd])
			last = fullEnd
		}
	}
	builder.WriteString(content[last:])
	return builder.String(), nil
}

func (n *articleAssetNormalizer) normalizeImageURL(rawURL string) (string, error) {
	key, err := n.objectKey(rawURL)
	if err != nil {
		return "", err
	}
	target, temp, err := n.normalizeKey(key)
	if err != nil {
		return "", err
	}
	if temp != "" {
		n.tempKeySet[temp] = struct{}{}
	}
	n.referencedKeySet[target] = struct{}{}
	return target, nil
}

func (n *articleAssetNormalizer) objectKey(rawURL string) (string, error) {
	key, err := n.store.ObjectKey(rawURL)
	if err == nil {
		return key, nil
	}
	if errors.Is(err, storage.ErrExternalObjectURL) {
		return "", fmt.Errorf("%w，请先上传到本站：%s", ErrArticleImageExternal, rawURL)
	}
	return "", fmt.Errorf("%w：%s", ErrArticleImageInvalid, rawURL)
}

func (n *articleAssetNormalizer) normalizeKey(key string) (target string, tempKey string, err error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("%w：对象 key 为空", ErrArticleImageInvalid)
	}
	tempImagePrefix := fmt.Sprintf("temp/articles/%d/images/", n.userID)
	tempCoverPrefix := fmt.Sprintf("temp/articles/%d/covers/", n.userID)
	formalImagePrefix := fmt.Sprintf("articles/%d/images/", n.articleID)
	formalCoverPrefix := fmt.Sprintf("articles/%d/cover/", n.articleID)

	switch {
	case strings.HasPrefix(key, tempImagePrefix):
		name := path.Base(key)
		target = formalImagePrefix + name
		if err := n.ensureTargetReady(key, target); err != nil {
			return "", "", err
		}
		return target, key, nil
	case strings.HasPrefix(key, tempCoverPrefix):
		name := path.Base(key)
		target = formalCoverPrefix + name
		if err := n.ensureTargetReady(key, target); err != nil {
			return "", "", err
		}
		return target, key, nil
	case strings.HasPrefix(key, formalImagePrefix), strings.HasPrefix(key, formalCoverPrefix):
		if err := n.ensureExists(key); err != nil {
			return "", "", err
		}
		return key, "", nil
	default:
		return "", "", fmt.Errorf("%w：%s", ErrArticleImageInvalid, key)
	}
}

func (n *articleAssetNormalizer) ensureTargetReady(source string, target string) error {
	exists, err := n.store.ObjectExists(n.ctx, target)
	if err != nil {
		return err
	}
	if !exists {
		if err := n.store.CopyObject(n.ctx, source, target); err != nil {
			return err
		}
		n.copiedKeySet[target] = struct{}{}
	}
	return n.ensureExists(target)
}

func (n *articleAssetNormalizer) ensureExists(key string) error {
	exists, err := n.store.ObjectExists(n.ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w：%s", ErrArticleImageNotFound, key)
	}
	return nil
}

func (n *articleAssetNormalizer) tempKeys() []string {
	return sortedKeys(n.tempKeySet)
}

func (n *articleAssetNormalizer) copiedKeys() []string {
	return sortedKeys(n.copiedKeySet)
}

func (n *articleAssetNormalizer) referencedKeys() []string {
	return sortedKeys(n.referencedKeySet)
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func hasArticleImageReferences(content string, cover *string) bool {
	if markdownImagePattern.MatchString(content) || htmlImagePattern.MatchString(content) {
		return true
	}
	return cover != nil && strings.TrimSpace(*cover) != ""
}
