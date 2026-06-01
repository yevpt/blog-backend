package service

import (
	"context"
	"math"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
)

type articleContentPolicy uint8

const (
	articleContentPublic articleContentPolicy = iota
	articleContentAdmin
)

func articlePageToDTO(result *repository.ArticlePageResult, objectURLResolver ObjectURLResolver) (*dto.ArticlePageResp, error) {
	items := make([]dto.ArticleListItemResp, 0, len(result.Articles))
	for _, aggregate := range result.Articles {
		item := articleListItemToDTO(&aggregate)
		if err := resolveListItemCoverURL(&item, objectURLResolver); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	pages := 0
	if result.Total > 0 && result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}

	return &dto.ArticlePageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}, nil
}

func resolveListItemCoverURL(item *dto.ArticleListItemResp, objectURLResolver ObjectURLResolver) error {
	// 未注入对象存储解析器时保留原值，方便纯业务测试和局部调用。
	if objectURLResolver == nil || item.CoverImgUrl == nil {
		return nil
	}

	// 空值或已经是完整 URL 时不再重复签名。
	objectName := strings.TrimSpace(*item.CoverImgUrl)
	if objectName == "" || isAbsoluteURL(objectName) {
		return nil
	}

	// 通过对象存储客户端生成 Garage 预签名或 CDN 私有签名访问 URL。
	objectURL, err := objectURLResolver.ObjectURL(context.Background(), objectName)
	if err != nil {
		return err
	}
	item.CoverImgUrl = &objectURL
	return nil
}

func isAbsoluteURL(value string) bool {
	// 已保存为外部 URL 的历史数据直接返回，避免把完整 URL 当作对象 key 处理。
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func articleDetailToDTO(aggregate *repository.ArticleAggregate, policy articleContentPolicy) *dto.ArticleDetailResp {
	item := articleListItemToDTO(aggregate)
	passworded := aggregate.Article.Status == 2
	content := ""
	if policy == articleContentAdmin || !passworded {
		content = aggregate.Article.Content
	}

	resp := &dto.ArticleDetailResp{
		ArticleListItemResp: item,
		Content:             content,
		Passworded:          passworded,
		IsLiked:             aggregate.IsLiked,
	}
	resp.CategoryIDs, resp.Categories = categoryDTOs(aggregate.Categories)
	resp.TagIDs, resp.Tags = tagDTOs(aggregate.Tags)
	resp.MusicIDs, resp.Music = musicDTOs(aggregate.Music)
	if aggregate.Recommend != nil {
		resp.IsRecommended = true
		resp.RecommendSeq = &aggregate.Recommend.Seq
	}
	return resp
}

func deletedArticleToDTO(article *model.Article) *dto.ArticleDetailResp {
	return &dto.ArticleDetailResp{ArticleListItemResp: dto.ArticleListItemResp{
		ID:            article.ID,
		Title:         article.Title,
		CoverImgUrl:   article.CoverImgUrl,
		ShortContent:  article.ShortContent,
		UserID:        article.UserID,
		Status:        article.Status,
		CommentStatus: article.CommentStatus,
		ReadCount:     article.ReadCount,
		CreatedAt:     article.CreatedAt,
		UpdatedAt:     article.UpdatedAt,
	}}
}

func articleListItemToDTO(aggregate *repository.ArticleAggregate) dto.ArticleListItemResp {
	article := aggregate.Article
	return dto.ArticleListItemResp{
		ID:            article.ID,
		Title:         article.Title,
		CoverImgUrl:   article.CoverImgUrl,
		ShortContent:  article.ShortContent,
		UserID:        article.UserID,
		Status:        article.Status,
		CommentStatus: article.CommentStatus,
		ReadCount:     article.ReadCount,
		LikeCount:     aggregate.LikeCount,
		CommentCount:  aggregate.CommentCount,
		IsRecommended: aggregate.Recommend != nil,
		CreatedAt:     article.CreatedAt,
		UpdatedAt:     article.UpdatedAt,
	}
}

func categoryDTOs(categories []model.Category) ([]uint, []dto.ArticleRelationResp) {
	ids := make([]uint, 0, len(categories))
	items := make([]dto.ArticleRelationResp, 0, len(categories))
	for _, category := range categories {
		ids = append(ids, category.ID)
		items = append(items, dto.ArticleRelationResp{
			ID:          category.ID,
			Name:        category.Name,
			URL:         category.URL,
			Icon:        category.Icon,
			Description: category.Description,
			CoverImgUrl: category.CoverImgUrl,
		})
	}
	return ids, items
}

func tagDTOs(tags []model.Tag) ([]uint, []dto.ArticleRelationResp) {
	ids := make([]uint, 0, len(tags))
	items := make([]dto.ArticleRelationResp, 0, len(tags))
	for _, tag := range tags {
		ids = append(ids, tag.ID)
		items = append(items, dto.ArticleRelationResp{
			ID:          tag.ID,
			Name:        tag.Name,
			URL:         tag.URL,
			Icon:        tag.Icon,
			Description: tag.Description,
			CoverImgUrl: tag.CoverImgUrl,
		})
	}
	return ids, items
}

func musicDTOs(music []model.Music) ([]uint, []dto.ArticleMusicResp) {
	ids := make([]uint, 0, len(music))
	items := make([]dto.ArticleMusicResp, 0, len(music))
	for _, item := range music {
		ids = append(ids, item.ID)
		items = append(items, dto.ArticleMusicResp{
			ID:          item.ID,
			Name:        item.Name,
			Singer:      item.Singer,
			Album:       item.Album,
			URL:         item.URL,
			CoverImgUrl: item.CoverImgUrl,
			Duration:    item.Duration,
		})
	}
	return ids, items
}

func cleanArticlePassword(status uint8, password *string) *string {
	if status != 2 || password == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*password)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeArticlePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizeArticlePageSize(pageSize int) int {
	if pageSize < 1 {
		return 10
	}
	if pageSize > 50 {
		return 50
	}
	return pageSize
}

func uniqueUintIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
