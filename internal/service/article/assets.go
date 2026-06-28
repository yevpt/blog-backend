package article

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/pkg/storage"
)

type articleAssetMove struct {
	source string
	target string
}

func (s *articleService) moveDeletedArticleAssets(ctx context.Context, article *model.Article) error {
	var keyResolver storage.ObjectKeyResolver
	if resolver, ok := s.objectURLResolver.(storage.ObjectKeyResolver); ok {
		keyResolver = resolver
	}
	moves := deletedArticleAssetMoves(article, keyResolver)
	if len(moves) == 0 {
		return nil
	}

	mover, ok := s.objectURLResolver.(storage.ObjectMover)
	if !ok || mover == nil {
		return errorsNewObjectMoverUnavailable()
	}
	for _, move := range moves {
		if err := mover.MoveObject(ctx, move.source, move.target); err != nil {
			return err
		}
	}
	return nil
}

func deletedArticleAssetMoves(article *model.Article, keyResolver storage.ObjectKeyResolver) []articleAssetMove {
	if article == nil {
		return nil
	}

	moves := make([]articleAssetMove, 0)
	seen := map[string]struct{}{}
	appendMove := func(value string) {
		source := articleAssetKey(article.ID, value, keyResolver)
		if source == "" {
			return
		}
		if _, exists := seen[source]; exists {
			return
		}
		seen[source] = struct{}{}
		moves = append(moves, articleAssetMove{
			source: source,
			target: "deleted/" + source,
		})
	}

	if article.CoverImgUrl != nil {
		appendMove(*article.CoverImgUrl)
	}
	if article.MobileCoverImgUrl != nil {
		appendMove(*article.MobileCoverImgUrl)
	}
	for _, value := range articleAssetValues(article.ID, article.Content) {
		appendMove(value)
	}
	return moves
}

func articleAssetValues(articleID uint, content string) []string {
	pattern := regexp.MustCompile(fmt.Sprintf(
		`https?://[^\s"'<>()[\]]*articles/%d/[^\s"'<>()[\]]*|/?[A-Za-z0-9._~:/?#@!$&*+,;=%%-]*articles/%d/[A-Za-z0-9._~:/?#@!$&*+,;=%%-]*`,
		articleID,
		articleID,
	))
	return pattern.FindAllString(content, -1)
}

func articleAssetKey(articleID uint, value string, keyResolver storage.ObjectKeyResolver) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	key := value
	if keyResolver != nil {
		resolvedKey, err := keyResolver.ObjectKey(value)
		if err != nil {
			return ""
		}
		key = resolvedKey
	}
	key, _, _ = strings.Cut(key, "?")
	key, _, _ = strings.Cut(key, "#")
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	deletedPrefix := fmt.Sprintf("deleted/articles/%d/", articleID)
	if strings.Contains(key, deletedPrefix) {
		return ""
	}

	prefix := fmt.Sprintf("articles/%d/", articleID)
	index := strings.Index(key, prefix)
	if index < 0 {
		return ""
	}
	return key[index:]
}

func errorsNewObjectMoverUnavailable() error {
	return fmt.Errorf("对象存储不支持移动文章资源")
}
