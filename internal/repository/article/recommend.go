package article

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

const maxRecommendedArticles = 100

// ErrRecommendOrderMismatch 表示推荐排序请求与当前推荐集合不一致。
var ErrRecommendOrderMismatch = errors.New("推荐排序与当前推荐文章不一致")

// RecommendArticleRow 推荐文章查询行，供 service 映射为 dto。
type RecommendArticleRow struct {
	Article model.Article
	Seq     uint
}

// ListRecommended 查询未软删除的推荐文章，按 seq 升序排列。
func (r *articleRepo) ListRecommended() ([]RecommendArticleRow, error) {
	var rows []struct {
		model.Article
		Seq uint `gorm:"column:seq"`
	}
	err := r.db.Model(&model.Article{}).
		Select("article.*, article_recommend.seq").
		Joins("JOIN article_recommend ON article_recommend.article_id = article.id AND article_recommend.deleted_at IS NULL").
		Where("article.deleted_at IS NULL").
		Order("article_recommend.seq ASC").
		Order("article.id DESC").
		Limit(maxRecommendedArticles).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make([]RecommendArticleRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, RecommendArticleRow{
			Article: row.Article,
			Seq:     row.Seq,
		})
	}
	return result, nil
}

// ReorderRecommended 按给定顺序批量更新推荐 seq；须与当前推荐集合完全一致。
func (r *articleRepo) ReorderRecommended(articleIDs []uint) error {
	existing, err := r.listRecommendedArticleIDs()
	if err != nil {
		return err
	}
	if len(existing) != len(articleIDs) {
		return ErrRecommendOrderMismatch
	}

	existingSet := make(map[uint]struct{}, len(existing))
	for _, id := range existing {
		existingSet[id] = struct{}{}
	}
	seen := make(map[uint]struct{}, len(articleIDs))
	for _, id := range articleIDs {
		if _, ok := existingSet[id]; !ok {
			return ErrRecommendOrderMismatch
		}
		if _, dup := seen[id]; dup {
			return ErrRecommendOrderMismatch
		}
		seen[id] = struct{}{}
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		for i, articleID := range articleIDs {
			seq := uint(i * 10)
			if err := tx.Model(&model.ArticleRecommend{}).
				Where("article_id = ?", articleID).
				Update("seq", seq).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *articleRepo) listRecommendedArticleIDs() ([]uint, error) {
	var ids []uint
	err := r.db.Model(&model.ArticleRecommend{}).
		Select("article_recommend.article_id").
		Joins("JOIN article ON article.id = article_recommend.article_id AND article.deleted_at IS NULL").
		Where("article_recommend.deleted_at IS NULL").
		Order("article_recommend.seq ASC").
		Order("article_recommend.article_id ASC").
		Pluck("article_id", &ids).Error
	return ids, err
}

// nextRecommendSeq 计算新推荐文章应使用的 seq，追加到当前队尾。
func nextRecommendSeq(tx *gorm.DB) (uint, error) {
	query := tx.Model(&model.ArticleRecommend{}).
		Joins("JOIN article ON article.id = article_recommend.article_id AND article.deleted_at IS NULL").
		Where("article_recommend.deleted_at IS NULL")

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}

	var maxSeq uint
	if err := query.Select("COALESCE(MAX(article_recommend.seq), 0)").Scan(&maxSeq).Error; err != nil {
		return 0, err
	}
	return maxSeq + 10, nil
}
