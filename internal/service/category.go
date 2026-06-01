package service

import (
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"gorm.io/gorm"
)

var (
	ErrCategoryNotFound        = errors.New("分类不存在")
	ErrCategoryNameRequired    = errors.New("分类名称不能为空")
	ErrCategorySeqRequired     = errors.New("分类排序不能为空")
	ErrCategoryIconRequired    = errors.New("分类图标不能为空")
	ErrCategoryDescRequired    = errors.New("分类描述不能为空")
	ErrCategoryCoverRequired   = errors.New("分类封面不能为空")
	ErrCategoryArticleRequired = errors.New("分类文章不能为空")
	ErrCategoryArticleMissing  = errors.New("分类文章不存在")
)

// CategoryService 分类业务接口。
type CategoryService interface {
	ListTabs() (*dto.CategoryTabsResp, error)
	Create(req dto.CategoryCreateReq) (*dto.CategoryItemResp, error)
	Update(id uint, req dto.CategoryUpdateReq) (*dto.CategoryItemResp, error)
	Delete(id uint) (*dto.CategoryItemResp, error)
	AddArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error)
	RemoveArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error)
}

type categoryService struct {
	repo repository.CategoryRepository
}

// NewCategoryService 创建分类业务服务实例。
func NewCategoryService(repo repository.CategoryRepository) CategoryService {
	return &categoryService{repo: repo}
}

func (s *categoryService) ListTabs() (*dto.CategoryTabsResp, error) {
	// 查询分类及公开文章数量，service 不直接访问数据库。
	rows, err := s.repo.ListWithArticleCount()
	if err != nil {
		return nil, err
	}
	// 将 model 聚合转换为对外 DTO，避免暴露数据库结构。
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

func (s *categoryService) Create(req dto.CategoryCreateReq) (*dto.CategoryItemResp, error) {
	// 先清洗并校验必填字段，确保进入仓储的数据已经有业务语义。
	category, err := newCategoryFromCreateReq(req)
	if err != nil {
		return nil, err
	}
	// 调用仓储创建分类，返回带文章数量的聚合结果。
	row, err := s.repo.Create(category)
	if err != nil {
		return nil, err
	}
	return categoryWithCountToDTO(row), nil
}

func (s *categoryService) Update(id uint, req dto.CategoryUpdateReq) (*dto.CategoryItemResp, error) {
	// 把可选请求字段转换为明确的更新数据，未传字段不参与更新。
	data, err := newCategoryUpdateData(req)
	if err != nil {
		return nil, err
	}
	// 仓储返回 nil 表示目标分类不存在。
	row, err := s.repo.Update(id, data)
	if err != nil {
		return nil, mapCategoryRepoError(err)
	}
	if row == nil {
		return nil, ErrCategoryNotFound
	}
	return categoryWithCountToDTO(row), nil
}

func (s *categoryService) Delete(id uint) (*dto.CategoryItemResp, error) {
	// 删除分类只清空文章分类关系，不删除文章主表数据。
	category, err := s.repo.Delete(id)
	if err != nil {
		return nil, mapCategoryRepoError(err)
	}
	if category == nil {
		return nil, ErrCategoryNotFound
	}
	return categoryToDTO(category, 0), nil
}

func (s *categoryService) AddArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error) {
	// 归一化文章 ID，支持单个和批量，并去掉重复与 0 值。
	articleIDs, err := normalizeCategoryArticleIDs(req.ArticleIDs)
	if err != nil {
		return nil, err
	}
	// 添加文章时仓储会先清空这些文章的旧分类关系，再归入当前分类。
	affected, err := s.repo.AddArticles(id, articleIDs)
	if err != nil {
		return nil, mapCategoryRepoError(err)
	}
	return &dto.CategoryArticlesResp{CategoryID: id, ArticleIDs: articleIDs, AffectedCount: affected}, nil
}

func (s *categoryService) RemoveArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error) {
	// 归一化文章 ID，删除关系时不触碰文章本身。
	articleIDs, err := normalizeCategoryArticleIDs(req.ArticleIDs)
	if err != nil {
		return nil, err
	}
	// 仓储只删除当前分类与这些文章的关联。
	affected, err := s.repo.RemoveArticles(id, articleIDs)
	if err != nil {
		return nil, mapCategoryRepoError(err)
	}
	return &dto.CategoryArticlesResp{CategoryID: id, ArticleIDs: articleIDs, AffectedCount: affected}, nil
}

func newCategoryFromCreateReq(req dto.CategoryCreateReq) (model.Category, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return model.Category{}, ErrCategoryNameRequired
	}
	if req.Seq == nil {
		return model.Category{}, ErrCategorySeqRequired
	}
	icon := strings.TrimSpace(req.Icon)
	if icon == "" {
		return model.Category{}, ErrCategoryIconRequired
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		return model.Category{}, ErrCategoryDescRequired
	}
	cover := strings.TrimSpace(req.CoverImgUrl)
	if cover == "" {
		return model.Category{}, ErrCategoryCoverRequired
	}
	return model.Category{
		ParentID:    req.ParentID,
		Name:        name,
		URL:         cleanOptionalString(req.URL),
		Icon:        &icon,
		Description: &description,
		CoverImgUrl: &cover,
		Seq:         *req.Seq,
	}, nil
}

func newCategoryUpdateData(req dto.CategoryUpdateReq) (repository.CategoryUpdateData, error) {
	var data repository.CategoryUpdateData
	data.ParentID = req.ParentID
	data.UpdateParentID = req.ParentID != nil
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return data, ErrCategoryNameRequired
		}
		data.Name = &name
	}
	data.URL, data.UpdateURL = cleanOptionalUpdateString(req.URL)
	data.Icon, data.UpdateIcon = cleanOptionalUpdateString(req.Icon)
	data.Description, data.UpdateDescription = cleanOptionalUpdateString(req.Description)
	data.CoverImgUrl, data.UpdateCoverImgUrl = cleanOptionalUpdateString(req.CoverImgUrl)
	data.Seq = req.Seq
	return data, nil
}

func normalizeCategoryArticleIDs(ids []uint) ([]uint, error) {
	unique := uniqueCategoryArticleIDs(ids)
	if len(unique) == 0 {
		return nil, ErrCategoryArticleRequired
	}
	return unique, nil
}

func uniqueCategoryArticleIDs(ids []uint) []uint {
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

func mapCategoryRepoError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrCategoryNotFound
	}
	if errors.Is(err, repository.ErrCategoryArticleMissing) {
		return ErrCategoryArticleMissing
	}
	return err
}

func categoryWithCountToDTO(row *repository.CategoryWithCount) *dto.CategoryItemResp {
	if row == nil {
		return nil
	}
	return categoryToDTO(&row.Category, row.ArticleCount)
}

func categoryToDTO(category *model.Category, articleCount int64) *dto.CategoryItemResp {
	return &dto.CategoryItemResp{
		ID:           category.ID,
		ParentID:     category.ParentID,
		Name:         category.Name,
		URL:          category.URL,
		Icon:         category.Icon,
		Description:  category.Description,
		CoverImgUrl:  category.CoverImgUrl,
		Seq:          category.Seq,
		ArticleCount: articleCount,
	}
}
