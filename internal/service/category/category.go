package category

import (
	"context"
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	categoryrepo "github.com/vpt/blog-backend/internal/repository/category"
	"github.com/vpt/blog-backend/pkg/storage"
	"github.com/vpt/blog-backend/pkg/strutil"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrCategoryNotFound        = errors.New("分类不存在")
	ErrCategoryNameRequired    = errors.New("分类名称不能为空")
	ErrCategorySeqRequired     = errors.New("分类排序不能为空")
	ErrCategoryArticleRequired = errors.New("分类文章不能为空")
	ErrCategoryArticleMissing  = errors.New("分类文章不存在")
	ErrCategoryAssetInvalid    = errors.New("分类素材无效")
	ErrCategoryAssetNotFound   = errors.New("分类素材不存在")
	ErrCategoryAssetForbidden  = errors.New("分类素材越权或不属于本分类")
)

// CategoryService 分类业务接口。
type CategoryService interface {
	ListTabs() (*dto.CategoryTabsResp, error)
	Create(ctx context.Context, userID uint, req dto.CategoryCreateReq) (*dto.CategoryItemResp, error)
	Update(ctx context.Context, userID uint, id uint, req dto.CategoryUpdateReq) (*dto.CategoryItemResp, error)
	Delete(ctx context.Context, id uint) (*dto.CategoryItemResp, error)
	AddArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error)
	RemoveArticles(id uint, req dto.CategoryArticlesReq) (*dto.CategoryArticlesResp, error)
	UploadIcon(ctx context.Context, userID uint, name string, data []byte) (*dto.CategoryAssetUploadResp, error)
	UploadCover(ctx context.Context, userID uint, name string, data []byte) (*dto.CategoryAssetUploadResp, error)
}

type categoryService struct {
	repo  categoryrepo.CategoryRepository
	store storage.ObjectStore
	log   *zap.Logger
}

// NewCategoryService 创建分类业务服务实例。
func NewCategoryService(repo categoryrepo.CategoryRepository, store storage.ObjectStore, log *zap.Logger) CategoryService {
	return &categoryService{repo: repo, store: store, log: log}
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
			Icon:         storage.ResolvePtrURL(s.store, row.Icon),
			Description:  row.Description,
			CoverImgUrl:  storage.ResolvePtrURL(s.store, row.CoverImgUrl),
			Seq:          row.Seq,
			ArticleCount: row.ArticleCount,
		})
	}
	return &dto.CategoryTabsResp{List: items}, nil
}

func (s *categoryService) Create(ctx context.Context, userID uint, req dto.CategoryCreateReq) (*dto.CategoryItemResp, error) {
	// 清洗并校验必填字段。
	category, err := newCategoryFromCreateReq(req)
	if err != nil {
		return nil, err
	}

	// 是否有可选素材需要处理
	hasAssets := (req.Icon != nil && strings.TrimSpace(*req.Icon) != "") ||
		(req.CoverImgUrl != nil && strings.TrimSpace(*req.CoverImgUrl) != "")

	if !hasAssets {
		// 无素材直接创建
		row, err := s.repo.CreateWithPrepare(categoryrepo.CategoryCreateData{Category: category})
		if err != nil {
			return nil, err
		}
		return s.categoryWithCountToDTO(row), nil
	}

	// 有素材：使用 prepare callback，在事务内取得 ID 后复制临时素材。
	var copiedKeys []string
	var tempKeys []string

	row, err := s.repo.CreateWithPrepare(categoryrepo.CategoryCreateData{
		Category: category,
		PrepareCategory: func(saved model.Category) (model.Category, error) {
			// 处理图标
			if req.Icon != nil && strings.TrimSpace(*req.Icon) != "" {
				result, normalizeErr := s.normalizeCategoryIconKey(ctx, userID, saved.ID, *req.Icon, nil)
				if normalizeErr != nil {
					return model.Category{}, normalizeErr
				}
				copiedKeys = append(copiedKeys, result.CopiedKeys...)
				if result.TempKey != "" {
					tempKeys = append(tempKeys, result.TempKey)
				}
				saved.Icon = &result.Key
			}
			// 处理封面
			if req.CoverImgUrl != nil && strings.TrimSpace(*req.CoverImgUrl) != "" {
				result, normalizeErr := s.normalizeCategoryCoverKey(ctx, userID, saved.ID, *req.CoverImgUrl, nil)
				if normalizeErr != nil {
					return model.Category{}, normalizeErr
				}
				copiedKeys = append(copiedKeys, result.CopiedKeys...)
				if result.TempKey != "" {
					tempKeys = append(tempKeys, result.TempKey)
				}
				saved.CoverImgUrl = &result.Key
			}
			return saved, nil
		},
	})
	if err != nil {
		// 回滚：删除本次新复制的正式对象
		s.tryDeleteKeys(ctx, copiedKeys)
		return nil, err
	}

	// 成功后：尽力删除临时对象，失败记日志，不把成功伪装成失败
	s.tryDeleteKeys(ctx, tempKeys)
	return s.categoryWithCountToDTO(row), nil
}

func (s *categoryService) Update(ctx context.Context, userID uint, id uint, req dto.CategoryUpdateReq) (*dto.CategoryItemResp, error) {
	// 读取旧素材 key 以便后续清理。
	existing, err := s.repo.Update(id, categoryrepo.CategoryUpdateData{})
	if err != nil {
		return nil, mapCategoryRepoError(err)
	}
	if existing == nil {
		return nil, ErrCategoryNotFound
	}
	oldIconKey := existing.Icon
	oldCoverKey := existing.CoverImgUrl

	// 把可选请求字段转换为明确的更新数据。
	data, err := newCategoryUpdateData(req)
	if err != nil {
		return nil, err
	}

	var copiedIconKeys, copiedCoverKeys []string
	var iconTempKey, coverTempKey string
	var normalizedIcon, normalizedCover string

	// 处理图标素材归一化
	if req.Icon != nil {
		v := strings.TrimSpace(*req.Icon)
		if v == "" {
			// 清空图标
			data.Icon = nil
			data.UpdateIcon = true
		} else {
			result, normalizeErr := s.normalizeCategoryIconKey(ctx, userID, id, v, oldIconKey)
			if normalizeErr != nil {
				return nil, normalizeErr
			}
			copiedIconKeys = result.CopiedKeys
			iconTempKey = result.TempKey
			normalizedIcon = result.Key
			data.Icon = &result.Key
			data.UpdateIcon = true
		}
	}

	// 处理封面素材归一化
	if req.CoverImgUrl != nil {
		v := strings.TrimSpace(*req.CoverImgUrl)
		if v == "" {
			// 清空封面
			data.CoverImgUrl = nil
			data.UpdateCoverImgUrl = true
		} else {
			result, normalizeErr := s.normalizeCategoryCoverKey(ctx, userID, id, v, oldCoverKey)
			if normalizeErr != nil {
				return nil, normalizeErr
			}
			copiedCoverKeys = result.CopiedKeys
			coverTempKey = result.TempKey
			normalizedCover = result.Key
			data.CoverImgUrl = &result.Key
			data.UpdateCoverImgUrl = true
		}
	}

	// 执行数据库更新
	row, err := s.repo.Update(id, data)
	if err != nil {
		// DB 失败：删除本次新复制的正式对象
		s.tryDeleteKeys(ctx, copiedIconKeys)
		s.tryDeleteKeys(ctx, copiedCoverKeys)
		return nil, mapCategoryRepoError(err)
	}
	if row == nil {
		s.tryDeleteKeys(ctx, copiedIconKeys)
		s.tryDeleteKeys(ctx, copiedCoverKeys)
		return nil, ErrCategoryNotFound
	}

	// DB 成功后清理临时对象
	s.tryDeleteKeys(ctx, []string{iconTempKey, coverTempKey})

	// 清理被替换的旧正式对象
	if normalizedIcon != "" && oldIconKey != nil && *oldIconKey != normalizedIcon {
		if replaceableCategoryIconKey(id, *oldIconKey) {
			s.tryDeleteKeys(ctx, []string{*oldIconKey})
		}
	}
	if normalizedCover != "" && oldCoverKey != nil && *oldCoverKey != normalizedCover {
		if replaceableCategoryCoverKey(id, *oldCoverKey) {
			s.tryDeleteKeys(ctx, []string{*oldCoverKey})
		}
	}
	// 清空时删除旧正式对象
	if req.Icon != nil && strings.TrimSpace(*req.Icon) == "" && oldIconKey != nil && *oldIconKey != "" {
		if replaceableCategoryIconKey(id, *oldIconKey) {
			s.tryDeleteKeys(ctx, []string{*oldIconKey})
		}
	}
	if req.CoverImgUrl != nil && strings.TrimSpace(*req.CoverImgUrl) == "" && oldCoverKey != nil && *oldCoverKey != "" {
		if replaceableCategoryCoverKey(id, *oldCoverKey) {
			s.tryDeleteKeys(ctx, []string{*oldCoverKey})
		}
	}

	return s.categoryWithCountToDTO(row), nil
}

func (s *categoryService) Delete(ctx context.Context, id uint) (*dto.CategoryItemResp, error) {
	// 先读取当前图标和封面 key，删除成功后尽力清理。
	existing, err := s.repo.Update(id, categoryrepo.CategoryUpdateData{})
	if err == nil && existing != nil {
		// 记录要清理的资源
		var cleanKeys []string
		if existing.Icon != nil && *existing.Icon != "" && replaceableCategoryIconKey(id, *existing.Icon) {
			cleanKeys = append(cleanKeys, *existing.Icon)
		}
		if existing.CoverImgUrl != nil && *existing.CoverImgUrl != "" && replaceableCategoryCoverKey(id, *existing.CoverImgUrl) {
			cleanKeys = append(cleanKeys, *existing.CoverImgUrl)
		}

		// 执行数据库删除
		category, delErr := s.repo.Delete(id)
		if delErr != nil {
			return nil, mapCategoryRepoError(delErr)
		}
		if category == nil {
			return nil, ErrCategoryNotFound
		}
		// 尽力清理对象存储，失败记日志不回滚
		s.tryDeleteKeys(ctx, cleanKeys)
		return categoryToDTO(category, 0, s.store), nil
	}

	// 无法预读（分类不存在或 DB 错误），直接删除
	category, delErr := s.repo.Delete(id)
	if delErr != nil {
		return nil, mapCategoryRepoError(delErr)
	}
	if category == nil {
		return nil, ErrCategoryNotFound
	}
	return categoryToDTO(category, 0, s.store), nil
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
	return model.Category{
		ParentID: req.ParentID,
		Name:     name,
		URL:      strutil.CleanOptional(req.URL),
		Seq:      *req.Seq,
		// Icon/Description/CoverImgUrl 全部可选，不在此赋值（prepare callback 负责）
	}, nil
}

func newCategoryUpdateData(req dto.CategoryUpdateReq) (categoryrepo.CategoryUpdateData, error) {
	var data categoryrepo.CategoryUpdateData
	data.ParentID = req.ParentID
	data.UpdateParentID = req.ParentID != nil
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return data, ErrCategoryNameRequired
		}
		data.Name = &name
	}
	data.URL, data.UpdateURL = strutil.CleanOptionalUpdate(req.URL)
	// Icon 和 CoverImgUrl 由 Update 调用层在归一化后单独设置 data.Icon/data.CoverImgUrl
	// Description 依然通过 strutil 处理（纯文本，无对象存储）
	data.Description, data.UpdateDescription = strutil.CleanOptionalUpdate(req.Description)
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
	if errors.Is(err, categoryrepo.ErrCategoryArticleMissing) {
		return ErrCategoryArticleMissing
	}
	return err
}

func (s *categoryService) categoryWithCountToDTO(row *categoryrepo.CategoryWithCount) *dto.CategoryItemResp {
	if row == nil {
		return nil
	}
	return categoryToDTO(&row.Category, row.ArticleCount, s.store)
}

func categoryToDTO(category *model.Category, articleCount int64, resolver storage.ObjectURLResolver) *dto.CategoryItemResp {
	return &dto.CategoryItemResp{
		ID:           category.ID,
		ParentID:     category.ParentID,
		Name:         category.Name,
		URL:          category.URL,
		Icon:         storage.ResolvePtrURL(resolver, category.Icon),
		Description:  category.Description,
		CoverImgUrl:  storage.ResolvePtrURL(resolver, category.CoverImgUrl),
		Seq:          category.Seq,
		ArticleCount: articleCount,
	}
}

// tryDeleteKeys 尽力删除对象存储中的 key，失败记日志不返回错误。
func (s *categoryService) tryDeleteKeys(ctx context.Context, keys []string) {
	if s.store == nil {
		return
	}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if err := s.store.DeleteObject(ctx, key); err != nil && s.log != nil {
			s.log.Warn("分类素材清理失败", zap.String("key", key), zap.Error(err))
		}
	}
}
