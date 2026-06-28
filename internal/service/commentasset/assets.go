package commentasset

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/vpt/blog-backend/pkg/storage"
)

var (
	ErrImageInvalid  = errors.New("评论图片无效")
	ErrImageExternal = errors.New("评论图片不支持外链")
	ErrImageNotFound = errors.New("评论图片不存在")
)

var (
	markdownImagePattern = regexp.MustCompile(`!\[[^\]]*]\(([^)]+)\)`)
	htmlImagePattern     = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc\s*=\s*['"]([^'"]+)['"][^>]*>`)
)

// ContainsImage 判断正文是否包含 Markdown 或 HTML 图片语法。
func ContainsImage(content string) bool {
	return markdownImagePattern.MatchString(content) || htmlImagePattern.MatchString(content)
}

// ImageTargets 按正文出现顺序提取 Markdown 与 HTML 图片目标。
func ImageTargets(content string) []string {
	type match struct {
		start  int
		target string
	}
	matches := make([]match, 0)
	appendPattern := func(pattern *regexp.Regexp, group int) {
		for _, indexes := range pattern.FindAllStringSubmatchIndex(content, -1) {
			start, end := indexes[group*2], indexes[group*2+1]
			if start >= 0 && end >= start {
				matches = append(matches, match{start: start, target: strings.TrimSpace(content[start:end])})
			}
		}
	}
	appendPattern(markdownImagePattern, 1)
	appendPattern(htmlImagePattern, 1)
	if len(matches) == 0 {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].start < matches[j].start })
	result := make([]string, 0, len(matches))
	for _, item := range matches {
		if item.target != "" {
			result = append(result, item.target)
		}
	}
	return result
}

type NormalizeInput struct {
	UserID       uint
	Content      string
	TargetPrefix string
}

type NormalizeResult struct {
	Content    string
	TempKeys   []string
	CopiedKeys []string
}

type normalizer struct {
	ctx          context.Context
	store        storage.ObjectStore
	userID       uint
	targetPrefix string
	tempKeySet   map[string]struct{}
	copiedKeySet map[string]struct{}
}

func Normalize(ctx context.Context, store storage.ObjectStore, input NormalizeInput) (*NormalizeResult, error) {
	if strings.TrimSpace(input.Content) == "" {
		return &NormalizeResult{Content: input.Content}, nil
	}
	if store == nil {
		if markdownImagePattern.MatchString(input.Content) || htmlImagePattern.MatchString(input.Content) {
			return nil, ErrImageInvalid
		}
		return &NormalizeResult{Content: input.Content}, nil
	}

	n := &normalizer{
		ctx:          ctx,
		store:        store,
		userID:       input.UserID,
		targetPrefix: strings.Trim(strings.TrimSpace(input.TargetPrefix), "/"),
		tempKeySet:   map[string]struct{}{},
		copiedKeySet: map[string]struct{}{},
	}
	content, err := n.normalizeContent(input.Content)
	if err != nil {
		return nil, err
	}
	return &NormalizeResult{
		Content:    content,
		TempKeys:   sortedKeys(n.tempKeySet),
		CopiedKeys: sortedKeys(n.copiedKeySet),
	}, nil
}

func ResolveContent(ctx context.Context, resolver storage.ObjectURLResolver, content string) string {
	if content == "" || resolver == nil {
		return content
	}
	content = rewriteResolvableContent(ctx, resolver, content, markdownImagePattern, 1)
	return rewriteResolvableContent(ctx, resolver, content, htmlImagePattern, 1)
}

func DeleteKeys(ctx context.Context, store storage.ObjectStore, keys []string) error {
	if store == nil || len(keys) == 0 {
		return nil
	}
	var cleanupErr error
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if err := store.DeleteObject(ctx, key); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func (n *normalizer) normalizeContent(content string) (string, error) {
	rewritten, err := n.rewritePattern(content, markdownImagePattern, 1)
	if err != nil {
		return "", err
	}
	return n.rewritePattern(rewritten, htmlImagePattern, 1)
}

func (n *normalizer) rewritePattern(content string, pattern *regexp.Regexp, urlGroup int) (string, error) {
	matches := pattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	var builder strings.Builder
	builder.Grow(len(content))
	last := 0
	for _, match := range matches {
		fullEnd := match[1]
		groupStart := match[urlGroup*2]
		groupEnd := match[urlGroup*2+1]
		if groupStart < 0 || groupEnd < 0 {
			continue
		}
		key, err := n.normalizeImageURL(content[groupStart:groupEnd])
		if err != nil {
			return "", err
		}
		builder.WriteString(content[last:groupStart])
		builder.WriteString(key)
		last = groupEnd
		if last < fullEnd {
			builder.WriteString(content[last:fullEnd])
			last = fullEnd
		}
	}
	builder.WriteString(content[last:])
	return builder.String(), nil
}

func (n *normalizer) normalizeImageURL(rawURL string) (string, error) {
	key, err := n.store.ObjectKey(rawURL)
	if err != nil {
		if errors.Is(err, storage.ErrExternalObjectURL) {
			return "", fmt.Errorf("%w，请先上传到本站：%s", ErrImageExternal, rawURL)
		}
		return "", fmt.Errorf("%w：%s", ErrImageInvalid, rawURL)
	}

	target, temp, err := n.normalizeKey(key)
	if err != nil {
		return "", err
	}
	if temp != "" {
		n.tempKeySet[temp] = struct{}{}
	}
	return target, nil
}

func (n *normalizer) normalizeKey(key string) (string, string, error) {
	key = strings.Trim(strings.TrimSpace(key), "/")
	if key == "" || n.targetPrefix == "" {
		return "", "", ErrImageInvalid
	}

	tempPrefix := fmt.Sprintf("temp/comments/%d/images/", n.userID)
	formalPrefix := n.targetPrefix + "/"
	switch {
	case strings.HasPrefix(key, tempPrefix):
		target := formalPrefix + path.Base(key)
		if err := n.ensureTargetReady(key, target); err != nil {
			return "", "", err
		}
		return target, key, nil
	case strings.HasPrefix(key, formalPrefix):
		if err := n.ensureExists(key); err != nil {
			return "", "", err
		}
		return key, "", nil
	default:
		return "", "", fmt.Errorf("%w：%s", ErrImageInvalid, key)
	}
}

func (n *normalizer) ensureTargetReady(source string, target string) error {
	if err := n.ensureExists(source); err != nil {
		return err
	}
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

func (n *normalizer) ensureExists(key string) error {
	exists, err := n.store.ObjectExists(n.ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w：%s", ErrImageNotFound, key)
	}
	return nil
}

func rewriteResolvableContent(
	ctx context.Context,
	resolver storage.ObjectURLResolver,
	content string,
	pattern *regexp.Regexp,
	urlGroup int,
) string {
	matches := pattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	var builder strings.Builder
	builder.Grow(len(content))
	last := 0
	for _, match := range matches {
		fullEnd := match[1]
		groupStart := match[urlGroup*2]
		groupEnd := match[urlGroup*2+1]
		if groupStart < 0 || groupEnd < 0 {
			continue
		}
		target := content[groupStart:groupEnd]
		resolved := resolveCommentImageTarget(ctx, resolver, target)
		builder.WriteString(content[last:groupStart])
		builder.WriteString(resolved)
		last = groupEnd
		if last < fullEnd {
			builder.WriteString(content[last:fullEnd])
			last = fullEnd
		}
	}
	builder.WriteString(content[last:])
	return builder.String()
}

func resolveCommentImageTarget(ctx context.Context, resolver storage.ObjectURLResolver, target string) string {
	key := strings.Trim(strings.TrimSpace(target), "/")
	if key == "" || storage.IsAbsoluteURL(key) ||
		(!strings.HasPrefix(key, "comments/") && !strings.HasPrefix(key, "moderation/")) {
		return target
	}
	url, err := resolver.ObjectURL(ctx, key)
	if err != nil {
		return target
	}
	return url
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
