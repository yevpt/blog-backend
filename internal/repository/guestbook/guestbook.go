package guestbook

import (
	"errors"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

const (
	// LikeType 表示 user_like 中的留言板留言点赞类型。
	LikeType uint8 = 5
)

var (
	// ErrOwnerNotFound 表示留言板主人不存在或已禁用。
	ErrOwnerNotFound = errors.New("留言板主人不存在")
	// ErrGuestbookNotFound 表示留言不存在。
	ErrGuestbookNotFound = errors.New("留言不存在")
	// ErrNoDeletePermission 表示当前用户无权删除留言。
	ErrNoDeletePermission = errors.New("无权删除留言")
)

// GuestbookAggregate 留言及其用户、点赞信息聚合，供 service 转换为 DTO。
type GuestbookAggregate struct {
	Message    model.Guestbook
	User       *model.User
	ReplyCount int64
	LikeCount  int64
	IsLiked    bool
}

// PageResult 留言分页查询结果，repository 不直接返回 dto。
type PageResult struct {
	Total    int64
	Page     int
	PageSize int
	Messages []GuestbookAggregate
}

// LikeResult 留言点赞切换结果。
type LikeResult struct {
	ID        uint
	IsLiked   bool
	LikeCount int64
}

// GuestbookRepository 留言板数据访问接口。
type GuestbookRepository interface {
	// List 分页查询指定用户留言板，并按 viewerID 补充点赞状态。
	List(ownerUserID uint, viewerID *uint, page int, pageSize int) (*PageResult, error)
	// Create 创建留言，并返回留言聚合。
	Create(ownerUserID uint, fromUserID uint, content string) (*GuestbookAggregate, error)
	// ToggleLike 切换当前用户对留言的点赞状态。
	ToggleLike(id uint, userID uint) (*LikeResult, error)
	// Delete 软删除留言；force 为 true 时跳过归属校验。
	Delete(id uint, userID uint, force bool) (*model.Guestbook, error)
}

type guestbookRepo struct {
	db *gorm.DB
}

// NewGuestbookRepository 创建留言板仓储实例。
func NewGuestbookRepository(db *gorm.DB) GuestbookRepository {
	return &guestbookRepo{db: db}
}
