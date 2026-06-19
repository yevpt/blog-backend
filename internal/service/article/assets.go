package article

import (
	"context"
	"fmt"
	"net/url"
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
	moves := deletedArticleAssetMoves(article)
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

func deletedArticleAssetMoves(article *model.Article) []articleAssetMove {
	if article == nil {
		return nil
	}

	moves := make([]articleAssetMove, 0)
	seen := map[string]struct{}{}
	appendMove := func(value string) {
		source := articleAssetKey(article.ID, value)
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

func articleAssetKey(articleID uint, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	rawPath := value
	if storage.IsAbsoluteURL(value) {
		parsed, err := url.Parse(value)
		if err == nil {
			rawPath = parsed.Path
		}
	}
	rawPath, _, _ = strings.Cut(rawPath, "?")
	rawPath, _, _ = strings.Cut(rawPath, "#")

	key := strings.TrimLeft(strings.TrimSpace(rawPath), "/")
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
