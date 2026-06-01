package repository

// ListWithArticleCount 查询所有未删除标签及其公开文章数量。
func (r *tagRepo) ListWithArticleCount() ([]TagWithCount, error) {
	var rows []TagWithCount
	err := r.db.Table("tag").
		Select("tag.*, COUNT(DISTINCT article.id) AS article_count").
		Joins("LEFT JOIN article_tag ON article_tag.tag_id = tag.id").
		Joins("LEFT JOIN article ON article.id = article_tag.article_id AND article.status = 1 AND article.deleted_at IS NULL").
		Where("tag.deleted_at IS NULL").
		Group("tag.id").
		Order("tag.seq ASC").
		Order("article_count DESC").
		Order("tag.id ASC").
		Find(&rows).Error
	return rows, err
}

// FindWithArticleCount 查询单个未删除标签及其公开文章数量。
func (r *tagRepo) FindWithArticleCount(id uint) (*TagWithCount, error) {
	var row TagWithCount
	err := r.db.Table("tag").
		Select("tag.*, COUNT(DISTINCT article.id) AS article_count").
		Joins("LEFT JOIN article_tag ON article_tag.tag_id = tag.id").
		Joins("LEFT JOIN article ON article.id = article_tag.article_id AND article.status = 1 AND article.deleted_at IS NULL").
		Where("tag.id = ? AND tag.deleted_at IS NULL", id).
		Group("tag.id").
		First(&row).Error
	return &row, err
}
