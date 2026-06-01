package service

import (
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/repository"
)

// CategoryService 分类业务接口。
type CategoryService interface {
	ListTabs() (*dto.CategoryTabsResp, error)
}

type categoryService struct {
	repo repository.CategoryRepository
}

// NewCategoryService 创建分类业务服务实例。
func NewCategoryService(repo repository.CategoryRepository) CategoryService {
	return &categoryService{repo: repo}
}

func (s *categoryService) ListTabs() (*dto.CategoryTabsResp, error) {
	rows, err := s.repo.ListWithArticleCount()
	if err != nil {
		return nil, err
	}
	items := make([]dto.CategoryTabItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.CategoryTabItem{
			ID:           row.ID,
			Name:         row.Name,
			URL:          row.URL,
			Icon:         row.Icon,
			Description:  row.Description,
			CoverImgUrl:  row.CoverImgUrl,
			Seq:          row.Seq,
			ArticleCount: row.ArticleCount,
		})
	}
	return &dto.CategoryTabsResp{List: items}, nil
}
