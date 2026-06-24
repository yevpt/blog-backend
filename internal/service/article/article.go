package article

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	articlerepo "github.com/vpt/blog-backend/internal/repository/article"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
	"github.com/vpt/blog-backend/internal/service/uv"
	"github.com/vpt/blog-backend/pkg/storage"
	"gorm.io/gorm"
)

var (
	ErrArticleNotFound           = errors.New("文章不存在")
	ErrArticlePasswordRequired   = errors.New("加密文章必须填写阅读密码")
	ErrArticleCategoryRequired   = errors.New("文章至少需要一个分类")
	ErrArticleNoDeletePermission = errors.New("无权删除文章")
	ErrArticleNotSoftDeleted     = errors.New("文章尚未软删除")
)

// ArticleService 文章业务接口，负责文章查询、保存、点赞和阅读计数。
type ArticleService interface {
	ListIDs() (*dto.ArticleIDsResp, error)
	ListPublic(req dto.ArticleListReq, viewerID *uint) (*dto.ArticlePageResp, error)
	ListAdmin(req dto.AdminArticleListReq) (*dto.AdminArticlePageResp, error)
	GetPublicDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error)
	GetAdminDetail(id uint, viewerID *uint) (*dto.AdminArticleDetailResp, error)
	Save(req dto.ArticleSaveReq, authorID uint) (*dto.ArticleDetailResp, error)
	Delete(id uint) (*dto.ArticleDetailResp, error)
	PermanentDelete(id uint, operatorID uint) (*dto.ArticleDeleteResp, error)
	View(id uint, visitorID string) (*dto.ArticleViewResp, error)
	IsLiked(id uint, userID uint) (*dto.ArticleLikeResp, error)
	ToggleLike(id uint, userID uint) (*dto.ArticleLikeResp, error)
}

type articleService struct {
	repo              articlerepo.ArticleRepository
	objectURLResolver storage.ObjectURLResolver
	uvSvc             uv.UVService
	publisher         notificationservice.Publisher
}

// NewArticleService 创建文章业务服务实例。
// publisher 用于点赞成功后发布通知事件，可为 nil（测试或关闭通知时跳过发布）。
func NewArticleService(repo articlerepo.ArticleRepository, objectURLResolver storage.ObjectURLResolver, uvSvc uv.UVService, publisher notificationservice.Publisher) ArticleService {
	return &articleService{repo: repo, objectURLResolver: objectURLResolver, uvSvc: uvSvc, publisher: publisher}
}

func (s *articleService) ListIDs() (*dto.ArticleIDsResp, error) {
	ids, err := s.repo.ListPublicIDs()
	if err != nil {
		return nil, err
	}
	return &dto.ArticleIDsResp{IDs: ids}, nil
}

func (s *articleService) ListPublic(req dto.ArticleListReq, viewerID *uint) (*dto.ArticlePageResp, error) {
	filter := articlerepo.ArticleListFilter{
		Page:       normalizeArticlePage(req.Page),
		PageSize:   normalizeArticlePageSize(req.PageSize),
		Recommend:  req.Recommend,
		CategoryID: req.CategoryID,
		TagID:      req.TagID,
		Search:     normalizeArticleSearch(req.Search),
	}
	result, err := s.repo.ListPublic(filter, viewerID)
	if err != nil {
		return nil, err
	}
	return articlePageToDTO(result, s.objectURLResolver)
}

func (s *articleService) ListAdmin(req dto.AdminArticleListReq) (*dto.AdminArticlePageResp, error) {
	filter := articlerepo.ArticleListFilter{
		Page:       normalizeArticlePage(req.Page),
		PageSize:   normalizeArticlePageSize(req.PageSize),
		Recommend:  req.Recommend,
		CategoryID: req.CategoryID,
		TagID:      req.TagID,
		Search:     normalizeArticleSearch(req.Search),
		SortBy:     normalizeArticleSortBy(req.SortBy),
		SortOrder:  normalizeArticleSortOrder(req.SortOrder),
	}
	result, err := s.repo.ListAdmin(filter)
	if err != nil {
		return nil, err
	}
	return adminArticlePageToDTO(result, s.objectURLResolver)
}

func (s *articleService) GetPublicDetail(id uint, viewerID *uint) (*dto.ArticleDetailResp, error) {
	aggregate, err := s.repo.FindPublicDetail(id, viewerID)
	if err != nil {
		return nil, err
	}
	if aggregate == nil {
		return nil, ErrArticleNotFound
	}
	return articleDetailToDTO(aggregate, articleContentPublic, s.objectURLResolver)
}

func (s *articleService) GetAdminDetail(id uint, viewerID *uint) (*dto.AdminArticleDetailResp, error) {
	aggregate, err := s.repo.FindAdminDetail(id, viewerID)
	if err != nil {
		return nil, err
	}
	if aggregate == nil {
		return nil, ErrArticleNotFound
	}
	return adminArticleDetailToDTO(aggregate, s.objectURLResolver)
}

func (s *articleService) Save(req dto.ArticleSaveReq, authorID uint) (*dto.ArticleDetailResp, error) {
	categoryIDs := firstCategoryID(req.CategoryIDs)
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

	var oldContent string
	if req.ID != nil {
		oldAggregate, err := s.repo.FindAdminDetail(*req.ID, nil)
		if err != nil {
			return nil, err
		}
		if oldAggregate == nil {
			return nil, ErrArticleNotFound
		}
		oldContent = oldAggregate.Article.Content
		if oldAggregate.Article.CoverImgUrl != nil && strings.TrimSpace(*oldAggregate.Article.CoverImgUrl) != "" {
			oldContent += "\n![](" + strings.TrimSpace(*oldAggregate.Article.CoverImgUrl) + ")"
		}
	}

	store, hasStore := s.objectURLResolver.(storage.ObjectStore)
	if hasArticleImageReferences(article.Content, article.CoverImgUrl) && (!hasStore || store == nil) {
		return nil, ErrArticleImageInvalid
	}

	var copiedKeys []string
	var tempKeys []string
	var newReferencedKeys []string
	var prepareArticle func(model.Article) (model.Article, error)
	if hasStore && store != nil {
		prepareArticle = func(article model.Article) (model.Article, error) {
			normalized, err := normalizeArticleAssets(context.Background(), store, articleAssetNormalizeInput{
				ArticleID: article.ID,
				UserID:    authorID,
				Content:   article.Content,
				Cover:     article.CoverImgUrl,
			})
			if err != nil {
				return model.Article{}, err
			}
			copiedKeys = normalized.CopiedKeys
			tempKeys = normalized.TempKeys
			newReferencedKeys = normalized.ReferencedKeys
			article.Content = normalized.Content
			article.CoverImgUrl = normalized.Cover
			return article, nil
		}
	}

	aggregate, err := s.repo.Save(articlerepo.ArticleSaveData{
		Article:        article,
		CategoryIDs:    categoryIDs,
		Tags:           normalizeArticleTagRelations(req),
		MusicIDs:       uniqueUintIDs(req.MusicIDs),
		Recommend:      req.Recommend,
		RecommendSeq:   req.RecommendSeq,
		PrepareArticle: prepareArticle,
	})
	if err != nil {
		_ = s.deleteArticleAssetKeys(context.Background(), copiedKeys)
		return nil, err
	}
	if aggregate == nil {
		return nil, ErrArticleNotFound
	}
	if err := s.deleteArticleAssetKeys(context.Background(), tempKeys); err != nil {
		return nil, err
	}
	if req.ID != nil {
		if err := s.moveRemovedArticleAssets(context.Background(), oldContent, newReferencedKeys, *req.ID); err != nil {
			return nil, err
		}
	}
	return articleDetailToDTO(aggregate, articleContentAdmin, s.objectURLResolver)
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

func (s *articleService) PermanentDelete(id uint, operatorID uint) (*dto.ArticleDeleteResp, error) {
	article, err := s.repo.FindDeletedByID(id)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, ErrArticleNotFound
	}
	if article.UserID != operatorID {
		return nil, ErrArticleNoDeletePermission
	}
	if !article.DeletedAt.Valid {
		return nil, ErrArticleNotSoftDeleted
	}
	if err := s.moveDeletedArticleAssets(context.Background(), article); err != nil {
		return nil, err
	}

	deleted, err := s.repo.PermanentDelete(id, operatorID)
	if err != nil {
		return nil, mapArticleDeleteError(err)
	}
	if deleted == nil {
		return nil, ErrArticleNotFound
	}
	return &dto.ArticleDeleteResp{ID: deleted.ID}, nil
}

func (s *articleService) View(id uint, visitorID string) (*dto.ArticleViewResp, error) {
	// 访客去重：同一访客 24 小时内只计一次。
	isNew := true
	if visitorID != "" {
		is, err := s.uvSvc.CheckAndMark(context.Background(), "article:viewed", strconv.FormatUint(uint64(id), 10), visitorID, 24*time.Hour)
		if err != nil {
			// Redis 异常时降级，视作新访客。
			isNew = true
		} else {
			isNew = is
		}
	}
	if !isNew {
		// 重复访客，返回当前阅读数，不增加。
		aggregate, err := s.repo.FindPublicDetail(id, nil)
		if err != nil {
			return nil, err
		}
		if aggregate == nil {
			return nil, ErrArticleNotFound
		}
		return &dto.ArticleViewResp{ID: aggregate.Article.ID, ViewCount: aggregate.Article.ReadCount}, nil
	}
	article, err := s.repo.IncrementReadCount(id)
	if err != nil {
		return nil, err
	}
	if article == nil {
		return nil, ErrArticleNotFound
	}
	return &dto.ArticleViewResp{ID: article.ID, ViewCount: article.ReadCount}, nil
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

func (s *articleService) ToggleLike(id uint, userID uint) (*dto.ArticleLikeResp, error) {
	aggregate, liked, err := s.repo.ToggleLike(id, userID)
	if err != nil {
		return nil, err
	}
	if aggregate == nil {
		return nil, ErrArticleNotFound
	}
	// 仅在本次为点赞（而非取消）时发布通知事件，接收人由分发器按文章作者解析。
	// 自己点赞自己的文章不产生通知事件。
	if liked && aggregate.Article.UserID != userID {
		s.notifyArticleLiked(id, userID, aggregate.Article.Title, notificationArticleExcerpt(aggregate.Article))
	}
	return &dto.ArticleLikeResp{IsLiked: liked, LikeCount: aggregate.LikeCount}, nil
}

// notifyArticleLiked 发布 article_liked 事件；点赞仅站内通知，不进邮件队列。
func (s *articleService) notifyArticleLiked(articleID uint, userID uint, title string, excerpt string) {
	if s.publisher == nil {
		return
	}
	actorID := userID
	_, _ = s.publisher.Publish(context.Background(), notificationservice.PublishEvent{
		Type:        notificationservice.EventTypeArticleLiked,
		ActorUserID: &actorID,
		SourceType:  "article",
		SourceID:    articleID,
		RootType:    "article",
		RootID:      articleID,
		Title:       title,
		Metadata: notificationservice.BuildSourceRootMetadata(
			notificationservice.NotificationSnapshot{Type: "article", ID: articleID, Title: title, Excerpt: excerpt},
			&notificationservice.NotificationSnapshot{Type: "article", ID: articleID, Title: title, Excerpt: excerpt},
		),
	})
}

func notificationArticleExcerpt(article model.Article) string {
	if article.ShortContent != nil {
		if excerpt := strings.TrimSpace(*article.ShortContent); excerpt != "" {
			return excerpt
		}
	}
	return strings.TrimSpace(article.Content)
}

func mapArticleDeleteError(err error) error {
	if errors.Is(err, articlerepo.ErrNoDeletePermission) {
		return ErrArticleNoDeletePermission
	}
	if errors.Is(err, articlerepo.ErrArticleNotSoftDeleted) {
		return ErrArticleNotSoftDeleted
	}
	return err
}

func (s *articleService) deleteArticleAssetKeys(ctx context.Context, keys []string) error {
	store, ok := s.objectURLResolver.(storage.ObjectStore)
	if !ok || store == nil || len(keys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if err := store.DeleteObject(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *articleService) moveRemovedArticleAssets(ctx context.Context, oldContent string, newKeys []string, articleID uint) error {
	if strings.TrimSpace(oldContent) == "" {
		return nil
	}
	mover, ok := s.objectURLResolver.(storage.ObjectMover)
	if !ok || mover == nil {
		return nil
	}
	oldKeys := articleAssetValues(articleID, oldContent)
	if len(oldKeys) == 0 {
		return nil
	}
	newKeySet := make(map[string]struct{}, len(newKeys))
	for _, key := range newKeys {
		key = strings.TrimSpace(key)
		if key != "" {
			newKeySet[key] = struct{}{}
		}
	}
	moved := make(map[string]struct{}, len(oldKeys))
	for _, key := range oldKeys {
		if _, keep := newKeySet[key]; keep {
			continue
		}
		if _, exists := moved[key]; exists {
			continue
		}
		moved[key] = struct{}{}
		if err := mover.MoveObject(ctx, key, "deleted/"+key); err != nil {
			return err
		}
	}
	return nil
}
