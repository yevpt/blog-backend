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

// CommentAggregate 一级评论及其用户、回复聚合，供 service 转换为 DTO。
type CommentAggregate struct {
	Comment CommentRecord
	User    *model.User
	Replies []ReplyAggregate
}

// ReplyAggregate 回复及其双方用户聚合，供 service 转换为 DTO。
type ReplyAggregate struct {
	Reply    model.CommentReply
	FromUser *model.User
	ToUser   *model.User
}

// PageResult 评论分页查询结果，保持 repository 不返回 dto。
type PageResult struct {
	Total    int64
	Page     int
	PageSize int
	Comments []CommentAggregate
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
	// List 分页查询一级评论，并附带当前页评论下的回复。
	List(target Target, page int, pageSize int) (*PageResult, error)
	// Create 创建一级评论，并返回评论聚合。
	Create(target Target, userID uint, content string) (*CommentAggregate, error)
	// Reply 创建评论回复，并根据 parent_reply_id 推导被回复用户。
	Reply(data ReplyData) (*ReplyAggregate, error)
	// DeleteComment 软删除一级评论；force 为 true 时跳过归属校验。
	DeleteComment(target Target, commentID uint, userID uint, force bool) (*CommentRecord, error)
	// DeleteReply 软删除回复；force 为 true 时跳过归属校验。
	DeleteReply(replyID uint, userID uint, force bool) (*model.CommentReply, error)
}

type commentRepo struct {
	db *gorm.DB
}

// NewCommentRepository 创建评论仓储实例。
func NewCommentRepository(db *gorm.DB) CommentRepository {
	return &commentRepo{db: db}
}
