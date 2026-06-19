package article

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

// ArticleLikeType 表示 user_like 中的文章点赞类型。
const ArticleLikeType uint8 = 1

var (
	// ErrNoDeletePermission 表示当前用户不是文章作者，不能真实删除文章。
	ErrNoDeletePermission = errors.New("无权删除文章")
	// ErrArticleNotSoftDeleted 表示文章尚未软删除，不能真实删除。
	ErrArticleNotSoftDeleted = errors.New("文章尚未软删除")
)

// ArticleListFilter 文章分页查询过滤条件。
type ArticleListFilter struct {
	Page       int
	PageSize   int
	Recommend  *bool
	CategoryID *uint
	TagID      *uint
	Search     *string
	SortBy     string
	SortOrder  string
}

// ArticlePageResult 文章分页查询结果，保持 model 聚合，不返回 dto。
type ArticlePageResult struct {
	Total    int64
	Page     int
	PageSize int
	Articles []ArticleAggregate
}

// ArticleAggregate 文章详情聚合模型，用于 service 转换为 dto。
type ArticleAggregate struct {
	Article      model.Article
	User         *model.User
	Categories   []model.Category
	Tags         []model.Tag
	Music        []model.Music
	Recommend    *model.ArticleRecommend
	LikeCount    int64
	CommentCount int64
	IsLiked      bool
}

// ArticleSaveData 保存文章所需的文章主表和关联数据。
type ArticleSaveData struct {
	Article      model.Article
	CategoryIDs  []uint
	TagIDs       []uint
	MusicIDs     []uint
	Recommend    bool
	RecommendSeq uint
}

// ArticleRepository 文章数据访问接口，只返回 model 或聚合模型。
type ArticleRepository interface {
	ListPublic(filter ArticleListFilter, viewerID *uint) (*ArticlePageResult, error)
	ListAdmin(filter ArticleListFilter) (*ArticlePageResult, error)
	ListPublicIDs() ([]uint, error)
	FindPublicDetail(id uint, viewerID *uint) (*ArticleAggregate, error)
	FindAdminDetail(id uint, viewerID *uint) (*ArticleAggregate, error)
	FindDeletedByID(id uint) (*model.Article, error)
	Save(data ArticleSaveData) (*ArticleAggregate, error)
	SoftDelete(id uint) (*model.Article, error)
	PermanentDelete(id uint, operatorID uint) (*model.Article, error)
	IncrementReadCount(id uint) (*model.Article, error)
	IsLiked(articleID uint, userID uint) (bool, int64, error)
	ToggleLike(articleID uint, userID uint) (*ArticleAggregate, bool, error)
}

type articleRepo struct {
	db *gorm.DB
}

// NewArticleRepository 创建文章仓储实例。
func NewArticleRepository(db *gorm.DB) ArticleRepository {
	return &articleRepo{db: db}
}
