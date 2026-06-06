package article

import (
	"errors"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

func (r *articleRepo) ListPublic(filter ArticleListFilter, viewerID *uint) (*ArticlePageResult, error) {
	page, pageSize := normalizeArticlePage(filter.Page, filter.PageSize)

	var total int64
	if err := r.publicArticleQuery(filter).
		Distinct("article.id").
		Count(&total).Error; err != nil {
		return nil, err
	}

	var articles []model.Article
	offset := (page - 1) * pageSize
	listQuery := r.applyArticleOrder(r.publicArticleQuery(filter), filter)
	if err := listQuery.
		Select("article.*").
		Limit(pageSize).
		Offset(offset).
		Find(&articles).Error; err != nil {
		return nil, err
	}

	aggregates, err := r.attachArticleCollections(articles, viewerID)
	if err != nil {
		return nil, err
	}

	return &ArticlePageResult{
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Articles: aggregates,
	}, nil
}

func (r *articleRepo) ListPublicIDs() ([]uint, error) {
	var ids []uint
	err := r.db.Model(&model.Article{}).
		Where("article.status = ?", uint8(1)).
		Order("article.created_at DESC").
		Order("article.id DESC").
		Pluck("id", &ids).Error
	return ids, err
}

func (r *articleRepo) FindPublicDetail(id uint, viewerID *uint) (*ArticleAggregate, error) {
	return r.findArticleDetail(id, viewerID, true)
}

func (r *articleRepo) FindAdminDetail(id uint, viewerID *uint) (*ArticleAggregate, error) {
	return r.findArticleDetail(id, viewerID, false)
}

func (r *articleRepo) IsLiked(articleID uint, userID uint) (bool, int64, error) {
	var articleCount int64
	if err := r.db.Model(&model.Article{}).
		Where("id = ? AND status IN ?", articleID, visibleArticleStatuses()).
		Count(&articleCount).Error; err != nil {
		return false, 0, err
	}
	if articleCount == 0 {
		return false, 0, gorm.ErrRecordNotFound
	}

	var likedCount int64
	if err := r.db.Model(&model.UserLike{}).
		Where("target_id = ? AND user_id = ? AND type = ?", articleID, userID, ArticleLikeType).
		Count(&likedCount).Error; err != nil {
		return false, 0, err
	}

	var total int64
	err := r.db.Model(&model.UserLike{}).
		Where("target_id = ? AND type = ?", articleID, ArticleLikeType).
		Count(&total).Error
	return likedCount > 0, total, err
}

func normalizeArticlePage(page int, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}
	return page, pageSize
}

func (r *articleRepo) publicArticleQuery(filter ArticleListFilter) *gorm.DB {
	query := r.db.Model(&model.Article{}).Where("article.status = ?", uint8(1))
	if filter.Recommend != nil && *filter.Recommend {
		query = query.Joins("JOIN article_recommend ON article_recommend.article_id = article.id AND article_recommend.deleted_at IS NULL")
	}
	if filter.CategoryID != nil {
		query = query.Joins("JOIN article_category ON article_category.article_id = article.id").
			Joins("JOIN category ON category.id = article_category.category_id AND category.deleted_at IS NULL").
			Where("article_category.category_id = ?", *filter.CategoryID)
	}
	if filter.TagID != nil {
		query = query.Joins("JOIN article_tag ON article_tag.article_id = article.id").
			Joins("JOIN tag ON tag.id = article_tag.tag_id AND tag.deleted_at IS NULL").
			Where("article_tag.tag_id = ?", *filter.TagID)
	}
	return query
}

func (r *articleRepo) applyArticleOrder(query *gorm.DB, filter ArticleListFilter) *gorm.DB {
	if filter.Recommend != nil && *filter.Recommend {
		return query.Order("article_recommend.seq ASC").
			Order("article.created_at DESC").
			Order("article.id DESC")
	}
	return query.Order("article.created_at DESC").Order("article.id DESC")
}

func (r *articleRepo) findArticleDetail(id uint, viewerID *uint, publicOnly bool) (*ArticleAggregate, error) {
	var article model.Article
	query := r.db.Model(&model.Article{}).Where("id = ?", id)
	if publicOnly {
		query = query.Where("article.status IN ?", visibleArticleStatuses())
	}
	err := query.First(&article).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	aggregates, err := r.attachArticleCollections([]model.Article{article}, viewerID)
	if err != nil {
		return nil, err
	}
	if len(aggregates) == 0 {
		return nil, nil
	}
	return &aggregates[0], nil
}

func (r *articleRepo) attachArticleCollections(articles []model.Article, viewerID *uint) ([]ArticleAggregate, error) {
	ids := articleIDs(articles)
	aggregates := make([]ArticleAggregate, 0, len(articles))
	if len(ids) == 0 {
		return aggregates, nil
	}

	likeCounts, err := r.articleLikeCounts(ids)
	if err != nil {
		return nil, err
	}
	commentCounts, err := r.articleCommentCounts(ids)
	if err != nil {
		return nil, err
	}
	recommends, err := r.articleRecommends(ids)
	if err != nil {
		return nil, err
	}
	categories, err := r.articleCategories(ids)
	if err != nil {
		return nil, err
	}
	tags, err := r.articleTags(ids)
	if err != nil {
		return nil, err
	}
	music, err := r.articleMusic(ids)
	if err != nil {
		return nil, err
	}
	users, err := r.articleUsers(articleUserIDs(articles))
	if err != nil {
		return nil, err
	}
	likedMap := map[uint]bool{}
	if viewerID != nil {
		likedMap, err = r.articleLikedMap(ids, *viewerID)
		if err != nil {
			return nil, err
		}
	}

	for _, article := range articles {
		aggregate := ArticleAggregate{
			Article:      article,
			User:         users[article.UserID],
			Categories:   categories[article.ID],
			Tags:         tags[article.ID],
			Music:        music[article.ID],
			Recommend:    recommends[article.ID],
			LikeCount:    likeCounts[article.ID],
			CommentCount: commentCounts[article.ID],
		}
		aggregate.IsLiked = likedMap[article.ID]
		aggregates = append(aggregates, aggregate)
	}
	return aggregates, nil
}

func articleUserIDs(articles []model.Article) []uint {
	seen := make(map[uint]struct{}, len(articles))
	ids := make([]uint, 0, len(articles))
	for _, article := range articles {
		if article.UserID == 0 {
			continue
		}
		if _, ok := seen[article.UserID]; ok {
			continue
		}
		seen[article.UserID] = struct{}{}
		ids = append(ids, article.UserID)
	}
	return ids
}

func (r *articleRepo) articleUsers(userIDs []uint) (map[uint]*model.User, error) {
	result := make(map[uint]*model.User, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	var users []model.User
	err := r.db.Where("id IN ?", userIDs).Find(&users).Error
	for i := range users {
		user := users[i]
		result[user.ID] = &user
	}
	return result, err
}

func articleIDs(articles []model.Article) []uint {
	ids := make([]uint, 0, len(articles))
	for _, article := range articles {
		ids = append(ids, article.ID)
	}
	return ids
}

func (r *articleRepo) articleLikeCounts(ids []uint) (map[uint]int64, error) {
	type row struct {
		TargetID uint
		Count    int64
	}
	var rows []row
	err := r.db.Model(&model.UserLike{}).
		Select("target_id, count(*) as count").
		Where("type = ? AND target_id IN ?", ArticleLikeType, ids).
		Group("target_id").
		Scan(&rows).Error
	result := make(map[uint]int64, len(rows))
	for _, row := range rows {
		result[row.TargetID] = row.Count
	}
	return result, err
}

func (r *articleRepo) articleLikedMap(ids []uint, userID uint) (map[uint]bool, error) {
	type row struct {
		TargetID uint
	}
	var rows []row
	err := r.db.Model(&model.UserLike{}).
		Select("target_id").
		Where("type = ? AND user_id = ? AND target_id IN ?", ArticleLikeType, userID, ids).
		Scan(&rows).Error
	result := make(map[uint]bool, len(rows))
	for _, row := range rows {
		result[row.TargetID] = true
	}
	return result, err
}

func (r *articleRepo) articleCommentCounts(ids []uint) (map[uint]int64, error) {
	type row struct {
		ArticleID uint
		Count     int64
	}
	var rows []row
	err := r.db.Model(&model.ArticleComment{}).
		Select("article_id, count(*) as count").
		Where("article_id IN ?", ids).
		Group("article_id").
		Scan(&rows).Error
	result := make(map[uint]int64, len(rows))
	for _, row := range rows {
		result[row.ArticleID] = row.Count
	}
	return result, err
}

func (r *articleRepo) articleRecommends(ids []uint) (map[uint]*model.ArticleRecommend, error) {
	var recommends []model.ArticleRecommend
	err := r.db.Where("article_id IN ?", ids).Find(&recommends).Error
	result := make(map[uint]*model.ArticleRecommend, len(recommends))
	for i := range recommends {
		rec := recommends[i]
		result[rec.ArticleID] = &rec
	}
	return result, err
}

type categoryJoinRow struct {
	ArticleID   uint
	ID          uint
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt
	ParentID    *uint
	Name        string
	URL         *string
	Icon        *string
	Description *string
	CoverImgUrl *string
	Seq         uint
}

func (r *articleRepo) articleCategories(ids []uint) (map[uint][]model.Category, error) {
	var rows []categoryJoinRow
	err := r.db.Table("article_category").
		Select("article_category.article_id, category.*").
		Joins("JOIN category ON category.id = article_category.category_id AND category.deleted_at IS NULL").
		Where("article_category.article_id IN ?", ids).
		Order("category.seq ASC").
		Order("category.id ASC").
		Scan(&rows).Error
	result := make(map[uint][]model.Category)
	for _, row := range rows {
		result[row.ArticleID] = append(result[row.ArticleID], model.Category{
			Base: model.Base{
				ID:        row.ID,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
				DeletedAt: row.DeletedAt,
			},
			ParentID:    row.ParentID,
			Name:        row.Name,
			URL:         row.URL,
			Icon:        row.Icon,
			Description: row.Description,
			CoverImgUrl: row.CoverImgUrl,
			Seq:         row.Seq,
		})
	}
	return result, err
}

type tagJoinRow struct {
	ArticleID   uint
	ID          uint
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt
	Name        string
	URL         *string
	Icon        *string
	Description *string
	CoverImgUrl *string
	Seq         uint
}

func (r *articleRepo) articleTags(ids []uint) (map[uint][]model.Tag, error) {
	var rows []tagJoinRow
	err := r.db.Table("article_tag").
		Select("article_tag.article_id, tag.*").
		Joins("JOIN tag ON tag.id = article_tag.tag_id AND tag.deleted_at IS NULL").
		Where("article_tag.article_id IN ?", ids).
		Order("tag.seq ASC").
		Order("tag.id ASC").
		Scan(&rows).Error
	result := make(map[uint][]model.Tag)
	for _, row := range rows {
		result[row.ArticleID] = append(result[row.ArticleID], model.Tag{
			Base: model.Base{
				ID:        row.ID,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
				DeletedAt: row.DeletedAt,
			},
			Name:        row.Name,
			URL:         row.URL,
			Icon:        row.Icon,
			Description: row.Description,
			CoverImgUrl: row.CoverImgUrl,
			Seq:         row.Seq,
		})
	}
	return result, err
}

type musicJoinRow struct {
	ArticleID   uint
	ID          uint
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt
	Name        string
	Singer      string
	Album       string
	SongDate    *time.Time
	URL         *string
	CoverImgUrl *string
	Description *string
	Lyric       *string
	Duration    uint16
	Seq         uint
}

func (r *articleRepo) articleMusic(ids []uint) (map[uint][]model.Music, error) {
	var rows []musicJoinRow
	err := r.db.Table("article_music").
		Select("article_music.article_id, music.*").
		Joins("JOIN music ON music.id = article_music.music_id AND music.deleted_at IS NULL").
		Where("article_music.article_id IN ?", ids).
		Order("music.seq ASC").
		Order("music.id ASC").
		Scan(&rows).Error
	result := make(map[uint][]model.Music)
	for _, row := range rows {
		result[row.ArticleID] = append(result[row.ArticleID], model.Music{
			Base: model.Base{
				ID:        row.ID,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
				DeletedAt: row.DeletedAt,
			},
			Name:        row.Name,
			Singer:      row.Singer,
			Album:       row.Album,
			SongDate:    row.SongDate,
			URL:         row.URL,
			CoverImgUrl: row.CoverImgUrl,
			Description: row.Description,
			Lyric:       row.Lyric,
			Duration:    row.Duration,
			Seq:         row.Seq,
		})
	}
	return result, err
}
