package service

import (
	"context"
	"errors"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	"github.com/vpt/blog-backend/internal/repository"
	"gorm.io/gorm"
)

var (
	ErrArticleNotFound         = errors.New("文章不存在")
	ErrArticlePasswordRequired = errors.New("加密文章必须填写阅读密码")
	ErrArticleCategoryRequired = errors.New("文章至少需要一个分类")
)

// ArticleService 文章业务接口，负责文章查询、保存、点赞和阅读计数。
type ArticleService interface {
	ListIDs() (*dto.ArticleIDsResp, error)
	ListPublic(req dto.ArticleListReq) (*dto.ArticlePageResp, error)
	GetPublicDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error)
	GetAdminDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error)
	Save(req dto.ArticleSaveReq, authorID uint) (*dto.ArticleDetailResp, error)
	Delete(id uint) (*dto.ArticleDetailResp, error)
	Read(id uint) (*dto.ArticleReadResp, error)
	IsLiked(id uint, userID uint) (*dto.ArticleLikeResp, error)
	ToggleLike(id uint, userID uint) (*dto.ArticleDetailResp, error)
}

// ObjectURLResolver 解析对象存储 key，返回可直接访问的 Garage 或 CDN 签名 URL。
type ObjectURLResolver interface {
	ObjectURL(ctx context.Context, objectName string) (string, error)
}

type articleService struct {
	repo              repository.ArticleRepository
	objectURLResolver ObjectURLResolver
}

// NewArticleService 创建文章业务服务实例。
func NewArticleService(repo repository.ArticleRepository, objectURLResolver ObjectURLResolver) ArticleService {
	return &articleService{repo: repo, objectURLResolver: objectURLResolver}
}

func (s *articleService) ListIDs() (*dto.ArticleIDsResp, error) {
	ids, err := s.repo.ListPublicIDs()
	if err != nil {
		return nil, err
	}
	return &dto.ArticleIDsResp{IDs: ids}, nil
}

func (s *articleService) ListPublic(req dto.ArticleListReq) (*dto.ArticlePageResp, error) {
	filter := repository.ArticleListFilter{
		Page:       normalizeArticlePage(req.Page),
		PageSize:   normalizeArticlePageSize(req.PageSize),
		Recommend:  req.Recommend,
		CategoryID: req.CategoryID,
		TagID:      req.TagID,
	}
	result, err := s.repo.ListPublic(filter)
	if err != nil {
		return nil, err
	}
	return articlePageToDTO(result, s.objectURLResolver)
}

func (s *articleService) GetPublicDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	aggregate, err := s.repo.FindPublicDetail(id, viewerID)
	if err != nil {
		return nil, err
	}
	if aggregate == nil {
		return nil, ErrArticleNotFound
	}
	return articleDetailToDTO(aggregate, articleContentPublic), nil
}

func (s *articleService) GetAdminDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	aggregate, err := s.repo.FindAdminDetail(id, viewerID)
	if err != nil {
		return nil, err
	}
	if aggregate == nil {
		return nil, ErrArticleNotFound
	}
	return articleDetailToDTO(aggregate, articleContentAdmin), nil
}

func (s *articleService) Save(req dto.ArticleSaveReq, authorID uint) (*dto.ArticleDetailResp, error) {
	categoryIDs := uniqueUintIDs(req.CategoryIDs)
	if len(categoryIDs) == 0 {
		return nil, ErrArticleCategoryRequired
	}
	password := cleanArticlePassword(req.Status, req.Password)
	if req.Status == 2 && password == nil {
		return nil, ErrArticlePasswordRequired
	}

	article := model.Article{
		Title:         strings.TrimSpace(req.Title),
		CoverImgUrl:   req.CoverImgUrl,
		ShortContent:  req.ShortContent,
		Content:       req.Content,
		UserID:        authorID,
		Status:        req.Status,
		CommentStatus: req.CommentStatus,
		Password:      password,
	}
	if req.ID != nil {
		article.ID = *req.ID
	}

	aggregate, err := s.repo.Save(repository.ArticleSaveData{
		Article:      article,
		CategoryIDs:  categoryIDs,
		TagIDs:       uniqueUintIDs(req.TagIDs),
		MusicIDs:     uniqueUintIDs(req.MusicIDs),
		Recommend:    req.Recommend,
		RecommendSeq: req.RecommendSeq,
	})
	if err != nil {
		return nil, err
	}
	if aggregate == nil {
		return nil, ErrArticleNotFound
	}
	return articleDetailToDTO(aggregate, articleContentAdmin), nil
}

func (s *articleService) Delete(id uint) (*dto.ArticleDetailResp, error) {
	article, err := s.repo.SoftDelete(id)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, ErrArticleNotFound
	}
	return deletedArticleToDTO(article), nil
}

func (s *articleService) Read(id uint) (*dto.ArticleReadResp, error) {
	article, err := s.repo.IncrementReadCount(id)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, ErrArticleNotFound
	}
	return &dto.ArticleReadResp{ID: article.ID, ReadCount: article.ReadCount}, nil
}

func (s *articleService) IsLiked(id uint, userID uint) (*dto.ArticleLikeResp, error) {
	liked, count, err := s.repo.IsLiked(id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrArticleNotFound
		}
		return nil, err
	}
	return &dto.ArticleLikeResp{IsLiked: liked, LikeCount: count}, nil
}

func (s *articleService) ToggleLike(id uint, userID uint) (*dto.ArticleDetailResp, error) {
	aggregate, _, err := s.repo.ToggleLike(id, userID)
	if err != nil {
		return nil, err
	}
	if aggregate == nil {
		return nil, ErrArticleNotFound
	}
	return articleDetailToDTO(aggregate, articleContentPublic), nil
}
