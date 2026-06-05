package service

import (
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	articleservice "github.com/vpt/blog-backend/internal/service/article"
	"gorm.io/gorm"
)

var (
	ErrTagNotFound        = errors.New("标签不存在")
	ErrTagNameRequired    = errors.New("标签名称不能为空")
	ErrTagSeqRequired     = errors.New("标签排序不能为空")
	ErrTagArticleRequired = errors.New("标签文章不能为空")
	ErrTagArticleMissing  = errors.New("标签文章不存在")
)

// TagService 标签业务接口。
type TagService interface {
	List() (*dto.TagListResp, error)
	Get(id uint) (*dto.TagItemResp, error)
	ListArticles(id uint, req dto.ArticleListReq) (*dto.ArticlePageResp, error)
	Create(req dto.TagCreateReq) (*dto.TagItemResp, error)
	Update(id uint, req dto.TagUpdateReq) (*dto.TagItemResp, error)
	Delete(id uint) (*dto.TagItemResp, error)
	AddArticles(id uint, req dto.TagArticlesReq) (*dto.TagArticlesResp, error)
	RemoveArticles(id uint, req dto.TagArticlesReq) (*dto.TagArticlesResp, error)
}

type tagService struct {
	repo       repository.TagRepository
	articleSvc articleservice.ArticleService
}

// NewTagService 创建标签业务服务实例。
func NewTagService(repo repository.TagRepository, articleSvc articleservice.ArticleService) TagService {
	return &tagService{repo: repo, articleSvc: articleSvc}
}

func (s *tagService) List() (*dto.TagListResp, error) {
	// 查询标签及公开文章数量，service 不直接访问数据库。
	rows, err := s.repo.ListWithArticleCount()
	if err != nil {
		return nil, err
	}
	// 将 model 聚合转换为对外 DTO，避免暴露数据库结构。
	items := make([]dto.TagItemResp, 0, len(rows))
	for _, row := range rows {
		items = append(items, *tagWithCountToDTO(&row))
	}
	return &dto.TagListResp{List: items}, nil
}

func (s *tagService) Get(id uint) (*dto.TagItemResp, error) {
	// 单标签查询返回公开文章数量，方便前端直接展示标签元信息。
	row, err := s.repo.FindWithArticleCount(id)
	if err != nil {
		return nil, mapTagRepoError(err)
	}
	if row == nil {
		return nil, ErrTagNotFound
	}
	return tagWithCountToDTO(row), nil
}

func (s *tagService) ListArticles(id uint, req dto.ArticleListReq) (*dto.ArticlePageResp, error) {
	// 先确认标签存在，避免不存在标签被文章分页自然返回空列表。
	row, err := s.repo.FindWithArticleCount(id)
	if err != nil {
		return nil, mapTagRepoError(err)
	}
	if row == nil {
		return nil, ErrTagNotFound
	}
	// 文章分页复用文章模块能力，保持过滤、排序和响应转换一致。
	req.TagID = &id
	return s.articleSvc.ListPublic(req, nil)
}

func (s *tagService) Create(req dto.TagCreateReq) (*dto.TagItemResp, error) {
	// 先清洗并校验必填字段，确保进入仓储的数据已经有业务语义。
	tag, err := newTagFromCreateReq(req)
	if err != nil {
		return nil, err
	}
	// 调用仓储创建标签，返回带文章数量的聚合结果。
	row, err := s.repo.Create(tag)
	if err != nil {
		return nil, err
	}
	return tagWithCountToDTO(row), nil
}

func (s *tagService) Update(id uint, req dto.TagUpdateReq) (*dto.TagItemResp, error) {
	// 把可选请求字段转换为明确的更新数据，未传字段不参与更新。
	data, err := newTagUpdateData(req)
	if err != nil {
		return nil, err
	}
	// 仓储返回 nil 表示目标标签不存在。
	row, err := s.repo.Update(id, data)
	if err != nil {
		return nil, mapTagRepoError(err)
	}
	if row == nil {
		return nil, ErrTagNotFound
	}
	return tagWithCountToDTO(row), nil
}

func (s *tagService) Delete(id uint) (*dto.TagItemResp, error) {
	// 删除标签只清空文章标签关系，不删除文章主表数据。
	tag, err := s.repo.Delete(id)
	if err != nil {
		return nil, mapTagRepoError(err)
	}
	if tag == nil {
		return nil, ErrTagNotFound
	}
	return tagToDTO(tag, 0), nil
}

func (s *tagService) AddArticles(id uint, req dto.TagArticlesReq) (*dto.TagArticlesResp, error) {
	// 归一化文章 ID，支持单个和批量，并去掉重复与 0 值。
	articleIDs, err := normalizeTagArticleIDs(req.ArticleIDs)
	if err != nil {
		return nil, err
	}
	// 仓储会跳过已经存在的文章标签关系。
	affected, err := s.repo.AddArticles(id, articleIDs)
	if err != nil {
		return nil, mapTagRepoError(err)
	}
	return &dto.TagArticlesResp{TagID: id, ArticleIDs: articleIDs, AffectedCount: affected}, nil
}

func (s *tagService) RemoveArticles(id uint, req dto.TagArticlesReq) (*dto.TagArticlesResp, error) {
	// 归一化文章 ID，删除关系时不触碰文章本身。
	articleIDs, err := normalizeTagArticleIDs(req.ArticleIDs)
	if err != nil {
		return nil, err
	}
	// 仓储只删除当前标签与这些文章的关联。
	affected, err := s.repo.RemoveArticles(id, articleIDs)
	if err != nil {
		return nil, mapTagRepoError(err)
	}
	return &dto.TagArticlesResp{TagID: id, ArticleIDs: articleIDs, AffectedCount: affected}, nil
}

func newTagFromCreateReq(req dto.TagCreateReq) (model.Tag, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return model.Tag{}, ErrTagNameRequired
	}
	if req.Seq == nil {
		return model.Tag{}, ErrTagSeqRequired
	}
	return model.Tag{
		Name:        name,
		URL:         cleanOptionalString(req.URL),
		Icon:        cleanOptionalString(req.Icon),
		Description: cleanOptionalString(req.Description),
		CoverImgUrl: cleanOptionalString(req.CoverImgUrl),
		Seq:         *req.Seq,
	}, nil
}

func newTagUpdateData(req dto.TagUpdateReq) (repository.TagUpdateData, error) {
	var data repository.TagUpdateData
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return data, ErrTagNameRequired
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

func normalizeTagArticleIDs(ids []uint) ([]uint, error) {
	unique := uniqueTagArticleIDs(ids)
	if len(unique) == 0 {
		return nil, ErrTagArticleRequired
	}
	return unique, nil
}

func uniqueTagArticleIDs(ids []uint) []uint {
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

func mapTagRepoError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrTagNotFound
	}
	if errors.Is(err, repository.ErrTagArticleMissing) {
		return ErrTagArticleMissing
	}
	return err
}

func tagWithCountToDTO(row *repository.TagWithCount) *dto.TagItemResp {
	if row == nil {
		return nil
	}
	return tagToDTO(&row.Tag, row.ArticleCount)
}

func tagToDTO(tag *model.Tag, articleCount int64) *dto.TagItemResp {
	return &dto.TagItemResp{
		ID:           tag.ID,
		Name:         tag.Name,
		URL:          tag.URL,
		Icon:         tag.Icon,
		Description:  tag.Description,
		CoverImgUrl:  tag.CoverImgUrl,
		Seq:          tag.Seq,
		ArticleCount: articleCount,
	}
}
