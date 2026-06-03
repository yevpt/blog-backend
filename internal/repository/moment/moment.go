package moment

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

const (
	// MomentLikeType 表示 user_like 中的碎语点赞类型。
	MomentLikeType uint8 = 3
	// MomentMediaOwnerType 表示 media 中的碎语所属类型。
	MomentMediaOwnerType uint8 = 2
	// MomentImageType 表示 media 中的图片类型。
	MomentImageType uint8 = 0
	// MaxTopMomentsPerUser 限制每个用户最多置顶三条碎语。
	MaxTopMomentsPerUser int64 = 3
)

var (
	// ErrMomentNotFound 表示碎语不存在、不可见或已删除。
	ErrMomentNotFound = errors.New("碎语不存在")
	// ErrAuthorNotFound 表示指定作者不存在或已禁用。
	ErrAuthorNotFound = errors.New("碎语作者不存在")
	// ErrNoPermission 表示当前用户无权操作该碎语。
	ErrNoPermission = errors.New("无权操作碎语")
	// ErrTopLimitExceeded 表示用户置顶碎语数量已达上限。
	ErrTopLimitExceeded = errors.New("最多置顶三条碎语")
)

// ListFilter 碎语分页查询过滤条件。
type ListFilter struct {
	Page     int
	PageSize int
	UserID   *uint
	RoleID   *uint
}

// SaveData 保存碎语所需的主表、图片和权限信息。
type SaveData struct {
	Moment     model.Moment
	Images     []model.Media
	OperatorID uint
	Force      bool
}

// MomentAggregate 碎语及其作者、图片、点赞和评论聚合。
type MomentAggregate struct {
	Moment       model.Moment
	User         *model.User
	Images       []model.Media
	LikeCount    int64
	CommentCount int64
	IsLiked      bool
}

// PageResult 碎语分页查询结果，repository 不直接返回 dto。
type PageResult struct {
	Total    int64
	Page     int
	PageSize int
	Moments  []MomentAggregate
}

// MomentRepository 碎语数据访问接口。
type MomentRepository interface {
	List(filter ListFilter, viewerID *uint) (*PageResult, error)
	FindPublicDetail(id uint, viewerID *uint) (*MomentAggregate, error)
	Save(data SaveData) (*MomentAggregate, error)
	Delete(id uint, operatorID uint, force bool) (*model.Moment, error)
	SetTop(id uint, operatorID uint, force bool) (*model.Moment, error)
	RemoveTop(id uint, operatorID uint, force bool) (*model.Moment, error)
	IncrementReadCount(id uint) (*model.Moment, error)
	IsLiked(id uint, userID uint) (bool, int64, error)
	ToggleLike(id uint, userID uint) (*MomentAggregate, bool, error)
}

type momentRepo struct {
	db *gorm.DB
}

// NewMomentRepository 创建碎语仓储实例。
func NewMomentRepository(db *gorm.DB) MomentRepository {
	return &momentRepo{db: db}
}
