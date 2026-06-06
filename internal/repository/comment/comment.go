package comment

import (
	"errors"
	"time"

	"github.com/vpt/blog-backend/internal/model"
	"gorm.io/gorm"
)

const (
	// TargetArticle 表示文章评论，与 comment_reply.comment_type 保持一致。
	TargetArticle uint8 = 1
	// TargetMoment 表示说说评论，与 comment_reply.comment_type 保持一致。
	TargetMoment uint8 = 2
	// TargetGuestbook 表示留言板留言，与 comment_reply.comment_type 保持一致。
	TargetGuestbook uint8 = 3
	// ArticleCommentLikeType 表示 user_like 中的文章评论点赞类型。
	ArticleCommentLikeType uint8 = 2
	// ArticleCommentReplyLikeType 表示 user_like 中的文章评论回复点赞类型。
	ArticleCommentReplyLikeType uint8 = 3
	// MomentCommentLikeType 表示 user_like 中的碎语评论点赞类型。
	MomentCommentLikeType uint8 = 6
	// MomentCommentReplyLikeType 表示 user_like 中的碎语评论回复点赞类型。
	MomentCommentReplyLikeType uint8 = 7
	// GuestbookLikeType 表示 user_like 中的留言点赞类型。
	GuestbookLikeType uint8 = 5
	// GuestbookReplyLikeType 表示 user_like 中的留言回复点赞类型。
	GuestbookReplyLikeType uint8 = 8
)

var (
	// ErrTargetNotFound 表示评论目标不存在、不可见或已删除。
	ErrTargetNotFound = errors.New("评论目标不存在")
	// ErrTargetCommentClosed 表示目标已关闭评论。
	ErrTargetCommentClosed = errors.New("评论已关闭")
	// ErrCommentNotFound 表示一级评论不存在。
	ErrCommentNotFound = errors.New("评论不存在")
	// ErrReplyNotFound 表示回复不存在。
	ErrReplyNotFound = errors.New("回复不存在")
	// ErrNoDeletePermission 表示当前用户无权删除该评论或回复。
	ErrNoDeletePermission = errors.New("无权删除评论")
)

// Target 评论目标，Type 对应 comment_reply.comment_type，ID 对应具体业务目标 ID。
type Target struct {
	Type uint8
	ID   uint
}

// CommentRecord 统一的一级评论记录，屏蔽 article/moment/guestbook 三张表差异。
type CommentRecord struct {
	ID        uint
	TargetID  uint
	UserID    uint
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CommentAggregate 一级评论及其用户、回复数量、点赞信息聚合，供 service 转换为 DTO。
type CommentAggregate struct {
	Comment    CommentRecord
	User       *model.User
	ReplyCount int64
	LikeCount  int64
	IsLiked    bool
}

// ReplyRecord 统一的评论回复记录，屏蔽三张回复表差异。
type ReplyRecord struct {
	ID            uint
	CommentID     uint
	ToUserID      uint
	FromUserID    uint
	ParentReplyID uint
	Content       string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ReplyAggregate 回复及其双方用户聚合，供 service 转换为 DTO。
type ReplyAggregate struct {
	Reply     ReplyRecord
	FromUser  *model.User
	ToUser    *model.User
	LikeCount int64
	IsLiked   bool
}

// PageResult 评论分页查询结果，保持 repository 不返回 dto。
type PageResult struct {
	Total    int64
	Page     int
	PageSize int
	Comments []CommentAggregate
}

// ReplyPageResult 评论回复分页查询结果。
type ReplyPageResult struct {
	Total    int64
	Page     int
	PageSize int
	Replies  []ReplyAggregate
}

// LikeResult 点赞切换结果。
type LikeResult struct {
	IsLiked   bool
	LikeCount int64
}

// ReplyData 创建回复所需的数据。
type ReplyData struct {
	Target        Target
	CommentID     uint
	ParentReplyID uint
	FromUserID    uint
	Content       string
}

// CommentRepository 评论数据访问接口。
type CommentRepository interface {
	// List 分页查询一级评论，并按 viewerID 附带回复数量与点赞信息。
	List(target Target, viewerID *uint, page int, pageSize int) (*PageResult, error)
	// Create 创建一级评论，并返回评论聚合。
	Create(target Target, userID uint, content string) (*CommentAggregate, error)
	// ListReplies 分页查询某条一级评论下的回复。
	ListReplies(target Target, commentID uint, viewerID *uint, page int, pageSize int) (*ReplyPageResult, error)
	// Reply 创建评论回复，并根据 parent_reply_id 推导被回复用户。
	Reply(data ReplyData) (*ReplyAggregate, error)
	// ToggleLike 切换一级评论点赞状态。
	ToggleLike(target Target, commentID uint, userID uint) (*LikeResult, error)
	// ToggleReplyLike 切换评论回复点赞状态。
	ToggleReplyLike(target Target, replyID uint, userID uint) (*LikeResult, error)
	// DeleteComment 软删除一级评论；force 为 true 时跳过归属校验。
	DeleteComment(target Target, commentID uint, userID uint, force bool) (*CommentRecord, error)
	// DeleteReply 软删除回复；force 为 true 时跳过归属校验。
	DeleteReply(target Target, replyID uint, userID uint, force bool) (*ReplyRecord, error)
}

type commentRepo struct {
	db *gorm.DB
}

// NewCommentRepository 创建评论仓储实例。
func NewCommentRepository(db *gorm.DB) CommentRepository {
	return &commentRepo{db: db}
}
