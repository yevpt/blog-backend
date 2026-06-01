package repository

// ListWithArticleCount 查询所有未删除分类及其公开文章数量。
func (r *categoryRepo) ListWithArticleCount() ([]CategoryWithCount, error) {
	var rows []CategoryWithCount
	err := r.db.Table("category").
		Select("category.*, COUNT(DISTINCT article.id) AS article_count").
		Joins("LEFT JOIN article_category ON article_category.category_id = category.id").
		Joins("LEFT JOIN article ON article.id = article_category.article_id AND article.status = 1 AND article.deleted_at IS NULL").
		Where("category.deleted_at IS NULL").
		Group("category.id").
		Order("category.seq ASC").
		Order("article_count DESC").
		Order("category.id ASC").
		Find(&rows).Error
	return rows, err
}

func (r *categoryRepo) findWithArticleCount(id uint) (*CategoryWithCount, error) {
	var row CategoryWithCount
	err := r.db.Table("category").
		Select("category.*, COUNT(DISTINCT article.id) AS article_count").
		Joins("LEFT JOIN article_category ON article_category.category_id = category.id").
		Joins("LEFT JOIN article ON article.id = article_category.article_id AND article.status = 1 AND article.deleted_at IS NULL").
		Where("category.id = ? AND category.deleted_at IS NULL", id).
		Group("category.id").
		First(&row).Error
	return &row, err
}
