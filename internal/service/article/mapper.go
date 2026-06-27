package article

import (
	"context"
	"math"
	"regexp"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	articlerepo "github.com/vpt/blog-backend/internal/repository/article"
	"github.com/vpt/blog-backend/pkg/storage"
)

var markdownInlineLinkPattern = regexp.MustCompile(`(!?\[[^\]]*\])\(([^)]+)\)`)

type articleContentPolicy uint8

const (
	articleContentPublic articleContentPolicy = iota
	articleContentAdmin
)

func articlePageToDTO(result *articlerepo.ArticlePageResult, objectURLResolver storage.ObjectURLResolver) (*dto.ArticlePageResp, error) {
	items := make([]dto.ArticleListItemResp, 0, len(result.Articles))
	for _, aggregate := range result.Articles {
		item := articleListItemToDTO(&aggregate)
		if err := resolveListItemCoverURL(&item, objectURLResolver); err != nil {
			return nil, err
		}
		if err := resolveArticleUserAvatarURL(item.User, objectURLResolver); err != nil {
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

func adminArticlePageToDTO(result *articlerepo.ArticlePageResult, objectURLResolver storage.ObjectURLResolver) (*dto.AdminArticlePageResp, error) {
	items := make([]dto.AdminArticleListItemResp, 0, len(result.Articles))
	for _, aggregate := range result.Articles {
		item := adminArticleListItemToDTO(&aggregate)
		if err := resolveListItemCoverURL(&item.ArticleListItemResp, objectURLResolver); err != nil {
			return nil, err
		}
		if err := resolveArticleUserAvatarURL(item.User, objectURLResolver); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	pages := 0
	if result.Total > 0 && result.PageSize > 0 {
		pages = int(math.Ceil(float64(result.Total) / float64(result.PageSize)))
	}

	return &dto.AdminArticlePageResp{
		Total:    result.Total,
		Pages:    pages,
		Page:     result.Page,
		PageSize: result.PageSize,
		List:     items,
	}, nil
}

func resolveURL(value *string, objectURLResolver storage.ObjectURLResolver) (*string, error) {
	// 未注入对象存储解析器时保留原值，方便纯业务测试和局部调用。
	if objectURLResolver == nil || value == nil {
		return value, nil
	}

	// 空值或已经是完整 URL 时不再重复签名。
	objectName := strings.TrimSpace(*value)
	if objectName == "" || storage.IsAbsoluteURL(objectName) {
		return value, nil
	}

	// 通过对象存储客户端生成 Garage 预签名或 CDN 私有签名访问 URL。
	objectURL, err := objectURLResolver.ObjectURL(context.Background(), objectName)
	if err != nil {
		return nil, err
	}
	return &objectURL, nil
}

func resolveListItemCoverURL(item *dto.ArticleListItemResp, objectURLResolver storage.ObjectURLResolver) error {
	url, err := resolveURL(item.CoverImgUrl, objectURLResolver)
	if err != nil {
		return err
	}
	item.CoverImgUrl = url
	return nil
}

func resolveArticleUserAvatarURL(user *dto.ArticleUserResp, objectURLResolver storage.ObjectURLResolver) error {
	if user == nil {
		return nil
	}
	url, err := resolveURL(user.AvatarUrl, objectURLResolver)
	if err != nil {
		return err
	}
	user.AvatarUrl = url
	return nil
}

func resolveArticleContent(content string, objectURLResolver storage.ObjectURLResolver) (string, error) {
	// 无内容或无解析器时直接返回原文，避免对非对象存储场景做无意义处理。
	if content == "" || objectURLResolver == nil {
		return content, nil
	}

	matches := markdownInlineLinkPattern.FindAllStringSubmatchIndex(content, -1)
	// 没有 Markdown 行内链接时保留原文，避免额外分配。
	if len(matches) == 0 {
		return content, nil
	}

	var builder strings.Builder
	builder.Grow(len(content))
	last := 0

	for _, match := range matches {
		labelEnd := match[3]
		targetStart, targetEnd := match[4], match[5]

		// 先写入上一个匹配之后到当前匹配之前的原文，保持正文结构不变。
		builder.WriteString(content[last:match[0]])

		target := content[targetStart:targetEnd]
		resolvedTarget, err := resolveMarkdownLinkTarget(target, objectURLResolver)
		if err != nil {
			return "", err
		}

		// 只替换链接目标地址，链接文本和是否图片链接都保留原状。
		builder.WriteString(content[match[0]:labelEnd])
		builder.WriteByte('(')
		builder.WriteString(resolvedTarget)
		builder.WriteByte(')')
		last = match[1]
	}

	builder.WriteString(content[last:])
	return builder.String(), nil
}

func resolveMarkdownLinkTarget(target string, objectURLResolver storage.ObjectURLResolver) (string, error) {
	resolved, err := resolveURL(&target, objectURLResolver)
	if err != nil {
		return "", err
	}
	if resolved == nil {
		return "", nil
	}
	return *resolved, nil
}

func articleDetailToDTO(
	aggregate *articlerepo.ArticleAggregate,
	policy articleContentPolicy,
	objectURLResolver storage.ObjectURLResolver,
	musicItems []dto.MusicItemResp,
) (*dto.ArticleDetailResp, error) {
	item := articleListItemToDTO(aggregate)
	if err := resolveListItemCoverURL(&item, objectURLResolver); err != nil {
		return nil, err
	}
	if err := resolveArticleUserAvatarURL(item.User, objectURLResolver); err != nil {
		return nil, err
	}
	passworded := aggregate.Article.Status == model.ArticleStatusEncrypted
	content := ""
	if policy == articleContentAdmin || !passworded {
		resolvedContent, err := resolveArticleContent(aggregate.Article.Content, objectURLResolver)
		if err != nil {
			return nil, err
		}
		content = resolvedContent
	}

	resp := &dto.ArticleDetailResp{
		ArticleListItemResp: item,
		Content:             content,
		Passworded:          passworded,
		IsLiked:             aggregate.IsLiked,
	}
	resp.CategoryIDs, resp.Categories = categoryDTOs(aggregate.Categories)
	resp.TagIDs, resp.Tags = tagDTOs(aggregate.Tags)
	resp.MusicIDs = musicIDs(aggregate.Music)
	resp.Music = musicItems
	if aggregate.Recommend != nil {
		resp.IsRecommended = true
		resp.RecommendSeq = &aggregate.Recommend.Seq
	}
	return resp, nil
}

func deletedArticleToDTO(article *model.Article) *dto.ArticleDetailResp {
	return &dto.ArticleDetailResp{ArticleListItemResp: dto.ArticleListItemResp{
		ID:                  article.ID,
		Title:               article.Title,
		CoverImgUrl:         article.CoverImgUrl,
		ShortContent:        article.ShortContent,
		UserID:              article.UserID,
		Status:              article.Status,
		CommentStatus:       article.CommentStatus,
		ReadCount:           article.ReadCount,
		CoverAiGenerated:    article.CoverAiGenerated,
		ContentAiReferenced: article.ContentAiReferenced,
		AiModels:            nil,
		CreatedAt:           article.CreatedAt,
		UpdatedAt:           article.UpdatedAt,
	}}
}

func categoryToRelationDTO(c model.Category) dto.ArticleRelationResp {
	return dto.ArticleRelationResp{
		ID:          c.ID,
		Name:        c.Name,
		URL:         c.URL,
		Icon:        c.Icon,
		Description: c.Description,
		CoverImgUrl: c.CoverImgUrl,
	}
}

func articleListItemToDTO(aggregate *articlerepo.ArticleAggregate) dto.ArticleListItemResp {
	article := aggregate.Article
	var category *dto.ArticleRelationResp
	// 每篇文章仅属一个分类，取索引 0
	if len(aggregate.Categories) > 0 {
		rel := categoryToRelationDTO(aggregate.Categories[0])
		category = &rel
	}
	return dto.ArticleListItemResp{
		ID:                  article.ID,
		Title:               article.Title,
		CoverImgUrl:         article.CoverImgUrl,
		ShortContent:        article.ShortContent,
		UserID:              article.UserID,
		User:                articleUserToDTO(aggregate.User),
		Status:              article.Status,
		CommentStatus:       article.CommentStatus,
		ReadCount:           article.ReadCount,
		LikeCount:           aggregate.LikeCount,
		CommentCount:        aggregate.CommentCount,
		IsLiked:             aggregate.IsLiked,
		IsRecommended:       aggregate.Recommend != nil,
		Category:            category,
		CoverAiGenerated:    article.CoverAiGenerated,
		ContentAiReferenced: article.ContentAiReferenced,
		AiModels:            articleAiModelsToDTO(article, aggregate.AiModels),
		CreatedAt:           article.CreatedAt,
		UpdatedAt:           article.UpdatedAt,
	}
}

func adminArticleListItemToDTO(aggregate *articlerepo.ArticleAggregate) dto.AdminArticleListItemResp {
	item := dto.AdminArticleListItemResp{
		ArticleListItemResp: articleListItemToDTO(aggregate),
	}
	if aggregate.Article.DeletedAt.Valid {
		deletedAt := aggregate.Article.DeletedAt.Time
		item.DeletedAt = &deletedAt
	}
	return item
}

func adminArticleDetailToDTO(
	aggregate *articlerepo.ArticleAggregate,
	policy articleContentPolicy,
	objectURLResolver storage.ObjectURLResolver,
	musicPresenter MusicItemPresenter,
) (*dto.AdminArticleDetailResp, error) {
	musicItems, err := mapArticleMusicRows(aggregate.Music, musicPresenter, objectURLResolver)
	if err != nil {
		return nil, err
	}
	detail, err := articleDetailToDTO(aggregate, policy, objectURLResolver, musicItems)
	if err != nil {
		return nil, err
	}
	resp := &dto.AdminArticleDetailResp{ArticleDetailResp: *detail}
	if aggregate.Article.DeletedAt.Valid {
		deletedAt := aggregate.Article.DeletedAt.Time
		resp.DeletedAt = &deletedAt
	}
	return resp, nil
}

func articleUserToDTO(user *model.User) *dto.ArticleUserResp {
	if user == nil {
		return nil
	}
	return &dto.ArticleUserResp{
		ID:        user.ID,
		Username:  user.Username,
		Nickname:  user.Nickname,
		AvatarUrl: user.AvatarUrl,
		Site:      user.Site,
		Mark:      user.Mark,
	}
}

func categoryDTOs(categories []model.Category) ([]uint, []dto.ArticleRelationResp) {
	ids := make([]uint, 0, len(categories))
	items := make([]dto.ArticleRelationResp, 0, len(categories))
	for _, category := range categories {
		ids = append(ids, category.ID)
		items = append(items, categoryToRelationDTO(category))
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

func musicIDs(music []model.Music) []uint {
	ids := make([]uint, 0, len(music))
	for _, item := range music {
		ids = append(ids, item.ID)
	}
	return ids
}

func mapArticleMusicRows(
	rows []model.Music,
	presenter MusicItemPresenter,
	objectURLResolver storage.ObjectURLResolver,
) ([]dto.MusicItemResp, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	if presenter != nil {
		return presenter.MusicItemsToDTO(rows)
	}
	return fallbackMusicItems(rows, objectURLResolver), nil
}

func fallbackMusicItems(rows []model.Music, objectURLResolver storage.ObjectURLResolver) []dto.MusicItemResp {
	items := make([]dto.MusicItemResp, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		display := row.ArtistDisplayName
		if display == "" {
			display = row.Singer
		}
		items = append(items, dto.MusicItemResp{
			ID:                row.ID,
			Name:              row.Name,
			ArtistDisplayName: display,
			Artists:           []dto.MusicArtistResp{},
			AlbumTrackNo:      row.AlbumTrackNo,
			AudioURL:          storage.ResolvePtrURL(objectURLResolver, row.AudioKey),
			CoverURL:          storage.ResolvePtrURL(objectURLResolver, row.CoverImgUrl),
			Duration:          row.Duration,
			IsPublic:          row.IsPublic,
			Seq:               row.Seq,
		})
	}
	return items
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

func normalizeArticleSearch(search *string) *string {
	if search == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*search)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeArticleSortBy(sortBy *string) string {
	if sortBy == nil {
		return ""
	}
	return strings.TrimSpace(*sortBy)
}

func normalizeArticleSortOrder(sortOrder *string) string {
	if sortOrder == nil {
		return "desc"
	}
	order := strings.TrimSpace(*sortOrder)
	if order == "" {
		return "desc"
	}
	return order
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

func normalizeArticleTagRelations(req dto.ArticleSaveReq) []articlerepo.ArticleTagSaveData {
	if len(req.Tags) > 0 {
		seen := make(map[uint]struct{}, len(req.Tags))
		tags := make([]articlerepo.ArticleTagSaveData, 0, len(req.Tags))
		for _, tag := range req.Tags {
			if tag.TagID == 0 {
				continue
			}
			if _, ok := seen[tag.TagID]; ok {
				continue
			}
			seen[tag.TagID] = struct{}{}
			tags = append(tags, articlerepo.ArticleTagSaveData{TagID: tag.TagID, Seq: tag.Seq})
		}
		return tags
	}

	tagIDs := uniqueUintIDs(req.TagIDs)
	tags := make([]articlerepo.ArticleTagSaveData, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		tags = append(tags, articlerepo.ArticleTagSaveData{TagID: tagID})
	}
	return tags
}

func firstCategoryID(ids []uint) []uint {
	for _, id := range ids {
		if id != 0 {
			return []uint{id}
		}
	}
	return nil
}

func normalizeArticleAiModels(req dto.ArticleSaveReq) []articlerepo.ArticleAiModelSaveData {
	includeCover := req.CoverAiGenerated
	includeContent := req.ContentAiReferenced
	if len(req.AiModels) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(req.AiModels))
	models := make([]articlerepo.ArticleAiModelSaveData, 0, len(req.AiModels))
	for _, item := range req.AiModels {
		if item.Scope == model.ArticleAiModelScopeCover && !includeCover {
			continue
		}
		if item.Scope == model.ArticleAiModelScopeContent && !includeContent {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		key := item.Scope + "\x00" + strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		models = append(models, articlerepo.ArticleAiModelSaveData{
			Scope:     item.Scope,
			ModelName: name,
			Seq:       item.Seq,
		})
	}
	return models
}

func articleAiModelsToDTO(article model.Article, models []model.ArticleAiModel) []dto.ArticleAiModelResp {
	if len(models) == 0 {
		return nil
	}
	items := make([]dto.ArticleAiModelResp, 0, len(models))
	for _, item := range models {
		if item.Scope == model.ArticleAiModelScopeCover && !article.CoverAiGenerated {
			continue
		}
		if item.Scope == model.ArticleAiModelScopeContent && !article.ContentAiReferenced {
			continue
		}
		items = append(items, dto.ArticleAiModelResp{
			Scope: item.Scope,
			Name:  item.ModelName,
			Seq:   item.Seq,
		})
	}
	if len(items) == 0 {
		return nil
	}
	return items
}
